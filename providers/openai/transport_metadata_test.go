package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_IncompleteRequestCaptureIsAbsent(t *testing.T) {
	const responseBody = `{"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		_, _ = io.ReadFull(req.Body, make([]byte, 1))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    req,
		}, nil
	})}
	m := NewResponses("sdk-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	result, err := m.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	assert.Nil(t, result.Request)
	assert.JSONEq(t, responseBody, string(result.Response.Body))
}

func TestModel_TransportMetadata(t *testing.T) {
	const responseBody = `{"id":"resp_123","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello!","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`
	for _, streaming := range []bool{false, true} {
		for _, configured := range []bool{false, true} {
			name := "default"
			if configured {
				name = "configured-client"
			}
			if streaming {
				name += "/stream"
			} else {
				name += "/generate"
			}
			t.Run(name, func(t *testing.T) {
				requests := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("reading outbound body: %v", err)
					}
					requests <- body
					w.Header().Set("X-Response-Proof", "actual-header")
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, responseBody)
				}))
				defer server.Close()
				var m provider.LanguageModel
				if configured {
					client := openaisdk.NewClient(option.WithAPIKey("sdk-key"), option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0))
					m = NewResponsesWithClient(client, "gpt-4o")
				} else {
					m = NewResponses("sdk-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
				}
				params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
				var returnedBody []byte
				var returnedHeaders map[string]string
				if streaming {
					result, err := m.DoStream(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					require.NotNil(t, result.Response)
					returnedBody = result.Request.Body
					returnedHeaders = result.Response.Headers
					for range result.Stream {
					}
				} else {
					result, err := m.DoGenerate(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					require.NotNil(t, result.Response)
					returnedBody = result.Request.Body
					returnedHeaders = result.Response.Headers
					assert.JSONEq(t, responseBody, string(result.Response.Body))
				}
				assert.Equal(t, "actual-header", returnedHeaders["x-response-proof"])
				select {
				case transmitted := <-requests:
					assert.Equal(t, transmitted, returnedBody)
					assert.JSONEq(t, string(transmitted), string(returnedBody))
				default:
					t.Fatal("request not received")
				}
			})
		}
	}
}
