package openai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const transportTestResponse = `{"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`

func TestModel_ConcurrentTransportCapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request: %v", err)
		}
		digest := sha256.Sum256(body)
		w.Header().Set("X-Call", r.Header.Get("X-Call"))
		w.Header().Set("X-Body-Digest", hex.EncodeToString(digest[:]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, transportTestResponse)
	}))
	defer server.Close()
	m := NewResponses("sdk-key", "gpt-4o", WithRequestOptions(
		option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0),
	))
	type outcome struct {
		marker string
		result *provider.GenerateResult
		err    error
	}
	results := make(chan outcome, 2)
	for _, marker := range []string{"first", "second"} {
		go func(marker string) {
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText(marker)}, Headers: map[string]string{"X-Call": marker},
			})
			results <- outcome{marker: marker, result: result, err: err}
		}(marker)
	}
	for range 2 {
		got := <-results
		require.NoError(t, got.err)
		require.NotNil(t, got.result.Request)
		require.NotNil(t, got.result.Response)
		assert.Equal(t, got.marker, got.result.Response.Headers["x-call"])
		assert.Contains(t, string(got.result.Request.Body), got.marker)
		digest := sha256.Sum256(got.result.Request.Body)
		assert.Equal(t, hex.EncodeToString(digest[:]), got.result.Response.Headers["x-body-digest"])
	}
}

func TestModel_TransportMetadataAfterRetry(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "generate"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan []byte, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading request: %v", err)
				}
				requests <- body
				if len(requests) == 1 {
					w.Header().Set("Retry-After-Ms", "0")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"message":"retry","type":"rate_limit_error"}}`)
					return
				}
				w.Header().Set("X-Attempt", "second")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\ndata: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, transportTestResponse)
			}))
			defer server.Close()
			m := NewResponses("sdk-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(1)))
			params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
			var body []byte
			var headers map[string]string
			if streaming {
				result, err := m.DoStream(context.Background(), params)
				require.NoError(t, err)
				require.NotNil(t, result.Request)
				body, headers = result.Request.Body, result.Response.Headers
				for range result.Stream {
				}
			} else {
				result, err := m.DoGenerate(context.Background(), params)
				require.NoError(t, err)
				require.NotNil(t, result.Request)
				body, headers = result.Request.Body, result.Response.Headers
			}
			require.Len(t, requests, 2)
			<-requests
			assert.Equal(t, <-requests, body)
			assert.Equal(t, "second", headers["x-attempt"])
		})
	}
}
