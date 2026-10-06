package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamErrorCase struct {
	name          string
	errorType     string
	initialStatus int
	initialRetry  bool
	streamRetry   bool
}

var streamErrorCases = []streamErrorCase{
	{name: "overloaded", errorType: "overloaded_error", initialStatus: 529, initialRetry: true, streamRetry: true},
	{name: "api", errorType: "api_error", initialStatus: 500, initialRetry: true, streamRetry: true},
	{name: "rate limit", errorType: "rate_limit_error", initialStatus: 500},
	{name: "invalid request", errorType: "invalid_request_error", initialStatus: 500},
}

func TestSafeguardsTransportFailure(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var calls atomic.Int32
			type capturedRequest struct {
				body  map[string]json.RawMessage
				betas []string
			}
			requests := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requests <- capturedRequest{body: body, betas: r.Header.Values("anthropic-beta")}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"provider failed"}}`))
			}))
			defer server.Close()
			model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
			maxTokens := 1024
			opts := provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens,
				ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Safeguards: []AnthropicSafeguard{{Type: AnthropicSafeguardDangerousToolUse}}}),
			}
			if stream {
				result, err := model.DoStream(context.Background(), opts)
				assert.Nil(t, result)
				require.Error(t, err)
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				assert.Nil(t, result)
				require.Error(t, err)
			}
			require.EqualValues(t, 1, calls.Load())
			captured := <-requests
			assert.JSONEq(t, `[{"type":"dangerous_tool_use"}]`, string(captured.body["safeguards"]))
			assert.Contains(t, captured.betas, "dangerous-tool-use-2026-09-03")
		})
	}
}

func TestSafeguardsMalformedStreamPartError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`+"\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_01":{"type":1,"explanation":"private-verdict"}}}}]},"usage":{"output_tokens":1}}`+"\n\n")
	}))
	defer server.Close()
	model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	var errors, finishes int
	for part := range result.Stream {
		switch part.Type {
		case provider.PartError:
			errors++
			require.NotNil(t, part.APICallError)
			assert.NotContains(t, part.APICallError.Message, "private-verdict")
		case provider.PartFinish:
			finishes++
		}
	}
	assert.Equal(t, 1, errors)
	assert.Zero(t, finishes)
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
			assert.Equal(t, tc.initialStatus, apiErr.StatusCode)
			assert.Equal(t, tc.initialRetry, apiErr.IsRetryable)
			assert.Equal(t, "failed", apiErr.Message)
			assert.JSONEq(t, fmt.Sprintf(`{"type":%q,"message":"failed"}`, tc.errorType), apiErr.ResponseBody)
			assert.Contains(t, apiErr.URL, "/v1/messages")
		})
	}
}

func TestDoStream_RawInitialError(t *testing.T) {
	for _, tc := range streamErrorCases {
		t.Run(tc.name, func(t *testing.T) {
			m, closeServer := newSSEErrorModel(t, tc.errorType, false)
			defer closeServer()
			result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: true})
			require.Error(t, err)
			assert.Nil(t, result)
			apiErr := requireAPICallError(t, err, tc.errorType)
			assert.Equal(t, tc.initialStatus, apiErr.StatusCode)
			assert.Equal(t, tc.initialRetry, apiErr.IsRetryable)
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
			var parts []provider.StreamPart
			for part := range result.Stream {
				parts = append(parts, part)
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

			require.NotEmpty(t, parts)
			assert.Equal(t, provider.PartStreamStart, parts[0].Type)
			assert.Equal(t, "msg_test", responseID)
			assert.Equal(t, "Hello", text)
			require.NotNil(t, apiErr)
			apiErr = requireAPICallError(t, apiErr, tc.errorType)
			assert.Equal(t, http.StatusOK, apiErr.StatusCode)
			assert.Equal(t, tc.streamRetry, apiErr.IsRetryable)
		})
	}
}

func TestDoStream_WarningsFollowSuccessfulPreflight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"unknown-model\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	model := New("test-key", "unknown-model",
		WithRequestOptions(
			option.WithBaseURL(server.URL),
			option.WithHTTPClient(server.Client()),
			option.WithMaxRetries(0),
		),
	)
	result, err := model.DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hello")},
	})
	require.NoError(t, err)

	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.NotEmpty(t, parts)
	assert.Equal(t, provider.PartStreamStart, parts[0].Type)
	require.NotEmpty(t, parts[0].Warnings)
	for _, part := range parts {
		if part.Type == provider.PartFinish {
			assert.Empty(t, part.Warnings)
			return
		}
	}
	t.Fatal("missing finish part")
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

	var envelope struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(apiErr.Data, &envelope))
	assert.Equal(t, errorType, envelope.Error.Type)
	return apiErr
}
