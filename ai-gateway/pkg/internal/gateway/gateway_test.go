package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginID_MatchesPluginMetadata(t *testing.T) {
	raw, err := os.ReadFile("../../../src/plugin.json")
	require.NoError(t, err)
	var metadata struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(raw, &metadata))
	assert.Equal(t, metadata.ID, PluginID)
}

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
			"GF_PLUGIN_CONFIG_FILE": writeModelConfig(t, providerURL, false),
			"GF_PLUGIN_ACCESS_MODE": "cloud",
			"GRAFANA_JWKS":          string(keys),
			"ANTHROPIC_SECRET":      providerSecret,
		},
		secrets: map[string]int{},
	}
}

func (e *startupEnvironment) lookup(name string) (string, bool) {
	if name == "ANTHROPIC_SECRET" || name == "GF_PLUGIN_AGENTO11Y_TOKEN" {
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
		ambient map[string]string
		message string
	}{
		{name: "missing access mode", change: func(env map[string]string) { delete(env, "GF_PLUGIN_ACCESS_MODE") }, message: "access mode is required"},
		{name: "unknown access mode", change: func(env map[string]string) { env["GF_PLUGIN_ACCESS_MODE"] = "hybrid" }, message: "cloud or self-hosted"},
		{name: "allowlist in self-hosted mode", change: func(env map[string]string) {
			env["GF_PLUGIN_ACCESS_MODE"] = "self-hosted"
			env["GF_PLUGIN_ACCESS_INTERNAL_SERVICES"] = "svc-a"
		}, message: "requires cloud access mode"},
		{name: "no signing keys", change: func(env map[string]string) { delete(env, "GRAFANA_JWKS") }, message: "GRAFANA_JWKS_URL or GRAFANA_JWKS"},
		{name: "no signing keys with insecure skip", change: func(env map[string]string) {
			delete(env, "GRAFANA_JWKS")
			env[plugin.EnvVarInsecureSkipAuthentication] = "true"
		}, message: "GRAFANA_JWKS_URL or GRAFANA_JWKS"},
		{name: "both signing key sources", change: func(env map[string]string) { env["GRAFANA_JWKS_URL"] = "https://auth.example/jwks" }, message: "only one"},
		{name: "unparsable scalar", change: func(env map[string]string) {
			env["GF_PLUGIN_AGENTO11Y_TLS"] = "password"
		}, message: "GF_PLUGIN_AGENTO11Y_TLS has an invalid value"},
		{name: "legacy settings are ignored", change: func(env map[string]string) {
			env["GRAFANA_AI_GATEWAY_SERVER_PRIVATE_LISTEN_ADDRESS"] = "bad"
			env["GRAFANA_AI_GATEWAY_ACCESS_MODE"] = "cloud"
			delete(env, "GF_PLUGIN_ACCESS_MODE")
		}, message: "access mode is required"},
		{name: "ambient agent observability environment", change: func(env map[string]string) {
			env["GF_PLUGIN_AGENTO11Y_ENABLED"] = "true"
			env["GF_PLUGIN_AGENTO11Y_ENDPOINT"] = "collector.example:4317"
		}, ambient: map[string]string{"SIGIL_TAGS": "private=ambient"}, message: "ambient agent observability SDK environment"},
		{name: "unsupported provider scheme", url: "ftp://provider.example", message: "must use http or https"},
		{name: "credential-bearing provider endpoint", url: "https://user:password@provider.example", message: "userinfo"},
		{name: "malformed credential-bearing provider endpoint", url: "https://user:password@provider.example:port", message: "not a valid URL"},
		{name: "missing configuration file", change: func(env map[string]string) {
			env["GF_PLUGIN_CONFIG_FILE"] += ".missing"
		}, message: "file does not exist"},
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
			for name, value := range tc.ambient {
				t.Setenv(name, value)
			}
			_, logs, err := env.start(t)
			require.ErrorContains(t, err, tc.message)
			assert.Empty(t, env.secrets)
			for _, private := range []string{providerSecret, "private=ambient", "password", env.values["GF_PLUGIN_CONFIG_FILE"]} {
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
			path := env.values["GF_PLUGIN_CONFIG_FILE"]
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
		{name: "keys URL", change: func(env map[string]string) {
			delete(env, "GRAFANA_JWKS")
			env["GRAFANA_JWKS_URL"] = "http://auth.internal/jwks"
		}},
		{name: "self-hosted", change: func(env map[string]string) { env["GF_PLUGIN_ACCESS_MODE"] = "self-hosted" }},
		{name: "grpc agent observability", change: func(env map[string]string) {
			for name, value := range map[string]string{
				"GF_PLUGIN_AGENTO11Y_ENABLED":  "true",
				"GF_PLUGIN_AGENTO11Y_PROTOCOL": "grpc",
				"GF_PLUGIN_AGENTO11Y_ENDPOINT": "127.0.0.1:1",
				"GF_PLUGIN_AGENTO11Y_TLS":      "false",
				"GF_PLUGIN_AGENTO11Y_TOKEN":    "agent-secret-value",
			} {
				env[name] = value
			}
		}},
		{name: "http agent observability", change: func(env map[string]string) {
			for name, value := range map[string]string{
				"GF_PLUGIN_AGENTO11Y_ENABLED":  "true",
				"GF_PLUGIN_AGENTO11Y_PROTOCOL": "http",
				"GF_PLUGIN_AGENTO11Y_ENDPOINT": collector.URL,
				"GF_PLUGIN_AGENTO11Y_TLS":      "false",
				"GF_PLUGIN_AGENTO11Y_TOKEN":    "agent-secret-value",
			} {
				env[name] = value
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newStartupEnvironment(t, provider.URL)
			tc.change(env.values)
			gw, logs, err := env.start(t)
			require.NoError(t, err)
			require.NotNil(t, gw.Handler())
			require.NotNil(t, gw.Authenticator())
			if _, enabled := env.values["GF_PLUGIN_AGENTO11Y_TOKEN"]; enabled {
				assert.Equal(t, 1, env.secrets["GF_PLUGIN_AGENTO11Y_TOKEN"])
			}
			for _, private := range []string{providerSecret, "agent-secret-value", "127.0.0.1:1"} {
				assert.NotContains(t, logs, private)
			}
		})
	}
	assert.Zero(t, providerCalls)
}

func TestNew_ProductionExportWithDefaultTimeoutsClosesWithinTheBudget(t *testing.T) {
	unsetAmbientAgentObservabilityEnvironment(t)
	env := newStartupEnvironment(t, "https://provider.example")
	for name, value := range map[string]string{
		"GF_PLUGIN_AGENTO11Y_ENABLED":  "true",
		"GF_PLUGIN_AGENTO11Y_ENDPOINT": "collector.invalid:4317",
		"GF_PLUGIN_AGENTO11Y_TOKEN":    "agent-secret-value",
	} {
		env.values[name] = value
	}
	gw, _, err := env.start(t)
	require.NoError(t, err)
	started := time.Now()
	gw.Close()
	assert.Less(t, time.Since(started), finalFlushBudget+500*time.Millisecond)
}

func buildPlugin(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the plugin executable")
	}
	binary := filepath.Join(t.TempDir(), "gpx_aigateway")
	command := exec.Command("go", "build", "-o", binary, ".")
	command.Dir = "../.."
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

type pluginProcess struct {
	host    *platformhost.Host
	process *platformhost.PluginProcess
	server  *httptest.Server
	logs    *syncBuffer
	raw     *syncBuffer
	fake    *fakeAnthropic
}

func launchPlugin(t *testing.T, binary string, env map[string]string) *pluginProcess {
	t.Helper()
	data := manifestdata.LocalManifest().ManifestData
	host, err := platformhost.New(platformhost.Config{Topology: platformhost.TopologyCloud, PluginID: PluginID, Group: data.Group, Version: data.PreferredVersion})
	require.NoError(t, err)
	fake := newFakeAnthropic(t)
	environment := []string{
		"GF_PLUGIN_CONFIG_FILE=" + writeModelConfig(t, fake.server.URL, false),
		"GF_PLUGIN_ACCESS_MODE=cloud",
		"GF_PLUGIN_ACCESS_INTERNAL_SERVICES=" + testInternalService,
		"ANTHROPIC_SECRET=" + providerSecret,
	}
	for name, value := range env {
		environment = append(environment, name+"="+value)
	}
	logs, raw := &syncBuffer{}, &syncBuffer{}
	process, err := host.LaunchPlugin(platformhost.PluginConfig{
		Binary: binary,
		Env:    environment,
		Logger: hclog.New(&hclog.LoggerOptions{Name: "plugin", Level: hclog.Info, Output: logs, JSONFormat: true}),
		Stderr: raw,
	})
	require.NoError(t, err)
	server := httptest.NewServer(host.Handler(process.RouteClient()))
	t.Cleanup(func() {
		server.Close()
		process.Stop()
	})
	return &pluginProcess{host: host, process: process, server: server, logs: logs, raw: raw, fake: fake}
}

func (p *pluginProcess) url(route string) string {
	return p.server.URL + strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/" + route
}

func TestPluginProcess_ServesThroughThePluginProtocol(t *testing.T) {
	p := launchPlugin(t, buildPlugin(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, message, err := p.process.CheckHealth(ctx)
	require.NoError(t, err)
	assert.Equal(t, "OK", status)
	assert.NotEmpty(t, message)

	token, err := p.host.ServiceToken(testInternalService, testNamespace)
	require.NoError(t, err)
	code, body := do(t, languageRequest(t, ctx, p.url("language-model"), unaryBody, false, accessToken(token)))
	assert.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, "hello")

	metrics, err := p.process.CollectMetrics(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(metrics), `grafana_ai_gateway_access_decisions_total{decision="configured"} 1`)
	assert.NotContains(t, string(metrics), providerSecret)

	assert.Less(t, p.process.Stop(), 2*time.Second)
}

func TestPluginProcess_LogsThroughTheHostProtocol(t *testing.T) {
	binary := buildPlugin(t)
	const (
		traceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
		parentSpanID = "00f067aa0ba902b7"
	)
	for _, tc := range []struct {
		name   string
		env    map[string]string
		traced bool
	}{
		{name: "host tracing configured", env: map[string]string{"GF_INSTANCE_OTLP_ADDRESS": "127.0.0.1:1", "GF_INSTANCE_OTLP_PROPAGATION": "w3c"}, traced: true},
		{name: "no tracing configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := launchPlugin(t, binary, tc.env)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			token, err := p.host.ServiceToken(testInternalService, testNamespace)
			require.NoError(t, err)
			request := languageRequest(t, ctx, p.url("language-model"), unaryBody, false, accessToken(token))
			request.Header.Set("traceparent", "00-"+traceID+"-"+parentSpanID+"-01")
			code, body := do(t, request)
			require.Equal(t, http.StatusOK, code, body)
			p.process.Stop()

			var completed map[string]any
			for _, line := range strings.Split(strings.TrimSpace(p.raw.String()), "\n") {
				var record map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &record), "plugin wrote outside the log protocol: %s", line)
				require.Contains(t, record, "@level", line)
				if record["@message"] == "http request completed" {
					completed = record
				}
			}
			require.NotNil(t, completed)
			assert.Equal(t, "info", completed["@level"])
			assert.Equal(t, "language_model", completed["route"])
			assert.Contains(t, p.logs.String(), `"@message":"http request completed"`)
			assert.Equal(t, traceID, completed["traceID"])
			if tc.traced {
				assert.NotEqual(t, parentSpanID, completed["spanID"], "the host-configured tracer creates the server span")
			} else {
				assert.Equal(t, parentSpanID, completed["spanID"], "without tracing configuration no local span is created")
			}
		})
	}
}
