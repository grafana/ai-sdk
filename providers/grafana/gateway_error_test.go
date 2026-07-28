package grafana

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayError_Error(t *testing.T) {
	gw := &GatewayError{
		Type:       GatewayErrorRateLimit,
		Message:    "rate limit exceeded",
		StatusCode: 429,
	}
	msg := gw.Error()
	assert.Contains(t, msg, "429")
	assert.Contains(t, msg, "rate limit exceeded")
	assert.Contains(t, msg, string(GatewayErrorRateLimit))
}

func TestNormalizeAPICallError_Mapping(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		body      string
		want      GatewayErrorType
		wantModel string
	}{
		{
			name: "authentication_error from nested data",
			data: `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`,
			want: GatewayErrorAuthentication,
		},
		{
			name: "permission_error maps to authentication",
			data: `{"error":{"type":"permission_error"}}`,
			want: GatewayErrorAuthentication,
		},
		{
			name: "invalid_request_error",
			data: `{"error":{"type":"invalid_request_error"}}`,
			want: GatewayErrorInvalidRequest,
		},
		{
			name: "billing_error maps to invalid_request",
			data: `{"error":{"type":"billing_error"}}`,
			want: GatewayErrorInvalidRequest,
		},
		{
			name: "rate_limit_error maps to rate_limit_exceeded",
			data: `{"error":{"type":"rate_limit_error"}}`,
			want: GatewayErrorRateLimit,
		},
		{
			name: "overloaded_error maps to rate_limit_exceeded",
			data: `{"error":{"type":"overloaded_error"}}`,
			want: GatewayErrorRateLimit,
		},
		{
			name:      "not_found_error maps to model_not_found with model id",
			data:      `{"error":{"type":"not_found_error","modelId":"claude-x"}}`,
			want:      GatewayErrorModelNotFound,
			wantModel: "claude-x",
		},
		{
			name:      "top-level model_not_found with param model id",
			data:      `{"type":"model_not_found","param":{"modelId":"gpt-x"}}`,
			want:      GatewayErrorModelNotFound,
			wantModel: "gpt-x",
		},
		{
			name: "api_error maps to internal_server_error",
			data: `{"error":{"type":"api_error"}}`,
			want: GatewayErrorInternalServer,
		},
		{
			name: "unknown type defaults to internal_server_error",
			data: `{"error":{"type":"some_new_error"}}`,
			want: GatewayErrorInternalServer,
		},
		{
			name: "missing data falls back to response body",
			body: `{"error":{"type":"rate_limit_error"}}`,
			want: GatewayErrorRateLimit,
		},
		{
			name: "empty data and body default to internal_server_error",
			want: GatewayErrorInternalServer,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
				Message:      "boom",
				StatusCode:   400,
				ResponseBody: tc.body,
			})
			if tc.data != "" {
				apiErr.Data = json.RawMessage(tc.data)
			}

			gw := NormalizeAPICallError(apiErr)
			require.NotNil(t, gw)
			assert.Equal(t, tc.want, gw.Type)
			assert.Equal(t, tc.wantModel, gw.ModelID)
			assert.Equal(t, "boom", gw.Message)
			assert.Equal(t, 400, gw.StatusCode)
		})
	}
}

func TestNormalizeAPICallError_DataPreferredOverBody(t *testing.T) {
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		StatusCode:   429,
		ResponseBody: `{"error":{"type":"invalid_request_error"}}`,
	})
	apiErr.Data = json.RawMessage(`{"error":{"type":"rate_limit_error"}}`)

	gw := NormalizeAPICallError(apiErr)
	require.NotNil(t, gw)
	assert.Equal(t, GatewayErrorRateLimit, gw.Type)
}

func TestNormalizeAPICallError_Nil(t *testing.T) {
	assert.Nil(t, NormalizeAPICallError(nil))
}

func TestGatewayError_ErrorsAs(t *testing.T) {
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "rate limited",
		StatusCode: 429,
	})
	apiErr.Data = json.RawMessage(`{"error":{"type":"rate_limit_error"}}`)

	var err error = NormalizeAPICallError(apiErr)

	var gw *GatewayError
	require.True(t, errors.As(err, &gw))
	assert.Equal(t, GatewayErrorRateLimit, gw.Type)

	var unwrapped *provider.APICallError
	require.True(t, errors.As(err, &unwrapped))
	assert.Equal(t, 429, unwrapped.StatusCode)
	assert.Equal(t, "rate limited", unwrapped.Message)
	assert.Equal(t, apiErr, unwrapped)
}
