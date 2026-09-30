package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_CoreTransportContext(t *testing.T) {
	for _, entry := range []string{"stream", "generate", "agent-stream", "agent-generate"} {
		t.Run(entry, func(t *testing.T) {
			var body []byte
			m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				body, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				return transportResponse(r, true), nil
			})})))
			var steps []aisdk.StepResult
			var response aisdk.ResponseMetadata
			switch entry {
			case "generate":
				result, err := aisdk.GenerateText(t.Context(), m, aisdk.WithModelMessages(provider.UserText("hi")))
				require.NoError(t, err)
				steps, response = result.Steps, result.Response
			case "agent-generate":
				result, err := aisdk.NewToolLoopAgent(m).Generate(t.Context(), aisdk.WithAgentPrompt("hi"))
				require.NoError(t, err)
				steps, response = result.Steps, result.Response
			default:
				var result *aisdk.StreamTextResult
				if entry == "stream" {
					result = aisdk.StreamText(t.Context(), m, aisdk.WithModelMessages(provider.UserText("hi")))
				} else {
					result = aisdk.NewToolLoopAgent(m).Stream(t.Context(), aisdk.WithAgentPrompt("hi"))
				}
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				steps, response = result.Steps(), result.Response()
			}
			require.Len(t, steps, 1)
			assert.JSONEq(t, string(body), string(steps[0].Request.Body))
			assert.Equal(t, "first, second", steps[0].Response.Headers["X-Transport"])
			assert.Equal(t, steps[0].Response.Headers, response.Headers)
		})
	}
}

func TestModel_TransportRetryAndBodyOverride(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, nonJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/nonJSON=%t", streaming, nonJSON), func(t *testing.T) {
				var attempts atomic.Int32
				var bodies []string
				opts := []option.RequestOption{option.WithMaxRetries(1), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					bodies = append(bodies, string(body))
					assert.Equal(t, "Bearer supplied-key", r.Header.Get("Authorization"))
					response := transportResponse(r, streaming)
					if attempts.Add(1) == 1 {
						response.StatusCode = http.StatusServiceUnavailable
						response.Header.Set("Retry-After", "0")
						response.Header.Set("X-Transport", "failed-attempt")
						response.Body = io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"retry"}}`))
					}
					return response, nil
				})})}
				if !nonJSON {
					var attempt int
					opts = append(opts, option.WithMiddleware(func(r *http.Request, next option.MiddlewareNext) (*http.Response, error) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							return nil, err
						}
						if err := r.Body.Close(); err != nil {
							return nil, err
						}
						var wire map[string]json.RawMessage
						if err := json.Unmarshal(body, &wire); err != nil {
							return nil, err
						}
						attempt++
						wire["capture_attempt"] = json.RawMessage(fmt.Sprint(attempt))
						body, err = json.Marshal(wire)
						if err != nil {
							return nil, err
						}
						r.Body = io.NopCloser(strings.NewReader(string(body)))
						r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(body))), nil }
						r.ContentLength = int64(len(body))
						return next(r)
					}))
				}
				if nonJSON {
					opts = append(opts, option.WithRequestBody("application/octet-stream", []byte("not JSON")))
				}
				client := openaisdk.NewClient(append([]option.RequestOption{option.WithAPIKey("supplied-key")}, opts...)...)
				m := NewResponsesWithClient(client, "gpt-4o")
				var request *provider.RequestMetadata
				var headers map[string]string
				if streaming {
					result, err := m.DoStream(t.Context(), provider.CallOptions{})
					require.NoError(t, err)
					request, headers = result.Request, result.Response.Headers
					for range result.Stream {
					}
				} else {
					result, err := m.DoGenerate(t.Context(), provider.CallOptions{})
					require.NoError(t, err)
					request, headers = result.Request, result.Response.Headers
				}
				require.Equal(t, int32(2), attempts.Load())
				require.Len(t, bodies, 2)
				if nonJSON {
					assert.Equal(t, bodies[0], bodies[1])
				} else {
					assert.NotEqual(t, bodies[0], bodies[1])
					assert.Contains(t, string(request.Body), `"capture_attempt":2`)
				}
				assert.Equal(t, "first, second", headers["X-Transport"])
				if nonJSON && !streaming {
					assert.Equal(t, "not JSON", bodies[1])
					assert.Empty(t, request.Body)
				} else {
					assert.JSONEq(t, bodies[1], string(request.Body))
				}
			})
		}
	}
}

func TestModel_ConcurrentTransportContext(t *testing.T) {
	m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		response := transportResponse(r, true)
		response.Header.Set("X-Captured-Body", string(body))
		response.Header.Set("X-Transport", r.Header.Get("X-Call"))
		return response, nil
	})})))
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			result, err := m.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText(t.Name())}, Headers: map[string]string{"X-Call": t.Name()}})
			require.NoError(t, err)
			for range result.Stream {
			}
			assert.Equal(t, t.Name(), result.Response.Headers["X-Transport"])
			assert.JSONEq(t, result.Response.Headers["X-Captured-Body"], string(result.Request.Body))
			assert.Contains(t, string(result.Request.Body), t.Name())
		})
	}
}
