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
			assert.Contains(t, api.ResponseBody, "private")
			assert.JSONEq(t, api.ResponseBody, string(api.Data))
			assert.NotContains(t, err.Error(), "private")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestGatewayError_EvidenceRetention(t *testing.T) {
	body := `{"error":{"message":"upstream failure","type":"internal_server_error","code":"upstream_error","param":null},"providerMetadata":{"gateway":{"evidence":{"attempts":[{"index":1,"provider":"anthropic","nativeError":{"statusCode":401,"isRetryable":false}},{"index":2,"provider":"openai","nativeError":{"statusCode":500,"isRetryable":false}}]}}}}`
	for _, streaming := range []bool{false, true} {
		t.Run(stringMode(streaming), func(t *testing.T) {
			model := developerEvidenceModel(t, http.StatusBadGateway, "application/json", body)
			var err error
			if streaming {
				_, err = model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			} else {
				_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			}
			var gateway *GatewayError
			var api *provider.APICallError
			require.ErrorAs(t, err, &gateway)
			require.ErrorAs(t, err, &api)
			assert.True(t, gateway.IsRetryable)
			assert.JSONEq(t, body, string(api.Data))
			assert.Equal(t, body, api.ResponseBody)
			assert.NotContains(t, err.Error(), "anthropic")
		})
	}
}

func TestStreamError_EvidenceRetention(t *testing.T) {
	for _, data := range []string{"", `null`, `{}`, `{ "detail": "<&", "escape": "\u0061", "nested": [ 1, 2 ] }`, `{"providerMetadata":{"gateway":{"evidence":{"selectedAttempt":2}}},"nativeError":{"code":42,"isRetryable":false}}`} {
		t.Run(data, func(t *testing.T) {
			suffix := ""
			if data != "" {
				suffix = `,"data":` + data
			}
			part, err := decodeStreamPart([]byte(`{"type":"error","error":{"message":"upstream failure","type":"internal_server_error","code":"upstream_error","param":null,"statusCode":502,"retryable":true` + suffix + `}}`))
			require.NoError(t, err)
			require.NotNil(t, part.APICallError)
			if data == "" {
				assert.Nil(t, part.APICallError.Data)
			} else {
				assert.Equal(t, json.RawMessage(data), part.APICallError.Data)
			}
			assert.True(t, part.APICallError.IsRetryable)
			assert.Empty(t, part.APICallError.ResponseBody)
		})
	}
	part, err := decodeStreamPart([]byte(`{"type":"error","Error":{"Message":"safe","Type":"internal_server_error","Code":"upstream_error","Param":null,"StatusCode":502,"Retryable":true,"Data":{ "detail": "\u0061<&", "value": 1e2 }}}`))
	require.NoError(t, err)
	require.NotNil(t, part.APICallError)
	assert.Equal(t, json.RawMessage(`{ "detail": "\u0061<&", "value": 1e2 }`), part.APICallError.Data)
	for _, body := range []string{
		`{"type":"error"}`,
		`{"type":"error","error":null}`,
		`{"type":"error","error":[]}`,
		`{"type":"error","error":{"message":"safe"}`,
		`{"type":"error","error":{"message":"safe","type":"failed_dependency","code":"failed_dependency","param":null,"statusCode":424,"retryable":true,"data":{}}}`,
		strings.Replace(developerEvidenceStream(true), "native account rejected", string([]byte{0xff}), 1),
	} {
		_, err := decodeStreamPart([]byte(body))
		require.Error(t, err)
	}
}

func TestGatewayError_StandardJSONDecoding(t *testing.T) {
	body := `{"Error":{"Message":"safe","Type":"internal_server_error","Code":"upstream_error","Param":null},"future":{"value":1e2}}`
	model := developerEvidenceModel(t, http.StatusBadGateway, "application/json", body)
	_, err := model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{}})
	var gateway *GatewayError
	var api *provider.APICallError
	require.ErrorAs(t, err, &gateway)
	require.ErrorAs(t, err, &api)
	assert.Equal(t, GatewayInternalServer, gateway.Category)
	assert.True(t, api.IsRetryable)
	assert.Equal(t, body, string(api.Data))
	assert.Equal(t, body, api.ResponseBody)
}

func stringMode(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "unary"
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
