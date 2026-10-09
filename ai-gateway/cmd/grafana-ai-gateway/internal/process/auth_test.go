package process

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAuthenticator_StaticSnapshot(t *testing.T) {
	effective := config.EffectiveAuth{Type: config.AuthStaticKey, Identities: map[string]config.StaticIdentityConfig{"second": {KeyEnv: "SECOND"}, "deployment": {KeyEnv: "KEY"}}}
	values := map[string]string{"KEY": "old._~+/-key==", "SECOND": "second-key"}
	var lookups []string
	lookup := func(name string) (string, bool) {
		lookups = append(lookups, name)
		value, ok := values[name]
		return value, ok
	}
	authenticator, source, err := buildAuthenticator(context.Background(), config.Settings{}, effective, lookup, testLogger())
	require.NoError(t, err)
	assert.Equal(t, gatewayauth.SourceStaticKey, source)
	assert.Equal(t, []string{"KEY", "SECOND"}, lookups)
	values["KEY"] = "replacement"
	for _, key := range []string{"old._~+/-key==", "second-key"} {
		_, err := authenticator.Authenticate(context.Background(), http.Header{"X-Access-Token": {key}})
		require.NoError(t, err)
	}
	_, err = authenticator.Authenticate(context.Background(), http.Header{"X-Access-Token": {"replacement"}})
	require.Error(t, err)
	restarted, _, err := buildAuthenticator(context.Background(), config.Settings{}, effective, lookup, testLogger())
	require.NoError(t, err)
	_, err = restarted.Authenticate(context.Background(), http.Header{"X-Access-Token": {"replacement"}})
	require.NoError(t, err)
	_, err = restarted.Authenticate(context.Background(), http.Header{"X-Access-Token": {"old._~+/-key=="}})
	require.Error(t, err)
}

func TestBuildAuthenticator_StaticInvalidSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		present   bool
	}{{"absent", "", false}, {"empty", "", true}, {"whitespace", "SECRET_SENTINEL ", true}, {"invalid", "SECRET_SENTINEL!", true}} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildAuthenticator(context.Background(), config.Settings{}, config.EffectiveAuth{Type: config.AuthStaticKey, Identities: map[string]config.StaticIdentityConfig{"deployment": {KeyEnv: "KEY"}}}, func(string) (string, bool) { return tc.key, tc.present }, testLogger())
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SECRET_SENTINEL")
		})
	}
	for _, sameRef := range []bool{true, false} {
		t.Run(map[bool]string{true: "shared reference", false: "distinct references"}[sameRef], func(t *testing.T) {
			second := "SECOND"
			if sameRef {
				second = "KEY"
			}
			calls := 0
			_, _, err := buildAuthenticator(context.Background(), config.Settings{}, config.EffectiveAuth{Type: config.AuthStaticKey, Identities: map[string]config.StaticIdentityConfig{"a": {KeyEnv: "KEY"}, "b": {KeyEnv: second}}}, func(string) (string, bool) { calls++; return "SECRET_SENTINEL", true }, testLogger())
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SECRET_SENTINEL")
			if sameRef {
				assert.Equal(t, 1, calls)
			} else {
				assert.Equal(t, 2, calls)
			}
		})
	}
}

