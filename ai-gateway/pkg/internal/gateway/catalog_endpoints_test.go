package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	endpointCatalogKey   = "sentinel-endpoint-catalog-key"
	compatibleCatalogKey = "sentinel-compatible-catalog-key"
	byokCredentialsError = "providerOptions.gateway.byok requires supported provider arrays with valid accounts"
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
	code, body := sendObject(t, context.Background(), method, catalogURL(h.server.URL, name), h.serviceToken(t, testInternalService), document)
	require.Contains(t, []int{http.StatusCreated, http.StatusOK}, code, body)
}

func TestCatalogRoutes_ListedEndpoints(t *testing.T) {
	anthropic := newTLSFakeAnthropic(t)
	compatible := newFakeCompatible(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{env: map[string]string{"GF_PLUGIN_CUSTOM_ENDPOINTS": anthropic.server.URL + "/team," + compatible.server.URL + "/v1"}})
	h.saveCatalog(t, http.MethodPost, endpointCatalog(t, map[string]any{
		"claude": map[string]any{"type": "anthropic", "baseURL": anthropic.server.URL + "/team"},
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

	t.Run("an endpoint outside the list is refused when saved", func(t *testing.T) {
		for _, endpoint := range []string{anthropic.server.URL + "/teams", anthropic.server.URL, strings.Replace(anthropic.server.URL, "https://", "http://", 1) + "/team", "https://localhost:" + strings.TrimPrefix(anthropic.server.URL, "https://127.0.0.1:") + "/team"} {
			code, body := sendObject(t, context.Background(), http.MethodPut, catalogURL(h.server.URL, testCatalogName), h.serviceToken(t, testInternalService), endpointCatalog(t, map[string]any{
				"claude": map[string]any{"type": "anthropic", "baseURL": endpoint},
			}, map[string]any{"claude-model": map[string]any{"name": "Claude", "primary": target("claude", "claude-sonnet")}}))
			assert.Equal(t, http.StatusBadRequest, code, endpoint)
			assert.Contains(t, body, "modelcatalog: providers.claude.baseURL is not a listed custom endpoint")
			assert.NotContains(t, body, "127.0.0.1")
		}
	})
}

func byokRequest(t *testing.T, h *harness, accounts []any, streaming bool, auth credential) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"prompt":          []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}}}},
		"maxOutputTokens": 16,
		"providerOptions": map[string]any{"gateway": map[string]any{"byok": map[string]any{"anthropic": accounts}}},
	})
	require.NoError(t, err)
	return catalogLanguageRequest(t, h.url("language-model"), "anthropic/claude-request", string(body), streaming, auth)
}

func account(baseURL string) map[string]any {
	return map[string]any{"apiKey": endpointCatalogKey, "baseURL": baseURL}
}

func TestBYOK_ListedEndpoints(t *testing.T) {
	custom := newTLSFakeAnthropic(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{env: map[string]string{"GF_PLUGIN_CUSTOM_ENDPOINTS": custom.server.URL + "/team"}})
	user := bearer(h.host.AddServiceAccount("private-account-uid", testNamespace))

	t.Run("a listed endpoint serves unary and streamed calls with the account key", func(t *testing.T) {
		for _, streaming := range []bool{false, true} {
			status, body := do(t, byokRequest(t, h, []any{account(custom.server.URL + "/team/")}, streaming, user))
			require.Equal(t, http.StatusOK, status, body)
			assert.Contains(t, body, map[bool]string{false: "hello", true: "first"}[streaming])
		}
		keys, bodies := custom.nativeCalls()
		assert.Equal(t, []string{endpointCatalogKey, endpointCatalogKey}, keys)
		assert.Contains(t, bodies[0], `"model":"claude-request"`)
	})
	t.Run("an unlisted endpoint fails at decode with the BYOK credentials document", func(t *testing.T) {
		before := custom.requests.Load()
		port := strings.TrimPrefix(custom.server.URL, "https://127.0.0.1:")
		for _, accounts := range [][]any{
			{account(custom.server.URL + "/teams")},
			{account("http://127.0.0.1:" + port + "/team")},
			{account("https://localhost:" + port + "/team")},
			{account("https://169.254.169.254")},
			{account(custom.server.URL + "/team"), account("https://169.254.169.254")},
		} {
			status, body := do(t, byokRequest(t, h, accounts, false, user))
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Contains(t, body, byokCredentialsError)
			for _, private := range []string{"127.0.0.1", "localhost", "169.254", endpointCatalogKey} {
				assert.NotContains(t, body, private)
			}
		}
		assert.Equal(t, before, custom.requests.Load(), "a refused call sends no request, even to a listed account")
	})
	t.Run("an unlisted endpoint of an unused provider is refused", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{
			"prompt": []any{}, "providerOptions": map[string]any{"gateway": map[string]any{"byok": map[string]any{
				"anthropic": []any{account(custom.server.URL + "/team")},
				"openai":    []any{account("https://169.254.169.254/v1")},
			}}},
		})
		require.NoError(t, err)
		status, response := do(t, catalogLanguageRequest(t, h.url("language-model"), "anthropic/claude-request", string(body), false, user))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, response, byokCredentialsError)
	})
	assert.NotContains(t, h.logs.String(), endpointCatalogKey)
}

func TestCustomEndpoints_OperatorCatalogIsNotChecked(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	status, body := do(t, catalogLanguageRequest(t, h.url("language-model"), "public", unaryBody, false, accessToken(h.serviceToken(t, testInternalService))))
	require.Equal(t, http.StatusOK, status, body)
	assert.EqualValues(t, 1, h.anthropic.requests.Load(), "the operator catalog reaches its unlisted plain-HTTP loopback endpoint")
}
