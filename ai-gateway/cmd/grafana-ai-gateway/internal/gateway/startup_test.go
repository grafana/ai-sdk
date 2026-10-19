package gateway

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

type startupEnvironment struct {
	values  map[string]string
	secrets map[string]int
}

func newStartupEnvironment(t *testing.T, providerURL string) *startupEnvironment {
	t.Helper()
	host, err := platformhost.New(platformhost.Config{Topology: platformhost.TopologyCloud, PluginID: PluginID, Group: "aigateway.ext.grafana.app", Version: "v1alpha1"})
	require.NoError(t, err)
	keys, err := host.JWKS()
	require.NoError(t, err)
	return &startupEnvironment{
		values: map[string]string{
			"GRAFANA_AI_GATEWAY_CONFIG_FILE": writeModelConfig(t, providerURL),
			"GRAFANA_AI_GATEWAY_ACCESS_MODE": "cloud",
			"GRAFANA_JWKS":                   string(keys),
			"ANTHROPIC_SECRET":               providerSecret,
		},
		secrets: map[string]int{},
	}
}

func (e *startupEnvironment) lookup(name string) (string, bool) {
	if name == "ANTHROPIC_SECRET" || name == "AO_SECRET" {
		e.secrets[name]++
	}
	value, ok := e.values[name]
	return value, ok
}

func (e *startupEnvironment) start(t *testing.T) (*Gateway, string, error) {
	t.Helper()
	var logs bytes.Buffer
	gw, err := New(Options{LookupEnv: e.lookup, Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Registerer: prometheus.NewRegistry()})
	if gw != nil {
		t.Cleanup(gw.Close)
	}
	return gw, logs.String(), err
}

func unsetAmbientAgentObservabilityEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AGENTO11Y_ENDPOINT", "SIGIL_ENDPOINT", "AGENTO11Y_PROTOCOL", "SIGIL_PROTOCOL",
		"AGENTO11Y_INSECURE", "SIGIL_INSECURE", "AGENTO11Y_HEADERS", "SIGIL_HEADERS",
		"AGENTO11Y_AUTH_MODE", "SIGIL_AUTH_MODE", "AGENTO11Y_AUTH_TENANT_ID", "SIGIL_AUTH_TENANT_ID",
		"AGENTO11Y_AUTH_TOKEN", "SIGIL_AUTH_TOKEN", "AGENTO11Y_AGENT_NAME", "SIGIL_AGENT_NAME",
		"AGENTO11Y_AGENT_VERSION", "SIGIL_AGENT_VERSION", "AGENTO11Y_USER_ID", "SIGIL_USER_ID",
		"AGENTO11Y_TAGS", "SIGIL_TAGS", "AGENTO11Y_CONTENT_CAPTURE_MODE", "SIGIL_CONTENT_CAPTURE_MODE",
		"AGENTO11Y_DEBUG", "SIGIL_DEBUG", "AGENTO11Y_REDACT_INPUT_MESSAGES", "SIGIL_REDACT_INPUT_MESSAGES",
	} {
		t.Setenv(name, "")
	}
}

func TestNew_ValidatesBeforeSecrets(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		change  func(map[string]string)
		message string
	}{
		{name: "missing access mode", change: func(env map[string]string) { delete(env, "GRAFANA_AI_GATEWAY_ACCESS_MODE") }, message: "access mode is required"},
		{name: "unknown access mode", change: func(env map[string]string) { env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "hybrid" }, message: "cloud or self-hosted"},
		{name: "allowlist in self-hosted mode", change: func(env map[string]string) {
			env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "self-hosted"
			env["GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES"] = "svc-a"
		}, message: "requires cloud access mode"},
		{name: "production without signing keys", change: func(env map[string]string) { delete(env, "GRAFANA_JWKS") }, message: "GRAFANA_JWKS_URL or GRAFANA_JWKS"},
		{name: "production with insecure skip", change: func(env map[string]string) { env[EnvInsecureSkipAuthentication] = "true" }, message: "requires development mode"},
		{name: "development without keys or insecure skip", change: func(env map[string]string) {
			delete(env, "GRAFANA_JWKS")
			env["GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE"] = "development"
		}, message: "GRAFANA_JWKS_URL or GRAFANA_JWKS"},
		{name: "both signing key sources", change: func(env map[string]string) { env["GRAFANA_JWKS_URL"] = "https://auth.example/jwks" }, message: "only one"},
		{name: "scalar relationship", change: func(env map[string]string) {
			env["GRAFANA_AI_GATEWAY_PROVIDERWIRE_STREAM_IDLE_DURATION"] = "10m"
		}, message: "stream idle duration"},
		{name: "legacy listener environment is ignored, legacy flags are not accepted", change: func(env map[string]string) {
			env["GRAFANA_AI_GATEWAY_SERVER_PRIVATE_LISTEN_ADDRESS"] = "bad"
			delete(env, "GRAFANA_AI_GATEWAY_ACCESS_MODE")
		}, message: "access mode is required"},
		{name: "ambient agent observability environment", change: func(env map[string]string) {
			env["GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED"] = "true"
			env["GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT"] = "collector.example:4317"
			env["GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV"] = "AO_SECRET"
			env["SIGIL_TAGS"] = "private=ambient"
		}, message: "ambient agent observability SDK environment"},
		{name: "production cleartext provider", url: "http://provider.example", message: "production endpoint must use https"},
		{name: "credential-bearing provider endpoint", url: "https://user:password@provider.example", message: "userinfo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := tc.url
			if url == "" {
				url = "https://provider.example"
			}
			env := newStartupEnvironment(t, url)
			if tc.change != nil {
				tc.change(env.values)
			}
			_, logs, err := env.start(t)
			require.ErrorContains(t, err, tc.message)
			assert.Empty(t, env.secrets)
			for _, private := range []string{providerSecret, "private=ambient", "password", env.values["GRAFANA_AI_GATEWAY_CONFIG_FILE"]} {
				assert.NotContains(t, err.Error(), private)
				assert.NotContains(t, logs, private)
			}
		})
	}
}