func TestRun_StaticProductionBootstrap(t *testing.T) {
	path := writeProcessConfig(t, "https://api.anthropic.com")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data = append(data, []byte("\nauth:\n  type: static-key\n  staticKey:\n    identities:\n      deployment:\n        keyEnv: GATEWAY_KEY\nserver:\n  cloud:\n    enabled: false\n")...)
	require.NoError(t, os.WriteFile(path, data, 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addresses := make(chan string, 2)
	result := make(chan error, 1)
	var logs bytes.Buffer
	var keyCalls, providerCalls, exportCalls atomic.Int32
	go func() {
		result <- Run(ctx, []string{"--config.file=" + path, "--server.private-listen-address=127.0.0.1:0", "--server.operational-listen-address=127.0.0.1:0", "--server.cloud-listen-address=invalid-unused", "--agento11y.auth-secret-env=EXPORT_KEY", "--auth.jwks-timeout=-1s"}, func(name string) (string, bool) {
			switch name {
			case "GATEWAY_KEY":
				keyCalls.Add(1)
				return "STATIC_SECRET_SENTINEL", true
			case "ANTHROPIC_SECRET":
				providerCalls.Add(1)
				return "PROVIDER_SECRET_SENTINEL", true
			case "EXPORT_KEY":
				exportCalls.Add(1)
				return "EXPORT_SECRET_SENTINEL", true
			}
			return "", false
		}, func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err == nil {
				addresses <- listener.Addr().String()
			}
			return listener, err
		}, slog.New(slog.NewJSONHandler(&logs, nil)))
	}()
	bound := []string{}
	for range 2 {
		select {
		case address := <-addresses:
			bound = append(bound, address)
		case err := <-result:
			require.FailNow(t, "startup failed", "%v", err)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "missing listener")
		}
	}
	require.Eventually(t, func() bool {
		response, err := http.Get("http://" + bound[1] + "/ready")
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == 200
	}, 3*time.Second, 10*time.Millisecond)
	for _, key := range []string{"", "wrong", "STATIC_SECRET_SENTINEL"} {
		request, err := http.NewRequest(http.MethodGet, "http://"+bound[0]+"/api/v1/aisdk/config", nil)
		require.NoError(t, err)
		if key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		_ = response.Body.Close()
		if key == "STATIC_SECRET_SENTINEL" {
			assert.Equal(t, 200, response.StatusCode)
		} else {
			assert.Equal(t, 401, response.StatusCode)
		}
		assert.NotContains(t, string(body), "SECRET_SENTINEL")
	}
	cancel()
	require.NoError(t, <-result)
	assert.EqualValues(t, 1, keyCalls.Load())
	assert.EqualValues(t, 1, providerCalls.Load())
	assert.Zero(t, exportCalls.Load())
	assert.NotContains(t, logs.String(), "SECRET_SENTINEL")
	assert.Contains(t, logs.String(), "static-key")
}

func TestRun_AuthConfigurationFailsBeforeSecrets(t *testing.T) {
	for _, tc := range []struct{ name, suffix string }{
		{"null auth", "auth: null"},
		{"binary auth key", "!!binary YXV0aA==: null"},
		{"quoted off", "server: {cloud: {enabled: 'off'}}"},
		{"boolean env", "auth: {type: static-key, staticKey: {identities: {app: {keyEnv: true}}}}"},
		{"numeric identity", "auth: {type: static-key, staticKey: {identities: {123: {keyEnv: KEY}}}}"},
		{"merged null auth", "<<: {auth: null}"},
		{"merged null provider", "auth: {type: unsafe, <<: {jwt: null}}"},
		{"explicit conflict", "auth: {type: unsafe}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProcessConfig(t, "https://api.anthropic.com")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, append(data, []byte("\n"+tc.suffix+"\n")...), 0600))
			secretCalls := 0
			bindCalls := 0
			args := []string{"--config.file=" + path, "--deployment.mode=development", "--server.private-listen-address=127.0.0.1:0", "--server.cloud-listen-address=127.0.0.1:0", "--server.operational-listen-address=127.0.0.1:0"}
			if tc.name != "boolean env" && tc.name != "numeric identity" {
				args = append(args, "--auth.unsafe")
			}
			err = Run(context.Background(), args, func(name string) (string, bool) {
				if name == "ANTHROPIC_SECRET" || name == "true" || name == "KEY" {
					secretCalls++
					return "SECRET_SENTINEL", true
				}
				return "", false
			}, func(string, string) (net.Listener, error) { bindCalls++; return nil, assert.AnError }, testLogger())
			require.Error(t, err)
			assert.Zero(t, secretCalls)
			assert.Zero(t, bindCalls)
			assert.NotContains(t, err.Error(), "SECRET_SENTINEL")
		})
	}
}

func TestBuildAuthenticator_StaticProvisioningUnchanged(t *testing.T) {
	const name, key = "STATIC_CLEANUP_KEY", "opaque._~+/-key=="
	t.Setenv(name, key)
	effective := config.EffectiveAuth{Type: config.AuthStaticKey, Identities: map[string]config.StaticIdentityConfig{"deployment": {KeyEnv: name}}}
	authenticator, _, err := buildAuthenticator(context.Background(), config.Settings{}, effective, os.LookupEnv, testLogger())
	require.NoError(t, err)
	assert.Equal(t, key, os.Getenv(name))
	for _, headers := range []http.Header{{"X-Access-Token": {key}}, {"Authorization": {"Bearer " + key}}} {
		before := headers.Clone()
		_, err := authenticator.Authenticate(context.Background(), headers)
		require.NoError(t, err)
		assert.Equal(t, before, headers)
	}
	assert.Equal(t, key, os.Getenv(name))
}
