package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStream_CancelWithStalledConsumer(t *testing.T) {
	blocks := make([]map[string]any, 100)
	for i := range blocks {
		blocks[i] = map[string]any{"type": "tool_use", "id": fmt.Sprintf("tool_%d", i), "name": "lookup", "input": map[string]any{"index": i}}
	}
	message, err := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "content": blocks,
			"model": "claude-test", "usage": map[string]int{"input_tokens": 1, "output_tokens": 0},
		},
	})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", message)
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	bodyClosed := make(chan struct{})
	client := server.Client()
	client.Transport = trackingRoundTripper{next: client.Transport, closed: bodyClosed}
	m := New("direct-key", "claude-test", WithRequestOptions(
		option.WithBaseURL(server.URL), option.WithHTTPClient(client), option.WithMaxRetries(0),
	))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	maxTokens := 64
	result, err := m.DoStream(ctx, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens, IncludeRawChunks: true,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(result.Stream) == cap(result.Stream) }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-bodyClosed:
	case <-time.After(2 * time.Second):
		assert.Fail(t, "SDK response body was not closed while consumer was stalled")
	}

	drained := make(chan []provider.StreamPart, 1)
	go func() {
		var parts []provider.StreamPart
		for part := range result.Stream {
			parts = append(parts, part)
		}
		drained <- parts
	}()
	select {
	case parts := <-drained:
		for _, part := range parts {
			assert.NotEqual(t, provider.PartError, part.Type)
			assert.NotEqual(t, provider.PartFinish, part.Type)
		}
	case <-time.After(2 * time.Second):
		require.FailNow(t, "provider stream did not close after cancellation")
	}
}

type trackingRoundTripper struct {
	next   http.RoundTripper
	closed chan struct{}
}

func (t trackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(req)
	if response != nil {
		response.Body = &trackingCloseBody{ReadCloser: response.Body, closed: t.closed}
	}
	return response, err
}

type trackingCloseBody struct {
	io.ReadCloser
	closed chan struct{}
}

func (b *trackingCloseBody) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return b.ReadCloser.Close()
}
