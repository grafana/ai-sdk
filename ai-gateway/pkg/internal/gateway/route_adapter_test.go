package gateway

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/testadapter"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/discovery"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const (
	testAdapterPath  = "/test-adapter"
	testAdapterLabel = "test_adapter"
	anthropicHost    = "anthropic.adapter.test"
	openAIHost       = "openai.adapter.test"
	openAIResponse   = `{"id":"resp_1","object":"response","status":"completed","model":"gpt","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
)

type adapterNativeRequest struct{ host, target string }

// adapterHarness serves the production routes plus the test adapter on an
// extra entry, over one shared selector and runtime. Anthropic always fails
// retryably, and OpenAI succeeds.
type adapterHarness struct {
	handler http.Handler
	logs    syncBuffer
	mu      sync.Mutex
	native  []adapterNativeRequest
}

func newAdapterHarness(t *testing.T) *adapterHarness {
	t.Helper()
	h := &adapterHarness{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		h.mu.Lock()
		h.native = append(h.native, adapterNativeRequest{host: r.URL.Host, target: r.Header.Get("X-Target")})
		h.mu.Unlock()
		status, body := http.StatusOK, openAIResponse
		if strings.HasSuffix(r.URL.Path, "/messages") {
			status, body = http.StatusServiceUnavailable, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	file := config.File{Models: map[string]config.Model{
		"grafana/chain": {Name: "Chain", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude"}, Fallback: []config.Primary{{Provider: "openai-backup", Model: "gpt"}}},
	}}
	providers := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "configured-anthropic", BaseURL: "https://" + anthropicHost},
		"openai-backup":     {Type: "openai", APIKey: "configured-openai", BaseURL: "https://" + openAIHost + "/v1"},
	}
	unwrapped := func(_ string, lower provider.LanguageModel) (provider.LanguageModel, error) { return lower, nil }
	models, err := service.BuildCatalog(file, providers, client, unwrapped)
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: service.NewSelector(models, &service.BYOKOptions{Client: client, Factory: unwrapped}), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	language, err := providerv4.New(runtime)
	require.NoError(t, err)
	translator := func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		options.Headers = map[string]string{"X-Target": string(target.Type) + "/" + target.ModelID}
		return options, nil
	}
	routes := append(productionRoutes(discovery.New(models, providerv4.DiscoveryProjection{}, providerv4.NewHostErrorWriter()), language), route{
		method: http.MethodPost, path: testAdapterPath, label: testAdapterLabel,
		handler: testadapter.New(runtime, translator), errors: testadapter.HostErrors{},
	})
	telemetry, err := service.NewTelemetry(slog.New(slog.NewJSONHandler(&h.logs, nil)), service.TelemetryOptions{Registerer: prometheus.NewRegistry()})
	require.NoError(t, err)
	policy := gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeCloud, InternalServices: []string{testInternalService}}
	h.handler, err = newHandler(policy, telemetry, routes)
	require.NoError(t, err)
	return h
}

func (h *adapterHarness) serve(caller bodyTestCaller, model string, body io.Reader) *httptest.ResponseRecorder {
	request := callerRequest(caller, http.MethodPost, testAdapterPath, body)
	request.Header.Set(testadapter.HeaderModel, model)
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)
	return response
}

func (h *adapterHarness) nativeRequests() []adapterNativeRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]adapterNativeRequest(nil), h.native...)
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTestAdapterRoute_RefusalUsesTheAdapterFormat(t *testing.T) {
	h := newAdapterHarness(t)
	body := &countingBody{}
	response := h.serve(bodyRefusedCaller, "grafana/chain", body)
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "host-error permission\n", response.Body.String())
	assert.Zero(t, body.reads.Load())
	assert.Empty(t, h.nativeRequests())
	assert.Contains(t, h.logs.String(), `"route":"`+testAdapterLabel+`"`)
}

func TestTestAdapterRoute_ConfiguredFallbackIsTranslatedPerAttempt(t *testing.T) {
	h := newAdapterHarness(t)
	response := h.serve(bodyConfiguredCaller, "grafana/chain", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "outcome category=0 reason=0 committed=false body=2\n", response.Body.String())
	assert.Equal(t, []adapterNativeRequest{{host: anthropicHost, target: "anthropic/claude"}, {host: openAIHost, target: "openai/gpt"}}, h.nativeRequests())
	assert.Contains(t, h.logs.String(), `"route":"`+testAdapterLabel+`"`)
	assert.Contains(t, h.logs.String(), `"access":"configured"`)
}

func TestTestAdapterRoute_BYOKUsesTheSharedSelector(t *testing.T) {
	h := newAdapterHarness(t)
	body := `{"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"dummy-request-key"}]}}}}`
	response := h.serve(bodyBYOKCaller, "openai/gpt", strings.NewReader(body))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, fmt.Sprintf("outcome category=0 reason=0 committed=false body=%d\n", len(body)), response.Body.String())
	assert.Equal(t, []adapterNativeRequest{{host: "api.openai.com", target: "openai/gpt"}}, h.nativeRequests())

	configured := h.serve(bodyBYOKCaller, "grafana/chain", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, configured.Code)
	assert.Contains(t, configured.Body.String(), fmt.Sprintf("outcome category=%d ", execution.CategoryBYOKSelector))
	assert.Len(t, h.nativeRequests(), 1)
}

func TestTestAdapterRoute_ReceivesTheWholeBody(t *testing.T) {
	h := newAdapterHarness(t)
	body := `{}` + strings.Repeat(" ", 2<<20)
	response := h.serve(bodyConfiguredCaller, "grafana/chain", strings.NewReader(body))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.HasSuffix(response.Body.String(), fmt.Sprintf(" body=%d\n", len(body))), response.Body.String())
}

func TestTestAdapter_IsNotLinkedIntoThePlugin(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	var stdout, stderr bytes.Buffer
	command := exec.Command("go", "list", "-deps", "github.com/grafana/ai-sdk/ai-gateway/pkg")
	command.Stdout, command.Stderr = &stdout, &stderr
	require.NoError(t, command.Run(), stderr.String())
	assert.Contains(t, stdout.String(), "github.com/grafana/ai-sdk/ai-gateway/internal/execution\n")
	assert.NotContains(t, stdout.String(), "github.com/grafana/ai-sdk/ai-gateway/internal/testadapter")
}
