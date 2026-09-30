package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDoStream_RawTransportEvents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		events     []string
		raw        []string
		errorFrame bool
	}{
		{name: "normal", events: transportEvents, raw: transportEvents},
		{name: "ping and ignored", events: []string{`{"type":"ping"}`, transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[4], transportEvents[5]}, raw: []string{`{"type":"ping"}`, transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[4], transportEvents[5]}},
		{name: "invalid JSON", events: []string{transportEvents[0], `not JSON`}, raw: []string{transportEvents[0], ""}, errorFrame: true},
		{name: "permissive SDK decoding", events: []string{transportEvents[0], `{"type":42}`}, raw: []string{transportEvents[0], `{"type":42}`}},
		{name: "normalization failure", events: []string{transportEvents[0], `{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_01":{"type":1,"explanation":"private"}}}}]},"usage":{"output_tokens":1}}`}, raw: []string{transportEvents[0], `{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_01":{"type":1,"explanation":"private"}}}}]},"usage":{"output_tokens":1}}`}, errorFrame: true},
		{name: "null", events: []string{transportEvents[0], `null`}, raw: []string{transportEvents[0], `null`}},
		{name: "error", events: []string{transportEvents[0], `{"type":"error","error":{"message":"failed","type":"api_error"}}`}, raw: []string{transportEvents[0], `{"type":"error","error":{"message":"failed","type":"api_error"}}`}, errorFrame: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var normalizedJSON []byte
			for _, includeRaw := range []bool{false, true} {
				m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					response := transportResponse(r, true)
					response.Body = io.NopCloser(strings.NewReader(": comment\n\n" + transportSSE(tc.events) + "data: [DONE]\n\n"))
					return response, nil
				})})))
				result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: includeRaw})
				require.NoError(t, err)
				var raws []json.RawMessage
				var parts, normalized []provider.StreamPart
				var errorCount int
				var latestRaw json.RawMessage
				for part := range result.Stream {
					if includeRaw && tc.name == "normal" && part.Type != provider.PartRaw {
						if want := map[provider.StreamPartType]string{provider.PartResponseMeta: transportEvents[0], provider.PartTextStart: transportEvents[1], provider.PartTextDelta: transportEvents[2], provider.PartTextEnd: transportEvents[3], provider.PartFinish: transportEvents[4]}[part.Type]; want != "" {
							assert.JSONEq(t, want, string(latestRaw))
						}
					}
					parts = append(parts, part)
					if part.Type == provider.PartRaw {
						raws = append(raws, part.RawValue)
						latestRaw = part.RawValue
					} else {
						normalized = append(normalized, part)
					}
					if part.Type == provider.PartError {
						errorCount++
					}
					if includeRaw && tc.errorFrame && part.Type == provider.PartError {
						require.GreaterOrEqual(t, len(parts), 2)
						assert.Equal(t, provider.PartRaw, parts[len(parts)-2].Type)
					}
				}
				if tc.errorFrame {
					assert.Positive(t, errorCount)
				}
				encoded, err := json.Marshal(normalized)
				require.NoError(t, err)
				if !includeRaw {
					normalizedJSON = encoded
				} else {
					assert.JSONEq(t, string(normalizedJSON), string(encoded))
				}
				require.NotEmpty(t, parts)
				assert.Equal(t, provider.PartStreamStart, parts[0].Type)
				if !includeRaw {
					assert.Empty(t, raws)
					continue
				}
				require.Len(t, raws, len(tc.raw))
				for i, want := range tc.raw {
					if want == "" {
						assert.Nil(t, raws[i])
					} else {
						assert.JSONEq(t, want, string(raws[i]))
					}
				}
			}
		})
	}
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
