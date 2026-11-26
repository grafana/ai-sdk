package gateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const catalogChatPath = "/modelcatalogs/" + testCatalogName + "/v1/chat/completions"

func TestCatalogChat_QualifiedModelAndBlocklist(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	parent := testCatalogInfo(testCatalogParent(t, func(object map[string]any) {
		spec := object["spec"].(map[string]any)
		spec["providers"] = map[string]any{
			"claude": map[string]any{"type": "anthropic", "serveAllModels": true, "modelBlocklist": []string{"org/blocked"}},
		}
		model := spec["models"].(map[string]any)["balanced"].(map[string]any)
		model["aliases"] = []string{"claude/org/alias"}
	}))
	for _, tc := range []struct {
		name, selector, backend string
	}{
		{name: "unlisted model", selector: "claude/org/new", backend: "org/new"},
		{name: "blocked model", selector: "claude/org/blocked"},
		{name: "listed alias wins", selector: "claude/org/alias", backend: "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := native.requests.Load()
			body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_completion_tokens":16}`, tc.selector)
			request := callerRequest(bodyBYOKCaller, http.MethodPost, catalogChatPath, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(resource.WithRouteRequestInfo(request.Context(), parent))
			response := httptest.NewRecorder()
			h.gateway.Handler().ServeHTTP(response, request)
			assert.Zero(t, h.anthropic.requests.Load())
			if tc.backend == "" {
				assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), `"code":"model_not_found"`)
				assert.Equal(t, before, native.requests.Load())
				return
			}
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, before+1, native.requests.Load())
			keys, calls := native.nativeCalls()
			require.NotEmpty(t, keys)
			require.NotEmpty(t, calls)
			assert.Equal(t, string(parent.DecryptedSecureValues["claude"]), keys[len(keys)-1])
			assert.Contains(t, calls[len(calls)-1], `"model":"`+tc.backend+`"`)
		})
	}
}

func TestCatalogChat_ParentAccessAndErrors(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	for _, tc := range []struct {
		name   string
		caller bodyTestCaller
		parent func() *resource.RouteRequestInfo
		method string
		path   string
		status int
		code   string
		unread bool
	}{
		{name: "missing parent", caller: bodyBYOKCaller, status: 500, code: "internal_error", unread: true},
		{name: "mismatched parent", caller: bodyBYOKCaller, parent: func() *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, func(object map[string]any) {
				object["metadata"].(map[string]any)["name"] = "other"
			}))
		}, status: 500, code: "internal_error", unread: true},
		{name: "refused caller", caller: bodyRefusedCaller, parent: func() *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, nil))
		}, status: 403, code: "permission_denied", unread: true},
		{name: "missing decrypted key", caller: bodyBYOKCaller, parent: func() *resource.RouteRequestInfo {
			info := testCatalogInfo(testCatalogParent(t, nil))
			info.DecryptedSecureValues = nil
			return info
		}, status: 424, code: "failed_dependency"},
		{name: "disallowed endpoint", caller: bodyBYOKCaller, parent: func() *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, func(object map[string]any) {
				object["spec"].(map[string]any)["providers"].(map[string]any)["claude"] = map[string]any{"type": "anthropic", "baseURL": "https://private-endpoint.invalid"}
			}))
		}, status: 424, code: "failed_dependency"},
		{name: "operator-only model", caller: bodyBYOKCaller, parent: func() *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, nil))
		}, status: 404, code: "model_not_found"},
		{name: "unlisted endpoint", caller: bodyBYOKCaller, path: "/modelcatalogs/" + testCatalogName + "/chat-completions", status: 404, unread: true},
		{name: "undeclared method", caller: bodyBYOKCaller, method: http.MethodGet, status: http.StatusMethodNotAllowed, unread: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodPost
			}
			path := tc.path
			if path == "" {
				path = catalogChatPath
			}
			request := callerRequest(tc.caller, method, path, strings.NewReader(`{"model":"public","messages":[{"role":"user","content":"hi"}]}`))
			request.Header.Set("Content-Type", "application/json")
			if tc.parent != nil {
				request = request.WithContext(resource.WithRouteRequestInfo(request.Context(), tc.parent()))
			}
			counter := &countingBody{}
			if tc.unread {
				request.Body = io.NopCloser(counter)
			}
			response := httptest.NewRecorder()
			h.gateway.Handler().ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusMethodNotAllowed {
				assert.Equal(t, http.MethodPost, response.Header().Get("Allow"))
			}
			if tc.code != "" {
				assert.Contains(t, response.Body.String(), `"code":"`+tc.code+`"`)
				assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			} else {
				assert.NotContains(t, response.Body.String(), `"error"`)
			}
			if tc.unread {
				assert.Zero(t, counter.reads.Load())
			}
			for _, private := range []string{"private-endpoint.invalid", "catalog-key", "balanced", "claude-sonnet"} {
				assert.NotContains(t, response.Body.String(), private)
			}
			assert.Zero(t, native.requests.Load())
			assert.Zero(t, h.anthropic.requests.Load())
		})
	}
	assert.Contains(t, h.logs.String(), `"route":"catalog_chat_completions"`)
	assert.Contains(t, h.logs.String(), `"route":"unmatched"`)
}
