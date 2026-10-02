package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mappedFallbackBody = `{"maxOutputTokens":64,"tools":[{"type":"function","name":"lookup","inputSchema":{}}],"toolChoice":{"type":"required"},"reasoning":"high","headers":{"x-ordinary":"value"},"providerOptions":{"vendor":{"nested":[null,false,0,"",[],{}]}},"prompt":[{"role":"user","content":[{"type":"text","text":"private-prompt"},{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain","filename":""}]}]}`

func TestFallbackAcceptance_MappedPrecommitAndCommitment(t *testing.T) {
	finish := provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	toolCall := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{}`}
	finished := []provider.StreamPartType{provider.PartStreamStart, provider.PartFinish}
	failed := []provider.StreamPartType{provider.PartStreamStart, provider.PartError}
	for _, tc := range []struct {
		name       string
		result     *provider.StreamResult
		err        error
		status     int
		outcomes   []fallback.AttemptOutcome
		types      []provider.StreamPartType
		errorCodes []string
	}{
		{"setup failure", nil, hostileFallbackError(503), 200, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, finished, nil},
		{"empty prepart", fallbackParts(), nil, 200, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, finished, nil},
		{"nil result", nil, nil, 200, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, finished, nil},
		{"nil channel", &provider.StreamResult{}, nil, 200, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, finished, nil},
		{"result plus error", fallbackParts(toolCall, finish), hostileFallbackError(503), 200, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, finished, nil},
		{"noneligible", nil, hostileFallbackError(400), 424, []fallback.AttemptOutcome{fallback.AttemptFailed}, nil, nil},
		{"canceled", nil, context.Canceled, 499, []fallback.AttemptOutcome{fallback.AttemptCanceled}, nil, nil},
		{"start then premature end", fallbackParts(provider.StreamPart{Type: provider.PartStreamStart}), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, failed, []string{"internal_error"}},
		{"first error", fallbackParts(provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartError, provider.PartFinish}, []string{"overloaded"}},
		{"tool input then error", fallbackParts(provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"}, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolInputStart, provider.PartError, provider.PartError}, []string{"overloaded", "internal_error"}},
		{"tool call then error", fallbackParts(toolCall, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolCall, provider.PartError, provider.PartFinish}, []string{"overloaded"}},
		{"reasoning then malformed finish", fallbackParts(provider.StreamPart{Type: provider.PartReasoningStart, ID: "r"}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartReasoningStart, provider.PartError}, []string{"internal_error"}},
		{"unsupported selected output", fallbackParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{}`, ProviderExecuted: true}), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, failed, []string{"internal_error"}},
		{"oversized selected call", fallbackParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: strings.Repeat("x", 1<<20)}), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, failed, []string{"internal_error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			h := newFallbackAcceptance(t, &observabilityTestModel{stream: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Len(t, opts.Tools, 1)
				assert.Equal(t, provider.ReasoningHigh, opts.Reasoning)
				assert.Equal(t, "value", opts.Headers["x-ordinary"])
				assert.Equal(t, provider.ContentPartTypeFile, opts.Prompt[0].Content[1].Type)
				if tc.err == context.Canceled {
					cancel()
				}
				return tc.result, tc.err
			}}, &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				assert.Len(t, tc.outcomes, 2, "selected or noneligible primary must never replay")
				return fallbackParts(finish), nil
			}})
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(ctx, true, mappedFallbackBody))
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			var types []provider.StreamPartType
			var errorCodes []string
			if tc.status == http.StatusOK {
				assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
				require.True(t, strings.HasSuffix(response.Body.String(), "\n\n"))
				for _, frame := range strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n") {
					require.True(t, strings.HasPrefix(frame, "data: "), frame)
					payload := strings.TrimPrefix(frame, "data: ")
					var event struct {
						Type provider.StreamPartType `json:"type"`
					}
					require.NoError(t, json.Unmarshal([]byte(payload), &event))
					types = append(types, event.Type)
					switch event.Type {
					case provider.PartStreamStart:
						assert.JSONEq(t, `{"type":"stream-start","warnings":[]}`, payload)
					case provider.PartToolInputStart:
						assert.JSONEq(t, `{"type":"tool-input-start","id":"call","toolName":"lookup"}`, payload)
					case provider.PartToolCall:
						assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":"{}"}`, payload)
					case provider.PartReasoningStart:
						assert.JSONEq(t, `{"type":"reasoning-start","id":"r"}`, payload)
					case provider.PartError:
						var errorEvent struct {
							Error struct {
								Code string `json:"code"`
							} `json:"error"`
						}
						require.NoError(t, json.Unmarshal([]byte(payload), &errorEvent))
						errorCodes = append(errorCodes, errorEvent.Error.Code)
						switch errorEvent.Error.Code {
						case "overloaded":
							assert.JSONEq(t, `{"type":"error","error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded","statusCode":503,"retryable":true}}`, payload)
						case "internal_error":
							assert.JSONEq(t, `{"type":"error","error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true}}`, payload)
						}
					}
				}
			} else {
				assert.NotContains(t, response.Body.String(), "data: ")
			}
			assert.Equal(t, tc.types, types)
			assert.Equal(t, tc.errorCodes, errorCodes)
			h.verify(t, "stream", response.Body.String(), tc.outcomes)
		})
	}
}

