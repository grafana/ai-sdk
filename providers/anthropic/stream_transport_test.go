package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingMessageDecoder struct {
	ssestream.Decoder
	closes atomic.Int32
	closed chan struct{}
	once   sync.Once
}

func (d *countingMessageDecoder) Close() error {
	d.closes.Add(1)
	defer d.once.Do(func() { close(d.closed) })
	return d.Decoder.Close()
}

func TestDoStream_SDKDecoderOwnership(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelStream), func(t *testing.T) {
			contentType := fmt.Sprintf("application/x-aisdk-anthropic-decoder-%t", cancelStream)
			instances := make(chan *countingMessageDecoder, 2)
			ssestream.RegisterDecoder(contentType, func(body io.ReadCloser) ssestream.Decoder {
				var marker [1]byte
				_, err := io.ReadFull(body, marker[:])
				assert.NoError(t, err)
				assert.Equal(t, byte('!'), marker[0])
				d := &countingMessageDecoder{Decoder: ssestream.NewDecoder(&http.Response{Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}), closed: make(chan struct{})}
				instances <- d
				return d
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = io.WriteString(w, "!"+transportSSE(transportEvents))
				if cancelStream {
					for range 512 {
						_, _ = io.WriteString(w, transportSSE([]string{transportEvents[2]}))
					}
					_ = http.NewResponseController(w).Flush()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := m.DoStream(ctx, provider.CallOptions{IncludeRawChunks: true})
			require.NoError(t, err)
			instance := <-instances
			if cancelStream {
				require.Eventually(t, func() bool { return len(result.Stream) == cap(result.Stream) }, time.Second, time.Millisecond)
				cancel()
			} else {
				for range result.Stream {
				}
			}
			select {
			case <-instance.closed:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "decoder was not closed")
			}
			assert.Equal(t, int32(1), instance.closes.Load())
			assert.Empty(t, instances)
		})
	}
}

func TestConsumeStream_CancelStalledConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := append([]string(nil), transportEvents...)
	for range 512 {
		events = append(events, transportEvents[2])
	}
	body := &startupBody{ReadCloser: io.NopCloser(strings.NewReader(transportSSE(events)))}
	response := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	items := pumpMessageStream(ctx, response, true)
	output := make(chan provider.StreamPart, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeStream(ctx, items, nil, output, toolNameMapping{}, nil, false, nil, func() string { return "id" }, "anthropic", false, nil)
	}()
	require.Eventually(t, func() bool { return len(output) == cap(output) }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "producer required output reads after cancellation")
	}
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, time.Second, time.Millisecond)
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
