package messages

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	"github.com/stretchr/testify/assert"
)

var _ host.ErrorWriter = (*HostErrorWriter)(nil)

func TestErrors_CategoryStatus(t *testing.T) {
	for _, tc := range []struct {
		name     string
		category execution.Category
		status   int
		typ      apiErrorType
		message  string
	}{
		{"invalid request", execution.CategoryInvalidRequest, 400, apiErrorInvalidRequest, "invalid or unsupported request"},
		{"BYOK credentials", execution.CategoryBYOKCredentials, 400, apiErrorInvalidRequest, "invalid or unsupported request"},
		{"BYOK selector", execution.CategoryBYOKSelector, 400, apiErrorInvalidRequest, "invalid or unsupported request"},
		{"gateway control", execution.CategoryGatewayControl, 400, apiErrorInvalidRequest, "invalid or unsupported request"},
		{"model not found", execution.CategoryModelNotFound, 404, apiErrorNotFound, "model or route not found"},
		{"permission", execution.CategoryPermission, 403, apiErrorPermission, "permission denied"},
		{"rate limit", execution.CategoryRateLimit, 429, apiErrorRateLimit, "rate limit exceeded"},
		{"overload", execution.CategoryOverload, 503, apiErrorOverloaded, "service unavailable"},
		{"unavailable", execution.CategoryUnavailable, 503, apiErrorOverloaded, "service unavailable"},
		{"failed dependency", execution.CategoryFailedDependency, 424, apiErrorAPI, "upstream dependency failed"},
		{"catalog configuration", execution.CategoryCatalogConfiguration, 424, apiErrorAPI, "upstream dependency failed"},
		{"upstream", execution.CategoryUpstream, 502, apiErrorAPI, "upstream failure"},
		{"timeout", execution.CategoryTimeout, 504, apiErrorTimeout, "request timed out"},
		{"cancellation", execution.CategoryCancellation, 499, apiErrorAPI, "request canceled"},
		{"internal", execution.CategoryInternal, 500, apiErrorAPI, "internal error"},
		{"provider stream", execution.CategoryProviderStream, 500, apiErrorAPI, "internal error"},
		{"unknown", execution.Category(255), 500, apiErrorAPI, "internal error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			WriteError(response, categoryStatus(tc.category))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			assert.JSONEq(t, fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":%q}}`, tc.typ, tc.message), response.Body.String())
		})
	}
}

func TestHostErrorWriter_WriteHostError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		category host.ErrorCategory
		status   int
		body     string
	}{
		{"permission", host.ErrorPermission, 403, `{"type":"error","error":{"type":"permission_error","message":"permission denied"}}`},
		{"BYOK discovery", host.ErrorBYOKDiscovery, 400, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid or unsupported request"}}`},
		{"internal", host.ErrorInternal, 500, `{"type":"error","error":{"type":"api_error","message":"internal error"}}`},
		{"catalog configuration", host.ErrorCatalogConfiguration, 500, `{"type":"error","error":{"type":"api_error","message":"internal error"}}`},
		{"unknown", host.ErrorCategory(255), 500, `{"type":"error","error":{"type":"api_error","message":"internal error"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewHostErrorWriter().WriteHostError(response, tc.category)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			assert.JSONEq(t, tc.body, response.Body.String())
		})
	}
}
