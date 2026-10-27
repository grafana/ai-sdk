package anthropic

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const panicMessageStart = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"

type panickingBody struct {
	prefix      io.Reader
	closePanics bool
	closed      atomic.Int32
}

func (b *panickingBody) Read(p []byte) (int, error) {
	if n, err := b.prefix.Read(p); n > 0 || !errors.Is(err, io.EOF) {
		return n, err
	}
	panic("secret-panic-value")
}

func (b *panickingBody) Close() error {
	b.closed.Add(1)
	if b.closePanics {
		panic("secret-panic-value")
	}
	return nil
}

type closePanickingBody struct {
	io.Reader
	closed atomic.Int32
}

func (b *closePanickingBody) Close() error {
	b.closed.Add(1)
	panic("secret-panic-value")
}

type blockingBody struct {
	prefix      io.Reader
	closePanics bool
	closing     chan struct{}
	closeOnce   sync.Once
	released    atomic.Bool
}

func newBlockingBody(prefix string, closePanics bool) *blockingBody {
	return &blockingBody{prefix: strings.NewReader(prefix), closePanics: closePanics, closing: make(chan struct{})}
}

func (b *blockingBody) Read(p []byte) (int, error) {
	if n, err := b.prefix.Read(p); n > 0 || !errors.Is(err, io.EOF) {
		return n, err
	}
	<-b.closing
	b.released.Store(true)
	return 0, net.ErrClosed
}

func (b *blockingBody) Close() error {
	b.closeOnce.Do(func() { close(b.closing) })
	if b.closePanics {
		panic("secret-panic-value")
	}
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func panicStreamModel(body io.ReadCloser) *model {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body, Request: req}, nil
	})}
	return New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL("http://provider.invalid"), option.WithHTTPClient(client), option.WithMaxRetries(0))).(*model)
}

func readPanicStream(result *provider.StreamResult) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	return parts
}

func assertPanicPart(t *testing.T, parts []provider.StreamPart) {
	t.Helper()
	require.NotEmpty(t, parts)
	last := parts[len(parts)-1]
	assert.Equal(t, provider.PartError, last.Type)
	require.NotNil(t, last.APICallError)
	assert.True(t, errors.Is(last.APICallError, streamrecover.ErrPanic))
	assert.False(t, last.APICallError.IsRetryable)
	assert.NotContains(t, last.APICallError.Error(), "secret-panic-value")
	for _, part := range parts[:len(parts)-1] {
		assert.NotEqual(t, provider.PartError, part.Type)
	}
}

func TestDoStream_WorkerPanic(t *testing.T) {
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}

	t.Run("transport reader before the first event", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader("")}
		_, err := panicStreamModel(body).DoStream(context.Background(), options)
		assert.Equal(t, streamrecover.ErrPanic, err)
		assert.NotContains(t, err.Error(), "secret-panic-value")
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("transport reader after the first event", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader(panicMessageStart)}
		result, err := panicStreamModel(body).DoStream(context.Background(), options)
		require.NoError(t, err)
		assertPanicPart(t, readPanicStream(result))
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("stream adapter", func(t *testing.T) {
		events := panicMessageStart +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"web_search_tool_result\",\"tool_use_id\":\"stu_1\",\"content\":[{\"type\":\"web_search_result\",\"title\":\"Page\",\"url\":\"https://example.com\",\"page_age\":\"2d\",\"encrypted_content\":\"abc\"}]}}\n\n"
		body := newBlockingBody(events, false)
		model := panicStreamModel(body)
		model.generateID = func() string { panic("secret-panic-value") }
		result, err := model.DoStream(context.Background(), options)
		require.NoError(t, err)
		parts := readPanicStream(result)
		assertPanicPart(t, parts)
		assert.Equal(t, provider.PartToolResult, parts[len(parts)-2].Type)
		assert.Eventually(t, body.released.Load, time.Second, time.Millisecond, "the reader must be released")
	})

	t.Run("body close panics after a reader panic", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader(panicMessageStart), closePanics: true}
		result, err := panicStreamModel(body).DoStream(context.Background(), options)
		require.NoError(t, err)
		assertPanicPart(t, readPanicStream(result))
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("body close panics after the stream finishes", func(t *testing.T) {
		body := &closePanickingBody{Reader: strings.NewReader(transportSSE(transportEvents))}
		result, err := panicStreamModel(body).DoStream(context.Background(), options)
		require.NoError(t, err)
		parts := readPanicStream(result)
		require.NotEmpty(t, parts)
		assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
		for _, part := range parts {
			assert.NotEqual(t, provider.PartError, part.Type)
		}
		assert.Eventually(t, func() bool { return body.closed.Load() > 0 }, time.Second, time.Millisecond)
	})

	t.Run("body close panics on cancellation", func(t *testing.T) {
		body := newBlockingBody(panicMessageStart, true)
		ctx, cancel := context.WithCancel(context.Background())
		result, err := panicStreamModel(body).DoStream(ctx, options)
		require.NoError(t, err)
		<-result.Stream
		cancel()
		readPanicStream(result)
		assert.Eventually(t, body.released.Load, time.Second, time.Millisecond, "the reader must be released")
	})
}
