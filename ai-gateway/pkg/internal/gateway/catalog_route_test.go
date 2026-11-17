package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const testCatalogName = "team"

func testCatalogParent(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	object := map[string]any{
		"apiVersion": v1alpha1.GroupVersion.Identifier(),
		"kind":       "ModelCatalog",
		"metadata":   map[string]any{"namespace": testNamespace, "name": testCatalogName, "resourceVersion": "7"},
		"spec": map[string]any{
			"providers": map[string]any{"claude": map[string]any{"type": "anthropic"}},
			"models":    map[string]any{"balanced": map[string]any{"name": "Balanced", "primary": map[string]any{"provider": "claude", "model": "claude-sonnet"}}},
		},
		"secure": map[string]any{"claude": map[string]any{"name": "inline-claude"}},
	}
	if mutate != nil {
		mutate(object)
	}
	raw, err := json.Marshal(object)
	require.NoError(t, err)
	return raw
}

func testCatalogInfo(parent []byte) *resource.RouteRequestInfo {
	return &resource.RouteRequestInfo{
		FullIdentifier:        resource.FullIdentifier{Group: v1alpha1.APIGroup, Version: v1alpha1.APIVersion, Namespace: testNamespace, Plural: "modelcatalogs", Name: testCatalogName},
		ResourceVersion:       "7",
		Parent:                parent,
		DecryptedSecureValues: resource.DecryptedSecureValues{"claude": "catalog-key"},
	}
}

func TestParentCatalog(t *testing.T) {
	metadata := func(key, value string) func(map[string]any) {
		return func(object map[string]any) { object["metadata"].(map[string]any)[key] = value }
	}
	for _, tc := range []struct {
		name   string
		info   func(t *testing.T) *resource.RouteRequestInfo
		path   string
		status int
		stack  *service.StackCatalog
	}{
		{name: "matching parent", info: func(t *testing.T) *resource.RouteRequestInfo { return testCatalogInfo(testCatalogParent(t, nil)) }, status: http.StatusNoContent, stack: &service.StackCatalog{
			Name: testCatalogName,
			Spec: v1alpha1.Spec{
				Providers: map[string]v1alpha1.Provider{"claude": {Type: "anthropic"}},
				Models:    map[string]v1alpha1.Model{"balanced": {Name: "Balanced", Primary: v1alpha1.Target{Provider: "claude", Model: "claude-sonnet"}}},
			},
			Keys: resource.DecryptedSecureValues{"claude": "catalog-key"},
		}},
		{name: "spec that does not decode", info: func(t *testing.T) *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, func(object map[string]any) { object["spec"] = "not a spec" }))
		}, status: http.StatusNoContent, stack: &service.StackCatalog{Name: testCatalogName}},
		{name: "no route info", info: func(*testing.T) *resource.RouteRequestInfo { return nil }, status: http.StatusInternalServerError},
		{name: "no parent", info: func(*testing.T) *resource.RouteRequestInfo { return testCatalogInfo(nil) }, status: http.StatusInternalServerError},
		{name: "another plural", info: func(t *testing.T) *resource.RouteRequestInfo {
			info := testCatalogInfo(testCatalogParent(t, nil))
			info.Plural = "dashboards"
			return info
		}, status: http.StatusInternalServerError},
		{name: "another group", info: func(t *testing.T) *resource.RouteRequestInfo {
			info := testCatalogInfo(testCatalogParent(t, nil))
			info.Group = "other.ext.grafana.app"
			return info
		}, status: http.StatusInternalServerError},
		{name: "info names another catalog", info: func(t *testing.T) *resource.RouteRequestInfo {
			info := testCatalogInfo(testCatalogParent(t, nil))
			info.Name = "other"
			return info
		}, status: http.StatusInternalServerError},
		{name: "info names another namespace", info: func(t *testing.T) *resource.RouteRequestInfo {
			info := testCatalogInfo(testCatalogParent(t, nil))
			info.Namespace = "stacks-2"
			return info
		}, status: http.StatusInternalServerError},
		{name: "parent names another catalog", info: func(t *testing.T) *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, metadata("name", "other")))
		}, status: http.StatusInternalServerError},
		{name: "parent from another namespace", info: func(t *testing.T) *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, metadata("namespace", "stacks-2")))
		}, status: http.StatusInternalServerError},
		{name: "parent of another kind", info: func(t *testing.T) *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, func(object map[string]any) { object["kind"] = "Dashboard" }))
		}, status: http.StatusInternalServerError},
		{name: "parent of another version", info: func(t *testing.T) *resource.RouteRequestInfo {
			return testCatalogInfo(testCatalogParent(t, func(object map[string]any) { object["apiVersion"] = v1alpha1.APIGroup + "/v2" }))
		}, status: http.StatusInternalServerError},
		{name: "parent that is not JSON", info: func(*testing.T) *resource.RouteRequestInfo { return testCatalogInfo([]byte("not json")) }, status: http.StatusInternalServerError},
		{name: "path names another catalog", info: func(t *testing.T) *resource.RouteRequestInfo { return testCatalogInfo(testCatalogParent(t, nil)) }, path: "other", status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var served *service.StackCatalog
			mux := http.NewServeMux()
			mux.Handle("POST /namespaces/{namespace}/modelcatalogs/{name}/language-model", parentCatalog(providerv4.NewHostErrorWriter(), http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				stack, ok := service.StackCatalogFromContext(request.Context())
				require.True(t, ok)
				served = &stack
				w.WriteHeader(http.StatusNoContent)
			})))
			name := tc.path
			if name == "" {
				name = testCatalogName
			}
			body := &countingBody{}
			request := httptest.NewRequest(http.MethodPost, "/namespaces/"+testNamespace+"/modelcatalogs/"+name+"/language-model", body)
			if info := tc.info(t); info != nil {
				request = request.WithContext(resource.WithRouteRequestInfo(request.Context(), info))
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.stack, served)
			if tc.status == http.StatusInternalServerError {
				assert.Zero(t, body.reads.Load())
				assert.JSONEq(t, `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`, response.Body.String())
				assert.NotContains(t, response.Body.String(), "catalog-key")
			}
		})
	}
}
