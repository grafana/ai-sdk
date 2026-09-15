package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayError_RegisteredMatrix(t *testing.T) {
	for _, tc := range []struct {
		status    int
		category  GatewayErrorCategory
		code      string
		retryable bool
	}{
		{400, GatewayInvalidRequest, "invalid_request", false}, {401, GatewayAuthentication, "authentication_error", false}, {403, GatewayForbidden, "forbidden", false}, {404, GatewayModelNotFound, "model_not_found", false}, {429, GatewayRateLimit, "rate_limit_exceeded", true}, {424, GatewayFailedDependency, "failed_dependency", false}, {500, GatewayInternalServer, "internal_error", true}, {502, GatewayInternalServer, "upstream_error", true}, {503, GatewayInternalServer, "overloaded", true}, {504, GatewayInternalServer, "timeout", true}, {499, GatewayInternalServer, "canceled", false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			calls := 0
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "safe message", "type": tc.category, "param": nil, "code": tc.code, "private": "private-backend"}, "generationId": "private-generation", "private": "private-topology"})
			}, nil)
			rows, err := p.ListModels(context.Background())
			assert.Nil(t, rows)
			require.Error(t, err)
			var gateway *GatewayError
			require.ErrorAs(t, err, &gateway)
			assert.Equal(t, tc.category, gateway.Category)
			assert.Equal(t, tc.code, gateway.Code)
			assert.Equal(t, tc.status, gateway.StatusCode)
			assert.Equal(t, tc.retryable, gateway.IsRetryable)
			assert.Equal(t, "safe message", gateway.Message)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, tc.retryable, api.IsRetryable)
			assert.NotContains(t, api.ResponseBody, "private")
			assert.NotContains(t, err.Error(), "private")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestGatewayError_InvalidEnvelopes(t *testing.T) {
	valid := `{"error":{"message":"safe","type":"invalid_request_error","param":null,"code":"invalid_request"}}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"unknown type", strings.Replace(valid, "invalid_request_error", "private-type", 1), 400},
		{"unknown code", strings.Replace(valid, `"code":"invalid_request"`, `"code":"private-code"`, 1), 400},
		{"wrong status", valid, 500},
		{"empty message", strings.Replace(valid, "safe", "", 1), 400},
		{"missing param", strings.Replace(valid, `"param":null,`, "", 1), 400},
		{"non-null param", strings.Replace(valid, `"param":null`, `"param":"private-path"`, 1), 400},
		{"trailing", valid + valid, 400}, {"html", "private-error-body", 500}, {"null", `{"error":null}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}, nil)
			_, err := p.ListModels(context.Background())
			require.Error(t, err)
			var gateway *GatewayError
			assert.NotErrorAs(t, err, &gateway)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
			assert.NotContains(t, err.Error(), "private")
			assert.Empty(t, api.ResponseBody)
		})
	}
}
