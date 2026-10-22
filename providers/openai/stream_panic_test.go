package openai

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

	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const panicResponseCreated = "data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"gpt-4o\",\"object\":\"response\",\"status\":\"in_progress\",\"output\":[]}}\n\n"

const panicTextDelta = "data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"output_index\":0,\"content_index\":0,\"item_id\":\"msg_1\",\"delta\":\"hi\"}\n\n"

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

type panicRoundTripFunc func(*http.Request) (*http.Response, error)

func (f panicRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func panicStreamModel(body io.ReadCloser, opts ...Option) provider.LanguageModel {
	client := &http.Client{Transport: panicRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body, Request: req}, nil
	})}
	opts = append(opts, WithRequestOptions(option.WithBaseURL("http://provider.invalid/v1/"), option.WithHTTPClient(client), option.WithMaxRetries(0)))
	return NewResponses("key", "gpt-4o", opts...)
}

func readPanicStream(result *provider.StreamResult) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	return parts
}

func assertSinglePanicError(t *testing.T, parts []provider.StreamPart) {
	t.Helper()
	var errs []provider.StreamPart
	for _, part := range parts {
		if part.Type == provider.PartError {
			errs = append(errs, part)
		}
	}
	require.Len(t, errs, 1)
	assert.Equal(t, provider.PartError, parts[len(parts)-1].Type, "nothing may follow the panic error")
	require.NotNil(t, errs[0].APICallError)
	assert.True(t, errors.Is(errs[0].APICallError, streamrecover.ErrPanic))
	assert.False(t, errs[0].APICallError.IsRetryable)
	assert.NotContains(t, errs[0].APICallError.Error(), "secret-panic-value")
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

	t.Run("transport reader during preflight", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader(panicResponseCreated)}
		_, err := panicStreamModel(body).DoStream(context.Background(), options)
		assert.Equal(t, streamrecover.ErrPanic, err)
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("transport reader after the first event", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader(panicResponseCreated + panicTextDelta)}
		result, err := panicStreamModel(body).DoStream(context.Background(), options)
		require.NoError(t, err)
		assertSinglePanicError(t, readPanicStream(result))
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("stream adapter", func(t *testing.T) {
		annotation := "data: {\"type\":\"response.output_text.annotation.added\",\"sequence_number\":2,\"output_index\":0,\"content_index\":0,\"item_id\":\"msg_1\",\"annotation_index\":0,\"annotation\":{\"type\":\"url_citation\",\"url\":\"https://example.com\",\"title\":\"Example\",\"start_index\":0,\"end_index\":2}}\n\n"
		body := newBlockingBody(panicResponseCreated+annotation, false)
		model := panicStreamModel(body, WithGenerateID(func() string { panic("secret-panic-value") }))
		result, err := model.DoStream(context.Background(), options)
		require.NoError(t, err)
		assertSinglePanicError(t, readPanicStream(result))
		assert.Eventually(t, body.released.Load, time.Second, time.Millisecond, "the reader must be released")
	})

	t.Run("body close panics after a reader panic", func(t *testing.T) {
		body := &panickingBody{prefix: strings.NewReader(panicResponseCreated + panicTextDelta), closePanics: true}
		result, err := panicStreamModel(body).DoStream(context.Background(), options)
		require.NoError(t, err)
		assertSinglePanicError(t, readPanicStream(result))
		assert.Equal(t, int32(1), body.closed.Load())
	})

	t.Run("body close panics after the stream finishes", func(t *testing.T) {
		completed := "data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"created_at\":1,\"model\":\"gpt-4o\",\"object\":\"response\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"
		body := &closePanickingBody{Reader: strings.NewReader(panicResponseCreated + panicTextDelta + completed)}
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
		body := newBlockingBody(panicResponseCreated+panicTextDelta, true)
		ctx, cancel := context.WithCancel(context.Background())
		result, err := panicStreamModel(body).DoStream(ctx, options)
		require.NoError(t, err)
		<-result.Stream
		cancel()
		readPanicStream(result)
		assert.Eventually(t, body.released.Load, time.Second, time.Millisecond, "the reader must be released")
	})
}
