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

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/vertex"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const transportMessage = `{"id":"msg_transport","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"unknown":{"retained":true}}`

var transportEvents = []string{
	`{"type":"message_start","message":{"id":"msg_transport","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
	`{"type":"message_stop"}`,
}

func transportSSE(events []string) string {
	var out strings.Builder
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(event), &envelope)
		if envelope.Type == "" {
			envelope.Type = "message_delta"
		}
		fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", envelope.Type, event)
	}
	return out.String()
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func transportResponse(r *http.Request, streaming bool) *http.Response {
	payload, contentType := transportMessage, "application/json"
	if streaming {
		payload, contentType = transportSSE(transportEvents), "text/event-stream"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}, "X-Transport": {"first", "second"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}
}

func TestModel_TransportContext(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var captured json.RawMessage
			var headers http.Header
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				captured, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				headers = r.Header.Clone()
				return transportResponse(r, streaming), nil
			})}
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0), option.WithHeader("X-Configured", "configured"), option.WithHeader("X-Shared", "configured"), option.WithHeader("anthropic-beta", "CONFIG-beta, shared-beta"), option.WithJSONSet("capture_marker", "body-option")))
			maxTokens := 64
			opts := provider.CallOptions{MaxOutputTokens: &maxTokens, Prompt: []provider.Message{provider.UserText("hi")}, Headers: map[string]string{"X-Call": "call", "X-Shared": "call", "Anthropic-Beta": "CALL-beta,SHARED-beta"}, ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: []string{"feature-beta"}})}
			var request *provider.RequestMetadata
			var responseHeaders map[string]string
			if streaming {
				result, err := m.DoStream(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				require.NotNil(t, result.Response)
				responseHeaders = result.Response.Headers
				var metadata int
				for part := range result.Stream {
					if part.Type == provider.PartResponseMeta {
						metadata++
						assert.Equal(t, responseHeaders, part.ResponseHeaders)
					}
				}
				assert.Equal(t, 1, metadata)
			} else {
				result, err := m.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				responseHeaders = result.Response.Headers
				assert.JSONEq(t, transportMessage, string(result.Response.Body))
			}
			require.NotNil(t, request)
			assert.JSONEq(t, string(captured), string(request.Body))
			assert.Equal(t, "first, second", responseHeaders["X-Transport"])
			assert.Equal(t, "configured", headers.Get("X-Configured"))
			assert.Equal(t, "call", headers.Get("X-Call"))
			assert.Equal(t, "call", headers.Get("X-Shared"))
			assert.ElementsMatch(t, []string{"config-beta", "shared-beta", "call-beta", "feature-beta"}, strings.Split(strings.ReplaceAll(strings.Join(headers.Values("anthropic-beta"), ","), " ", ""), ","))
			assert.Equal(t, "test-key", headers.Get("X-Api-Key"))
			assert.Equal(t, "CALL-beta,SHARED-beta", opts.Headers["Anthropic-Beta"])
		})
	}
}

func TestModel_EmptyBetaTokens(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, feature := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/feature=%t", streaming, feature), func(t *testing.T) {
				var header http.Header
				m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithHeader("anthropic-beta", " , , "), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					header = r.Header.Clone()
					return transportResponse(r, streaming), nil
				})})))
				callHeaders := map[string]string{"ANTHROPIC-BETA": " , "}
				maxTokens := 64
				opts := provider.CallOptions{Headers: callHeaders, MaxOutputTokens: &maxTokens}
				if feature {
					opts.ProviderOptions = provider.BuildProviderOptions(AnthropicOptions{Betas: []string{"feature-beta"}})
				}
				if streaming {
					result, err := m.DoStream(t.Context(), opts)
					require.NoError(t, err)
					for range result.Stream {
					}
				} else {
					_, err := m.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
				}
				if feature {
					assert.Equal(t, "feature-beta", header.Get("anthropic-beta"))
				} else {
					assert.Empty(t, header.Values("anthropic-beta"))
				}
				assert.Equal(t, " , ", callHeaders["ANTHROPIC-BETA"])
			})
		}
	}
}

func TestSDK_RawStreamingRequestEquivalence(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%t", failure), func(t *testing.T) {
			type capture struct {
				body    string
				headers http.Header
				path    string
			}
			var requests []capture
			client := sdk.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey("test-key"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				requests = append(requests, capture{string(body), r.Header.Clone(), r.URL.RequestURI()})
				response := transportResponse(r, true)
				if failure {
					response.Body = io.NopCloser(strings.NewReader("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"failed\"}}\n\n"))
				}
				return response, nil
			})}))
			params, _, _, br, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: []string{"feature-beta"}})}, true)
			require.NoError(t, err)
			stream := client.Beta.Messages.NewStreaming(t.Context(), params, br.requestOptions...)
			first := stream.Next()
			initialErr := stream.Err()
			require.NoError(t, stream.Close())
			var response *http.Response
			opts := append([]option.RequestOption(nil), br.requestOptions...)
			for _, beta := range params.Betas {
				opts = append(opts, option.WithHeaderAdd("anthropic-beta", string(beta)))
			}
			opts = append(opts, option.WithJSONSet("stream", true))
			require.NoError(t, client.Post(context.Background(), "v1/messages?beta=true", params, &response, opts...))
			decoder := ssestream.NewDecoder(response)
			rawStream := ssestream.NewStream[sdk.BetaRawMessageStreamEventUnion](decoder, nil)
			assert.Equal(t, first, rawStream.Next())
			if failure {
				want := wrapInitialStreamError(initialErr, params)
				got := wrapInitialStreamError(rawStream.Err(), params)
				var wantAPI, gotAPI *provider.APICallError
				require.ErrorAs(t, want, &wantAPI)
				require.ErrorAs(t, got, &gotAPI)
				assert.Equal(t, wantAPI.StatusCode, gotAPI.StatusCode)
				assert.Equal(t, wantAPI.ResponseBody, gotAPI.ResponseBody)
				assert.Equal(t, wantAPI.IsRetryable, gotAPI.IsRetryable)
			}
			require.NoError(t, rawStream.Close())
			require.Len(t, requests, 2)
			assert.JSONEq(t, requests[0].body, requests[1].body)
			assert.Equal(t, requests[0].headers, requests[1].headers)
			assert.Equal(t, requests[0].path, requests[1].path)
		})
	}
}

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
