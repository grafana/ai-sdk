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

func TestDoStream_RecoversMalformedEvents(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"type":"response.created"`,
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
			`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_1","delta":"hello"}`,
			`not JSON`,
			`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}}`,
			`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
			`[DONE]`,
		} {
			_, err := fmt.Fprintf(w, "data: %s\n\n", data)
			require.NoError(t, err)
		}
	}))
	defer server.Close()
	model := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
	result, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	var errors int
	var text string
	var finishes int
	for part := range result.Stream {
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
			assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
		}
	}
	assert.Equal(t, 2, errors)
	assert.Equal(t, "hello", text)
	assert.Equal(t, 1, finishes)
	assert.Equal(t, int32(1), requests.Load())
}
