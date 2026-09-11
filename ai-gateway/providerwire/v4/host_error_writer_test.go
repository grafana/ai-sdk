package v4

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostErrorWriter(t *testing.T) {
	require.NotNil(t, NewHostErrorWriter())
}

func TestHostErrorWriter_Write(t *testing.T) {
	writer := NewHostErrorWriter()
	tests := []struct {
		name     string
		category HostErrorCategory
		status   int
		body     []byte
	}{
		{name: "authentication", category: HostErrorAuthentication, status: http.StatusUnauthorized, body: []byte(`{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`)},
		{name: "permission", category: HostErrorPermission, status: http.StatusForbidden, body: []byte(`{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`)},
		{name: "internal", category: HostErrorInternal, status: http.StatusInternalServerError, body: []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)},
		{name: "zero category", category: 0, status: http.StatusInternalServerError, body: []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)},
		{name: "unknown category", category: 255, status: http.StatusInternalServerError, body: []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writer.Write(response, tc.category)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, string(tc.body), response.Body.String())
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		})
	}
}
