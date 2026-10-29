package v4

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
)

var _ host.ErrorWriter = (*HostErrorWriter)(nil)

func TestHostErrorWriter_WriteHostError(t *testing.T) {
	internal := `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`
	tests := []struct {
		name     string
		category host.ErrorCategory
		status   int
		body     string
	}{
		{name: "permission", category: host.ErrorPermission, status: http.StatusForbidden, body: `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`},
		{name: "BYOK discovery", category: host.ErrorBYOKDiscovery, status: http.StatusBadRequest, body: `{"error":{"message":"catalog discovery is unsupported for BYOK","type":"invalid_request_error","param":null,"code":"invalid_request"}}`},
		{name: "internal", category: host.ErrorInternal, status: http.StatusInternalServerError, body: internal},
		{name: "zero category", category: 0, status: http.StatusInternalServerError, body: internal},
		{name: "unknown category", category: 255, status: http.StatusInternalServerError, body: internal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewHostErrorWriter().WriteHostError(response, tc.category)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.String())
			assert.Equal(t, http.Header{"Content-Type": {"application/json"}}, response.Header())
		})
	}
}
