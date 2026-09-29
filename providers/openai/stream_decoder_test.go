package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingStreamDecoder struct {
	ssestream.Decoder
	closes atomic.Int32
	closed chan struct{}
	once   sync.Once
}

func (d *countingStreamDecoder) Close() error {
	d.closes.Add(1)
	defer d.once.Do(func() { close(d.closed) })
	return d.Decoder.Close()
}

func TestDoStream_SDKDecoderOwnership(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		name := "completed"
		if cancelStream {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			contentType := "application/x-aisdk-decoder-" + name
			var mu sync.Mutex
			var decoders []*countingStreamDecoder
			ssestream.RegisterDecoder(contentType, func(body io.ReadCloser) ssestream.Decoder {
				var marker [1]byte
				_, err := io.ReadFull(body, marker[:])
				assert.NoError(t, err)
				assert.Equal(t, byte('!'), marker[0])
				d := &countingStreamDecoder{Decoder: ssestream.NewDecoder(&http.Response{Body: body, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}), closed: make(chan struct{})}
				mu.Lock()
				decoders = append(decoders, d)
				mu.Unlock()
				return d
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/responses", r.URL.Path)
				assert.Equal(t, "Bearer sdk-key", r.Header.Get("Authorization"))
				assert.Equal(t, "client", r.Header.Get("X-SDK-Client"))
				assert.Equal(t, "call", r.Header.Get("X-SDK-Call"))
				var body map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, true, body["stream"])
				w.Header().Set("Content-Type", contentType)
				_, err := io.WriteString(w, "!data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"delta\":\"hello\"}\n\n")
				assert.NoError(t, err)
				if cancelStream {
					assert.NoError(t, http.NewResponseController(w).Flush())
					<-r.Context().Done()
					return
				}
				_, err = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\ndata: [DONE]\n\n")
				assert.NoError(t, err)
			}))
			defer server.Close()
			client := openaisdk.NewClient(option.WithAPIKey("sdk-key"), option.WithBaseURL(server.URL), option.WithHeader("X-SDK-Client", "client"), option.WithMaxRetries(0))
			model := NewResponsesWithClient(client, "gpt-4o")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := model.DoStream(ctx, provider.CallOptions{Headers: map[string]string{"X-SDK-Call": "call"}})
			require.NoError(t, err)
			var text string
			for part := range result.Stream {
				if part.Type == provider.PartTextDelta {
					text += part.Delta
					if cancelStream {
						cancel()
					}
				}
			}
			assert.Equal(t, "hello", text)
			mu.Lock()
			instances := append([]*countingStreamDecoder(nil), decoders...)
			mu.Unlock()
			require.Len(t, instances, 1)
			select {
			case <-instances[0].closed:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "decoder was not closed")
			}
			assert.Equal(t, int32(1), instances[0].closes.Load())
		})
	}
}
