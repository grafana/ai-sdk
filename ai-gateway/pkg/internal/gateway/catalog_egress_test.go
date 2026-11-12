package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	endpointCatalogKey   = "sentinel-endpoint-catalog-key"
	compatibleCatalogKey = "sentinel-compatible-catalog-key"
	egressError          = `{"message":"provider endpoint refused by the egress policy or not resolvable","type":"failed_dependency","param":null,"code":"failed_dependency"}`
)

// fakeCompatible is an openai-compatible backend over TLS on 127.0.0.1.
type fakeCompatible struct {
	server *httptest.Server
	mu     sync.Mutex
	keys   []string
	paths  []string
}

func newFakeCompatible(t *testing.T) *fakeCompatible {
	t.Helper()
	fake := &fakeCompatible{}
	fake.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.keys = append(fake.keys, r.Header.Get("Authorization"))
		fake.paths = append(fake.paths, r.URL.Path)
		fake.mu.Unlock()
		if !strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chatcmpl_1","created":1710000000,"model":"llama","choices":[{"message":{"role":"assistant","content":"compatible hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl_2","created":1710000000,"model":"llama","choices":[{"delta":{"role":"assistant","content":"compatible "},"finish_reason":null}]}`+"\n\n"+
			`data: {"id":"chatcmpl_2","created":1710000000,"model":"llama","choices":[{"delta":{"content":"stream"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`+"\n\n"+
			"data: [DONE]\n\n")
	}))
	fake.server.TLS = &tls.Config{Certificates: []tls.Certificate{endpointCertificate}}
	fake.server.StartTLS()
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeCompatible) calls() (keys, paths []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.keys...), append([]string(nil), f.paths...)
}

// endpointCatalog is catalog `team` with the given providers and models, and
// a key for each provider.
func endpointCatalog(t *testing.T, providers map[string]any, models map[string]any) string {
	t.Helper()
	secure := map[string]any{}
	for name := range providers {
		secure[name] = map[string]any{"create": endpointCatalogKey}
	}
	if _, ok := providers["local"]; ok {
		secure["local"] = map[string]any{"create": compatibleCatalogKey}
	}
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": testCatalogName},
		"spec":     map[string]any{"providers": providers, "models": models},
		"secure":   secure,
	})
	require.NoError(t, err)
	return string(body)
}

func target(provider, model string) map[string]any {
	return map[string]any{"provider": provider, "model": model}
}

func (h *harness) saveCatalog(t *testing.T, method, document string) {
	t.Helper()
	name := ""
	if method == http.MethodPut {
		name = testCatalogName
	}
	code, body := sendCatalog(t, context.Background(), method, catalogURL(h.server.URL, name), h.serviceToken(t, testInternalService), document)
	require.Contains(t, []int{http.StatusCreated, http.StatusOK}, code, body)
}

func TestCatalogRoutes_CustomEndpoints(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{env: map[string]string{"GF_PLUGIN_CUSTOM_ENDPOINT_ALLOW": "127.0.0.1"}})
	anthropic := newTLSFakeAnthropic(t)
	compatible := newFakeCompatible(t)
	h.saveCatalog(t, http.MethodPost, endpointCatalog(t, map[string]any{
		"claude": map[string]any{"type": "anthropic", "baseURL": anthropic.server.URL},
		"local":  map[string]any{"type": "openai-compatible", "baseURL": compatible.server.URL + "/v1", "providerName": "acme", "supportsStructuredOutputs": true},
	}, map[string]any{
		"claude-model": map[string]any{"name": "Claude", "primary": target("claude", "claude-sonnet")},
		"local-model":  map[string]any{"name": "Local", "primary": target("local", "llama")},
	}))
	user := session(h.host.AddSession(types.TypeUser, "private-user-uid", testNamespace))

	status, body := do(t, discoveryRequest(t, h.catalogURL("config"), user))
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"primary":{"providerInstance":"local","provider":"acme","providerModelId":"llama"}`)
	assert.NotContains(t, body, "127.0.0.1")

	for _, streaming := range []bool{false, true} {
		requestBody := unaryBody
		if streaming {
			requestBody = streamedCatalogBody
		}
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "claude-model", requestBody, streaming, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, map[bool]string{false: "hello", true: "first"}[streaming])
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "local-model", requestBody, streaming, user))
		require.Equal(t, http.StatusOK, status, body)
		assert.Contains(t, body, map[bool]string{false: "compatible hello", true: "stream"}[streaming])
	}
	keys, _ := anthropic.nativeCalls()
	assert.Equal(t, []string{endpointCatalogKey, endpointCatalogKey}, keys)
	compatibleKeys, paths := compatible.calls()
	assert.Equal(t, []string{"Bearer " + compatibleCatalogKey, "Bearer " + compatibleCatalogKey}, compatibleKeys)
	assert.Equal(t, []string{"/v1/chat/completions", "/v1/chat/completions"}, paths)
	assert.Zero(t, h.anthropic.requests.Load(), "the operator catalog's provider is not used")
}

func TestCatalogRoutes_RefusedEndpoints(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{tlsOperator: true})
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	operator, err := url.Parse(h.anthropic.server.URL)
	require.NoError(t, err)
	refused := "https://localhost:" + operator.Port()
	h.saveCatalog(t, http.MethodPost, endpointCatalog(t, map[string]any{
		"claude": map[string]any{"type": "anthropic", "baseURL": refused},
		"native": map[string]any{"type": "anthropic"},
	}, map[string]any{
		"refused":   map[string]any{"name": "Refused", "primary": target("claude", "claude-sonnet")},
		"fallsback": map[string]any{"name": "Falls back", "primary": target("claude", "claude-sonnet"), "fallback": []any{target("native", "claude-native")}},
	}))
	user := session(h.host.AddSession(types.TypeUser, "private-user-uid", testNamespace))

	status, body := do(t, catalogLanguageRequest(t, h.url("language-model"), "public", unaryBody, false, accessToken(h.serviceToken(t, testInternalService))))
	require.Equal(t, http.StatusOK, status, body)
	require.EqualValues(t, 1, h.anthropic.requests.Load(), "the operator catalog reaches https://localhost, and keeps the connection idle")

	for _, streaming := range []bool{false, true} {
		requestBody := map[bool]string{false: unaryBody, true: streamedCatalogBody}[streaming]
		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "refused", requestBody, streaming, user))
		assert.Equal(t, http.StatusFailedDependency, status)
		var document struct {
			Error json.RawMessage `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &document), body)
		assert.JSONEq(t, egressError, string(document.Error))
		assert.Contains(t, body, `"error":{"message":"`+execution.ErrEgressRefused.Error()+`"}`, "the overview carries the fixed refusal")
		for _, private := range []string{"localhost", operator.Port(), endpointCatalogKey} {
			assert.NotContains(t, body, private)
		}

		status, body = do(t, catalogLanguageRequest(t, h.catalogURL("language-model"), "fallsback", requestBody, streaming, user))
		require.Equal(t, http.StatusOK, status, body)
		if !streaming {
			assert.Contains(t, body, execution.ErrEgressRefused.Error(), "the overview carries the fixed refusal")
		}
		for _, private := range []string{"localhost", operator.Port(), endpointCatalogKey} {
			assert.NotContains(t, body, private)
		}
	}
	assert.EqualValues(t, 1, h.anthropic.requests.Load(), "the refused endpoint receives no request, even with an idle operator connection to it")
	assert.EqualValues(t, 2, native.requests.Load())
	logs := h.logs.String()
	for _, private := range []string{"localhost:" + operator.Port(), endpointCatalogKey} {
		assert.NotContains(t, logs, private)
	}
}
