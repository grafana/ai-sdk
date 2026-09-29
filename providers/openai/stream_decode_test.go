package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectHTTPStream(t *testing.T, events []string, opts provider.CallOptions) []provider.StreamPart {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range events {
			_, err := fmt.Fprintf(w, "data: %s\n\n", data)
			assert.NoError(t, err)
		}
	}))
	defer server.Close()
	model := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
	result, err := model.DoStream(context.Background(), opts)
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	assert.Equal(t, int32(1), requests.Load())
	return parts
}

func TestDoStream_RecoversMalformedEvents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		terminal  string
		lateError bool
	}{
		{name: "completed", terminal: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`},
		{name: "incomplete", terminal: `{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`},
		{name: "malformed after completion", terminal: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`, lateError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{
				`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
				`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_1","delta":"hello"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}}`,
				tc.terminal,
			}
			if tc.lateError {
				events = append(events, `not JSON`)
			} else {
				events = append([]string{`{"type":"response.created"`, `not JSON`}, events...)
			}
			events = append(events, `[DONE]`)
			parts := collectHTTPStream(t, events, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			var errors, finishes int
			var text string
			for _, part := range parts {
				switch part.Type {
				case provider.PartError:
					errors++
					require.NotNil(t, part.APICallError)
					assert.False(t, part.APICallError.IsRetryable)
				case provider.PartTextDelta:
					text += part.Delta
				case provider.PartFinish:
					finishes++
					require.NotNil(t, part.FinishReason)
					assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonError}, *part.FinishReason)
					require.NotNil(t, part.Usage)
					require.NotNil(t, part.Usage.InputTokens.Total)
					require.NotNil(t, part.Usage.OutputTokens.Total)
					assert.Equal(t, 3, *part.Usage.InputTokens.Total)
					assert.Equal(t, 2, *part.Usage.OutputTokens.Total)
				}
			}
			if tc.lateError {
				assert.Equal(t, 1, errors)
			} else {
				assert.Equal(t, 2, errors)
			}
			assert.Equal(t, "hello", text)
			assert.Equal(t, 1, finishes)
			require.NotEmpty(t, parts)
			assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
		})
	}
}

func TestDoStream_TruncatedParallelInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		last string
	}{
		{name: "EOF"},
		{name: "malformed done", last: `{"type":"response.output_item.done"`},
		{name: "completed without item done", last: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{
				`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_b","call_id":"call_b","name":"parallel","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"fc_b","delta":"second"}`,
				`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"parallel","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_a","delta":"first"}`,
			}
			if tc.last != "" {
				events = append(events, tc.last)
			}
			parts := collectHTTPStream(t, events, provider.CallOptions{})
			var toolParts []provider.StreamPart
			finished := false
			for _, part := range parts {
				switch part.Type {
				case provider.PartFinish:
					finished = true
				case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall:
					assert.False(t, finished, "tool input must precede finish")
					toolParts = append(toolParts, part)
				}
			}
			assert.Equal(t, []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "call_a", ToolName: "parallel"},
				{Type: provider.PartToolInputDelta, ID: "call_a", Delta: "first"},
				{Type: provider.PartToolInputStart, ID: "call_b", ToolName: "parallel"},
				{Type: provider.PartToolInputDelta, ID: "call_b", Delta: "second"},
			}, toolParts)
		})
	}
}
