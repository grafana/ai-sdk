package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/vertex"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestModel_CoreTransportContext(t *testing.T) {
	for _, entry := range []string{"stream", "generate", "agent-stream", "agent-generate"} {
		t.Run(entry, func(t *testing.T) {
			var body []byte
			var headers http.Header
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				body, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				headers = r.Header.Clone()
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
			if strings.HasPrefix(entry, "agent-") {
				assert.Contains(t, headers.Get("User-Agent"), "ai-sdk-agent/tool-loop")
			}
		})
	}
}

func TestModel_TransportRetryAndBodyOverride(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, nonJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/nonJSON=%t", streaming, nonJSON), func(t *testing.T) {
				var attempts atomic.Int32
				var bodies []string
				opts := []option.RequestOption{option.WithMaxRetries(1), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					bodies = append(bodies, string(body))
					response := transportResponse(r, streaming)
					if attempts.Add(1) == 1 {
						response.StatusCode = http.StatusServiceUnavailable
						response.Header.Set("Retry-After", "0")
						response.Header.Set("X-Transport", "failed-attempt")
						response.Body = io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"api_error","message":"retry"}}`))
					}
					return response, nil
				})})}
				if nonJSON {
					opts = append(opts, option.WithRequestBody("application/octet-stream", []byte("not JSON")))
				}
				m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(opts...))
				maxTokens := 64
				call := provider.CallOptions{MaxOutputTokens: &maxTokens}
				var request *provider.RequestMetadata
				var headers map[string]string
				if streaming {
					result, err := m.DoStream(t.Context(), call)
					require.NoError(t, err)
					request, headers = result.Request, result.Response.Headers
					for range result.Stream {
					}
				} else {
					result, err := m.DoGenerate(t.Context(), call)
					require.NoError(t, err)
					request, headers = result.Request, result.Response.Headers
				}
				require.Equal(t, int32(2), attempts.Load())
				require.Len(t, bodies, 2)
				assert.Equal(t, bodies[0], bodies[1])
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

func TestModel_MultiStepTransportContext(t *testing.T) {
	for _, agent := range []bool{false, true} {
		t.Run(fmt.Sprintf("agent=%t", agent), func(t *testing.T) {
			var bodies [][]byte
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				bodies = append(bodies, body)
				response := transportResponse(r, true)
				response.Header.Set("X-Transport-Step", fmt.Sprint(len(bodies)))
				if len(bodies) == 1 {
					events := []string{transportEvents[0], `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"weather","input":{}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`, `{"type":"message_stop"}`}
					response.Body = io.NopCloser(strings.NewReader(transportSSE(events)))
				}
				return response, nil
			})}
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			tools := aisdk.ToolSet{"weather": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`"sunny"`), nil
			}}}
			options := []aisdk.StreamOption{aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(2))}
			var steps []aisdk.StepResult
			if agent {
				a := aisdk.NewToolLoopAgent(m, aisdk.WithToolLoopAgentOptions(options...))
				result, err := a.Generate(t.Context(), aisdk.WithAgentPrompt("hi"))
				require.NoError(t, err)
				steps = result.Steps
			} else {
				options = append(options, aisdk.WithModelMessages(provider.UserText("hi")))
				result := aisdk.StreamText(t.Context(), m, options...)
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				steps = result.Steps()
			}
			require.Len(t, bodies, 2)
			require.Len(t, steps, 2)
			for i, body := range bodies {
				assert.JSONEq(t, string(body), string(steps[i].Request.Body))
				assert.Equal(t, fmt.Sprint(i+1), steps[i].Response.Headers["X-Transport-Step"])
			}
			assert.NotEqual(t, string(steps[0].Request.Body), string(steps[1].Request.Body))
		})
	}
}

func TestModel_VertexTransportContext(t *testing.T) {
	original := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = original })
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var body []byte
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "Bearer vertex-token", r.Header.Get("Authorization"))
				assert.Contains(t, r.URL.Path, "/projects/project/locations/us-east5/publishers/anthropic/models/")
				var err error
				body, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				return transportResponse(r, streaming), nil
			})}
			vertexGoogleAuth = func(ctx context.Context, region, project string, _ ...string) option.RequestOption {
				return option.Join(option.WithHTTPClient(client), vertexsdk.WithCredentials(ctx, region, project, &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "vertex-token"})}))
			}
			m, err := NewVertex(t.Context(), "us-east5", "project", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0)))
			require.NoError(t, err)
			maxTokens := 64
			var request *provider.RequestMetadata
			if streaming {
				result, err := m.DoStream(t.Context(), provider.CallOptions{MaxOutputTokens: &maxTokens})
				require.NoError(t, err)
				request = result.Request
				for range result.Stream {
				}
			} else {
				result, err := m.DoGenerate(t.Context(), provider.CallOptions{MaxOutputTokens: &maxTokens})
				require.NoError(t, err)
				request = result.Request
			}
			assert.JSONEq(t, string(body), string(request.Body))
			var wire map[string]any
			require.NoError(t, json.Unmarshal(body, &wire))
			assert.NotContains(t, wire, "model")
			assert.Equal(t, vertexsdk.DefaultVersion, wire["anthropic_version"])
		})
	}
}

func TestModel_FinalRetryRequestCapture(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var attempts int
			var bodies []string
			middleware := func(r *http.Request, next option.MiddlewareNext) (*http.Response, error) {
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
				attempts++
				wire["capture_attempt"] = json.RawMessage(fmt.Sprint(attempts))
				body, err = json.Marshal(wire)
				if err != nil {
					return nil, err
				}
				r.Body = io.NopCloser(strings.NewReader(string(body)))
				r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(body))), nil }
				r.ContentLength = int64(len(body))
				return next(r)
			}
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(1), option.WithMiddleware(middleware), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return nil, err
				}
				bodies = append(bodies, string(body))
				response := transportResponse(r, streaming)
				if len(bodies) == 1 {
					response.StatusCode = http.StatusServiceUnavailable
					response.Header.Set("Retry-After", "0")
					response.Body = io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"api_error","message":"retry"}}`))
				}
				return response, nil
			})})))
			var request *provider.RequestMetadata
			maxTokens := 64
			call := provider.CallOptions{MaxOutputTokens: &maxTokens}
			if streaming {
				result, err := m.DoStream(t.Context(), call)
				require.NoError(t, err)
				request = result.Request
				for range result.Stream {
				}
			} else {
				result, err := m.DoGenerate(t.Context(), call)
				require.NoError(t, err)
				request = result.Request
			}
			require.Len(t, bodies, 2)
			assert.NotEqual(t, bodies[0], bodies[1])
			assert.JSONEq(t, bodies[1], string(request.Body))
			assert.Contains(t, string(request.Body), `"capture_attempt":2`)
		})
	}
}

func TestModel_ConcurrentTransportContext(t *testing.T) {
	m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
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