func TestNew_RejectsInvalidRoutesBeforeSecrets(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{"invalid ID", "  public:", "  bad id:"},
		{"blank name", "name: Public", "name: ' '"},
		{"blank native model", "model: backend-private", "model: ' '"},
		{"duplicate candidate", "model: backend-private", "model: backend-private\n    fallback:\n      - provider: anthropic-primary\n        model: backend-private"},
		{"duplicate alias", "model: backend-private", "model: backend-private\n    aliases: [alias, alias]"},
		{"canonical alias collision", "model: backend-private", "model: backend-private\n    aliases: [public]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newStartupEnvironment(t, "https://provider.example")
			path := env.values["GRAFANA_AI_GATEWAY_CONFIG_FILE"]
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			changed := strings.Replace(string(data), tc.old, tc.replacement, 1)
			require.NotEqual(t, string(data), changed)
			require.NoError(t, os.WriteFile(path, []byte(changed), 0o600))
			_, _, err = env.start(t)
			require.ErrorContains(t, err, "config:")
			assert.Empty(t, env.secrets)
		})
	}
}

func TestNew_EnvironmentOnlyStartup(t *testing.T) {
	unsetAmbientAgentObservabilityEnvironment(t)
	var providerCalls int
	provider := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { providerCalls++ }))
	defer provider.Close()
	collector := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer collector.Close()

	tests := []struct {
		name   string
		change func(map[string]string)
	}{
		{name: "development with insecure skip", change: func(env map[string]string) {
			delete(env, "GRAFANA_JWKS")
			env[EnvInsecureSkipAuthentication] = "true"
		}},
		{name: "keys URL", change: func(env map[string]string) {
			delete(env, "GRAFANA_JWKS")
			env["GRAFANA_JWKS_URL"] = "http://auth.internal/jwks"
		}},
		{name: "self-hosted", change: func(env map[string]string) { env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "self-hosted" }},
		{name: "grpc agent observability", change: func(env map[string]string) {
			for name, value := range map[string]string{
				"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":          "true",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_PROTOCOL":         "grpc",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":         "127.0.0.1:1",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_TLS":              "false",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV":  "AO_SECRET",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_MAX_RETRIES":      "1",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_FLUSH_TIMEOUT":    "10ms",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_SHUTDOWN_TIMEOUT": "10ms",
				"AO_SECRET": "agent-secret-value",
			} {
				env[name] = value
			}
		}},
		{name: "http agent observability", change: func(env map[string]string) {
			for name, value := range map[string]string{
				"GRAFANA_AI_GATEWAY_AGENTO11Y_ENABLED":         "true",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_PROTOCOL":        "http",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_ENDPOINT":        collector.URL,
				"GRAFANA_AI_GATEWAY_AGENTO11Y_TLS":             "false",
				"GRAFANA_AI_GATEWAY_AGENTO11Y_AUTH_SECRET_ENV": "AO_SECRET",
				"AO_SECRET": "agent-secret-value",
			} {
				env[name] = value
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newStartupEnvironment(t, provider.URL)
			env.values["GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE"] = "development"
			tc.change(env.values)
			gw, logs, err := env.start(t)
			require.NoError(t, err)
			require.NotNil(t, gw.Handler())
			require.NotNil(t, gw.Authenticator())
			if _, enabled := env.values["AO_SECRET"]; enabled {
				assert.Equal(t, 1, env.secrets["AO_SECRET"])
			}
			for _, private := range []string{providerSecret, "agent-secret-value", "AO_SECRET", "127.0.0.1:1"} {
				assert.NotContains(t, logs, private)
			}
		})
	}
	assert.Zero(t, providerCalls)
}
