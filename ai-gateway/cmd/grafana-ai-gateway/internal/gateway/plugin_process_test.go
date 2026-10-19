package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

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
		"GRAFANA_AI_GATEWAY_CONFIG_FILE=" + writeModelConfig(t, fake.server.URL),
		"GRAFANA_AI_GATEWAY_DEPLOYMENT_MODE=development",
		"GRAFANA_AI_GATEWAY_ACCESS_MODE=cloud",
		"GRAFANA_AI_GATEWAY_ACCESS_INTERNAL_SERVICES=" + testInternalService,
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
	assert.Contains(t, string(metrics), `grafana_ai_gateway_http_requests_total{method="POST",route="language_model",status="2xx"} 1`)
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