func TestFallbackAcceptance_MappedSelection(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name           string
			primaryError   error
			secondaryError error
			status         int
			outcomes       []fallback.AttemptOutcome
		}{
			{"primary", nil, nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptSelected}},
			{"secondary", hostileFallbackError(503), nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
			{"noneligible", hostileFallbackError(400), nil, http.StatusFailedDependency, []fallback.AttemptOutcome{fallback.AttemptFailed}},
			{"exhausted", hostileFallbackError(503), hostileFallbackError(503), http.StatusServiceUnavailable, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptFailed}},
			{"canceled", context.Canceled, nil, 499, []fallback.AttemptOutcome{fallback.AttemptCanceled}},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				candidate := func(err error) *observabilityTestModel {
					invoke := func(opts provider.CallOptions) error {
						assert.Len(t, opts.Tools, 1)
						assert.Equal(t, provider.ToolChoiceRequired, opts.ToolChoice.Type)
						assert.Equal(t, provider.ReasoningHigh, opts.Reasoning)
						assert.Equal(t, "value", opts.Headers["x-ordinary"])
						assert.Equal(t, provider.ContentPartTypeFile, opts.Prompt[0].Content[1].Type)
						if err == context.Canceled {
							cancel()
						}
						return err
					}
					return &observabilityTestModel{
						generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
							if err := invoke(opts); err != nil {
								return nil, err
							}
							return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
						},
						stream: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
							if err := invoke(opts); err != nil {
								return nil, err
							}
							return fallbackParts(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}), nil
						},
					}
				}
				h := newFallbackAcceptance(t, candidate(tc.primaryError), candidate(tc.secondaryError))
				response := httptest.NewRecorder()
				h.handler.ServeHTTP(response, h.request(ctx, streaming, mappedFallbackBody))
				assert.Equal(t, tc.status, response.Code, response.Body.String())
				mode := "generate"
				if streaming {
					mode = "stream"
				}
				h.verify(t, mode, response.Body.String(), tc.outcomes)
			})
		}
	}
}

func TestFallbackAcceptance_MappedUnaryEncodingDoesNotReplay(t *testing.T) {
	for _, tc := range []struct {
		name        string
		unsupported bool
		input       json.RawMessage
		status      int
	}{
		{"selected call", false, json.RawMessage(`{}`), http.StatusOK},
		{"unsupported output", true, json.RawMessage(`{}`), http.StatusInternalServerError},
		{"oversized selected call", false, json.RawMessage(`{"payload":"` + strings.Repeat("x", 1<<20) + `"}`), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFallbackAcceptance(t, &observabilityTestModel{generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				assert.Len(t, opts.Tools, 1)
				return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "lookup", Input: tc.input, ProviderExecuted: tc.unsupported}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, nil
			}}, &observabilityTestModel{})
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(t.Context(), false, mappedFallbackBody))
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			h.verify(t, "generate", response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}
