package grafana

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	part, err := decodeStreamPart([]byte(`{"type":"error","error":{"message":"safe","type":"internal_server_error","code":"upstream_error","param":null,"statusCode":502,"retryable":true,"data":{ "correct": "<" },"Data":{"wrong":true}},"Error":{"data":{"wrong":true}}}`))
	require.NoError(t, err)
	assert.Equal(t, json.RawMessage(`{ "correct": "<" }`), part.APICallError.Data)
	for _, body := range []string{
		`{"type":"error","error":{"message":"safe","type":"failed_dependency","code":"failed_dependency","param":null,"statusCode":424,"retryable":true,"data":{}}}`,
		strings.Replace(developerEvidenceStream(true), "native account rejected", string([]byte{0xff}), 1),
	} {
		_, err := decodeStreamPart([]byte(body))
		require.Error(t, err)
	}
}

func stringMode(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "unary"
}
