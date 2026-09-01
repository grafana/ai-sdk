package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamErrorCase struct {
	name      string
	errorType string
	status    int
	retryable bool
}

var streamErrorCases = []streamErrorCase{
	{name: "overloaded", errorType: "overloaded_error", status: 529, retryable: true},
	{name: "api", errorType: "api_error", status: 500, retryable: true},
	{name: "rate limit", errorType: "rate_limit_error", status: 429, retryable: true},
	{name: "request too large", errorType: "request_too_large", status: 413},
	{name: "authentication", errorType: "authentication_error", status: 401},
	{name: "permission", errorType: "permission_error", status: 403},
	{name: "not found", errorType: "not_found_error", status: 404},
	{name: "billing", errorType: "billing_error", status: 400},
	{name: "invalid request", errorType: "invalid_request_error", status: 400},
}

func TestDoStream_InitialSSEError(t *testing.T) {
	for _, tc := range streamErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			model, closeServer := newSSEErrorModel(t, tc.errorType, false)
			defer closeServer()

			result, err := model.DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hello")},
			})

			require.Error(t, err)
			assert.Nil(t, result)
			apiErr := requireAPICallError(t, err, tc.errorType)
			assert.Equal(t, tc.status, apiErr.StatusCode)
			assert.Equal(t, tc.retryable, apiErr.IsRetryable)
			assert.Equal(t, "failed", apiErr.Message)
			assert.JSONEq(t, fmt.Sprintf(`{"type":%q,"message":"failed"}`, tc.errorType), apiErr.ResponseBody)
			assert.Contains(t, apiErr.URL, "/v1/messages")
		})
	}
}

func TestDoStream_PostOutputSSEError(t *testing.T) {
	for _, tc := range streamErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			model, closeServer := newSSEErrorModel(t, tc.errorType, true)
			defer closeServer()

			result, err := model.DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hello")},
			})
			require.NoError(t, err)

			var text string
			var responseID string
			var apiErr *provider.APICallError
			for part := range result.Stream {
				if part.Type == provider.PartResponseMeta {
					responseID = part.ResponseID
				}
				if part.Type == provider.PartTextDelta {
					text += part.Delta
				}
				if part.Type == provider.PartError {
					apiErr = part.APICallError
				}
			}

			assert.Equal(t, "msg_test", responseID)
			assert.Equal(t, "Hello", text)
			require.NotNil(t, apiErr)
			apiErr = requireAPICallError(t, apiErr, tc.errorType)
			assert.Equal(t, tc.status, apiErr.StatusCode)
			assert.Equal(t, tc.retryable, apiErr.IsRetryable)
		})
	}
}

func newSSEErrorModel(t *testing.T, errorType string, afterOutput bool) (provider.LanguageModel, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if afterOutput {
			_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
			_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		}
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":%q,\"message\":\"failed\"}}\n\n", errorType)
	}))

	model := New("test-key", "claude-test",
		WithRequestOptions(
			option.WithBaseURL(server.URL),
			option.WithHTTPClient(server.Client()),
			option.WithMaxRetries(0),
		),
	)
	return model, server.Close
}

func requireAPICallError(t *testing.T, err error, errorType string) *provider.APICallError {
	t.Helper()

	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr)
	require.NotEmpty(t, apiErr.Data)
	assert.Equal(t, errorType, apiErr.Type)

	var envelope struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(apiErr.Data, &envelope))
	assert.Equal(t, errorType, envelope.Error.Type)
	return apiErr
}
