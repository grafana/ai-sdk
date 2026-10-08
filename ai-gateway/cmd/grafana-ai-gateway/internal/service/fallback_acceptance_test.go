package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
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
		{"provider call then error", fallbackParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{}`, ProviderExecuted: true}, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolCall, provider.PartError, provider.PartFinish}, []string{"overloaded"}},
		{"opaque MCP call then error", fallbackParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{}`, ProviderExecuted: true, ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"unconfigured","caller":null}`)}}, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolCall, provider.PartError, provider.PartFinish}, []string{"overloaded"}},
		{"tool call then error", fallbackParts(toolCall, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartToolCall, provider.PartError, provider.PartFinish}, []string{"overloaded"}},
		{"reasoning then malformed finish", fallbackParts(provider.StreamPart{Type: provider.PartReasoningStart, ID: "r"}, finish), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, []provider.StreamPartType{provider.PartStreamStart, provider.PartReasoningStart, provider.PartError}, []string{"internal_error"}},
		{"unsupported selected output", fallbackParts(provider.StreamPart{Type: provider.PartFile}), nil, 200, []fallback.AttemptOutcome{fallback.AttemptSelected}, failed, []string{"internal_error"}},
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
						switch tc.name {
						case "opaque MCP call then error":
							assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":"{}","providerExecuted":true,"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":"unconfigured","caller":null}}}`, payload)
						case "provider call then error":
							assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":"{}","providerExecuted":true}`, payload)
						default:
							assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":"{}"}`, payload)
						}
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
							assert.Contains(t, payload, `"message":"service overloaded"`)
							assert.Contains(t, payload, `"statusCode":503,"retryable":true`)
							assert.Contains(t, payload, `"outcome":"selected"`)
							assert.NotContains(t, payload, `"completion"`)
							assert.NotContains(t, payload, `"selectedAttempt"`)
						case "internal_error":
							assert.Contains(t, payload, `"message":"internal error"`)
							assert.Contains(t, payload, `"statusCode":500,"retryable":true`)
							assert.Contains(t, payload, `"outcome":"selected"`)
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
		contentType provider.GenerateContentType
		input       json.RawMessage
		status      int
	}{
		{"selected call", provider.ContentToolCall, json.RawMessage(`{}`), http.StatusOK},
		{"unsupported output", provider.ContentFile, json.RawMessage(`{}`), http.StatusInternalServerError},
		{"oversized selected call", provider.ContentToolCall, json.RawMessage(`{"payload":"` + strings.Repeat("x", 1<<20) + `"}`), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFallbackAcceptance(t, &observabilityTestModel{generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				assert.Len(t, opts.Tools, 1)
				return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: tc.contentType, ToolCallID: "call", ToolName: "lookup", Input: tc.input}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, nil
			}}, &observabilityTestModel{})
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(t.Context(), false, mappedFallbackBody))
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			h.verify(t, "generate", response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}

func TestFallbackAcceptance_UnarySelectionAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name           string
		primaryError   error
		secondaryError error
		status         int
		outcomes       []fallback.AttemptOutcome
	}{
		{"primary", nil, nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{"secondary", hostileFallbackError(503), nil, http.StatusOK, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{"nonretryable", hostileFallbackError(400), nil, http.StatusFailedDependency, []fallback.AttemptOutcome{fallback.AttemptFailed}},
		{"exhausted", hostileFallbackError(503), hostileFallbackError(503), http.StatusServiceUnavailable, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := func(err error) *observabilityTestModel {
				return &observabilityTestModel{generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
					assert.Equal(t, []provider.Message{provider.UserText("private-prompt")}, opts.Prompt)
					if err != nil {
						return nil, err
					}
					return &provider.GenerateResult{
						Content:          []provider.GenerateContentPart{{Type: provider.ContentText, Text: "public-answer", ProviderMetadata: hostileFallbackMetadata()}},
						Usage:            provider.Usage{Raw: json.RawMessage(`{"native_usage_marker":true}`)},
						FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
						ProviderMetadata: hostileFallbackMetadata(),
						Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"private-request-body"}`)},
						Response:         &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Provider: "primary-instance", ModelID: "backend-primary"}, Headers: map[string]string{"Authorization": "private-credential"}, Body: json.RawMessage(`{"secret":"private-response-body"}`)},
					}, nil
				}}
			}
			h := newFallbackAcceptance(t, candidate(tc.primaryError), candidate(tc.secondaryError))
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(context.Background(), false))
			assert.Equal(t, tc.status, response.Code)
			if tc.status == http.StatusOK {
				assert.Contains(t, response.Body.String(), "public-answer")
				assert.Contains(t, response.Body.String(), `"raw":{"native_usage_marker":true}`)
			}
			h.verify(t, "generate", response.Body.String(), tc.outcomes)
		})
	}
}

func TestFallbackAcceptance_StreamCommitmentAndPrivacy(t *testing.T) {
	valid := hostileFallbackParts()
	errorPart := provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}
	for _, tc := range []struct {
		name     string
		primary  *provider.StreamResult
		err      error
		outcomes []fallback.AttemptOutcome
		types    []string
	}{
		{"setup_failure", nil, hostileFallbackError(503), []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"premature_eof", fallbackParts(), nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"nil_result", nil, nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"nil_channel", &provider.StreamResult{}, nil, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"leading_errors", fallbackParts(append([]provider.StreamPart{errorPart, errorPart}, valid...)...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "error", "error", "response-metadata", "text-start", "text-delta", "text-end", "finish"}},
		{"later_error", fallbackParts(append(append([]provider.StreamPart{}, valid[:3]...), append([]provider.StreamPart{errorPart}, valid[3:]...)...)...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "error", "text-end", "finish"}},
		{"postcommit_close", fallbackParts(valid[:3]...), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "response-metadata", "text-start", "text-delta", "error"}},
		{"postcommit_error_close", fallbackParts(errorPart), nil, []fallback.AttemptOutcome{fallback.AttemptSelected}, []string{"stream-start", "error", "error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFallbackAcceptance(t,
				&observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return tc.primary, tc.err }},
				&observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return fallbackParts(valid...), nil
				}},
			)
			response := httptest.NewRecorder()
			h.handler.ServeHTTP(response, h.request(context.Background(), true))
			require.Equal(t, http.StatusOK, response.Code)
			var types []string
			for _, line := range strings.Split(response.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event struct {
					Type    string `json:"type"`
					ModelID string `json:"modelId"`
					Delta   string `json:"delta"`
				}
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
				types = append(types, event.Type)
				if event.Type == "response-metadata" {
					assert.Equal(t, "backend-primary", event.ModelID)
				}
				if event.Type == "text-delta" {
					assert.Equal(t, "public-answer", event.Delta)
				}
				if event.Type == "finish" {
					assert.Contains(t, line, `"raw":{"native_usage_marker":true}`)
				}
			}
			assert.Equal(t, tc.types, types)
			h.verify(t, "stream", response.Body.String(), tc.outcomes)
		})
	}
}

func TestFallbackAcceptance_CancellationBeforeSelection(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		for _, during := range []string{"setup", "first_part", "ready_result"} {
			if mode == "generate" && during == "first_part" {
				continue
			}
			t.Run(mode+"/"+during, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				entered := make(chan struct{})
				returned := make(chan struct{})
				primary := &observabilityTestModel{
					generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
						close(entered)
						if during == "ready_result" {
							cancel()
							close(returned)
							return &provider.GenerateResult{}, nil
						}
						<-ctx.Done()
						defer close(returned)
						return nil, hostileFallbackError(503)
					},
					stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
						close(entered)
						if during == "ready_result" {
							cancel()
							close(returned)
							return fallbackParts(hostileFallbackParts()...), nil
						}
						if during == "setup" {
							<-ctx.Done()
							defer close(returned)
							return nil, hostileFallbackError(503)
						}
						parts := make(chan provider.StreamPart)
						go func() { <-ctx.Done(); close(parts); close(returned) }()
						return &provider.StreamResult{Stream: parts}, nil
					},
				}
				h := newFallbackAcceptance(t, primary, &observabilityTestModel{})
				response := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { h.handler.ServeHTTP(response, h.request(ctx, mode == "stream")); close(done) }()
				awaitFallbackSignal(t, entered)
				cancel()
				awaitFallbackSignal(t, done)
				awaitFallbackSignal(t, returned)
				h.verify(t, mode, response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptCanceled})
			})
		}
	}
}

func TestFallbackAcceptance_CanceledBlockedConsumer(t *testing.T) {
	for _, scenario := range []string{"silent", "continuously_ready"} {
		t.Run(scenario, func(t *testing.T) {
			parts := make(chan provider.StreamPart)
			stop := make(chan struct{})
			producerDone := make(chan struct{})
			var consumed atomic.Int64
			go func() {
				defer close(producerDone)
				defer close(parts)
				select {
				case parts <- provider.StreamPart{Type: provider.PartStreamStart}:
				case <-stop:
					return
				}
				if scenario == "silent" {
					<-stop
					return
				}
				for {
					select {
					case parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "private-output"}:
						consumed.Add(1)
					case <-stop:
						return
					}
				}
			}()
			t.Cleanup(func() { close(stop); awaitFallbackSignal(t, producerDone) })
			h := newFallbackAcceptance(t, &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: parts}, nil
			}}, &observabilityTestModel{})
			ctx, cancel := context.WithCancel(h.context(context.Background()))
			defer cancel()
			result, err := h.model.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			if scenario == "continuously_ready" {
				require.Eventually(t, func() bool { return consumed.Load() >= 64 }, time.Second, time.Millisecond)
			}
			cancel()
			require.Eventually(t, func() bool {
				return countModelLogEvent(t, h.logText(), "aisdk.model.stream.cancelled") == 1
			}, time.Second, time.Millisecond, "logical cancellation must complete before downstream resumes reading")
			done := make(chan struct{})
			go func() {
				for range result.Stream {
				}
				close(done)
			}()
			awaitFallbackSignal(t, done)
			time.Sleep(150 * time.Millisecond)
			stoppedAt := consumed.Load()
			time.Sleep(30 * time.Millisecond)
			assert.Equal(t, stoppedAt, consumed.Load(), "fallback-owned draining must stop even while producer remains ready")
			h.verify(t, "stream", "", []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}

type fallbackAcceptance struct {
	model   provider.LanguageModel
	handler http.Handler
	logText func() string
	calls   [2]atomic.Int32
	context func(context.Context) context.Context
	verify  func(*testing.T, string, string, []fallback.AttemptOutcome)
}

func newFallbackAcceptance(t *testing.T, primary, secondary *observabilityTestModel) *fallbackAcceptance {
	t.Helper()
	env := testkit.NewEnv(t, func(cfg *agento11y.Config) {
		cfg.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		cfg.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs lockedBuffer
	var output physicalBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, 8, time.Second, time.Second)
	t.Cleanup(sink.Close)
	h := &fallbackAcceptance{}
	h.logText = logs.String
	catalog, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ nativemodel.Config, id string, _ *http.Client) provider.LanguageModel {
		index, model := 0, primary
		if id == "backend-secondary" {
			index, model = 1, secondary
		}
		return &observabilityTestModel{
			generate: func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				h.calls[index].Add(1)
				if model.generate == nil {
					return nil, errors.New("unexpected secondary invocation")
				}
				return model.generate(ctx, opts)
			},
			stream: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				h.calls[index].Add(1)
				if model.stream == nil {
					return nil, errors.New("unexpected secondary invocation")
				}
				return model.stream(ctx, opts)
			},
		}
	}, factory, sink)
	require.NoError(t, err)
	resolved, err := catalog.ResolveModel(context.Background(), "alias")
	require.NoError(t, err)
	h.model = resolved.Model
	h.handler, err = providerv4.New(providerv4.Config{Selector: providerv4.CatalogSelector(catalog), Limits: serviceTestLimits()})
	require.NoError(t, err)
	h.context = func(ctx context.Context) context.Context {
		state := &telemetryState{}
		state.observation.Store(&requestObservation{correlationID: "fallback-correlation"})
		return context.WithValue(ctx, telemetryStateKey{}, state)
	}
	h.verify = func(t *testing.T, mode, public string, outcomes []fallback.AttemptOutcome) {
		t.Helper()
		require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
		sink.Close()
		env.Shutdown(t)
		generation := env.SingleGenerationJSON(t)
		encoded, err := json.Marshal(generation)
		require.NoError(t, err)
		assert.Equal(t, "public", testkit.StringValue(t, generation, "model", "name"))
		assert.Equal(t, "fallback-correlation", testkit.StringValue(t, generation, "metadata", "gateway.correlation_id"))
		assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".start"))
		var metrics string
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			terminal := 0
			logSnapshot := logs.String()
			for _, event := range []string{"finish", "error", "cancelled"} {
				terminal += countModelLogEvent(t, logSnapshot, "aisdk.model."+mode+"."+event)
			}
			assert.Equal(collect, 1, terminal)
			metrics = testMetrics(t, telemetry)
			assert.Equal(collect, 1, strings.Count(metrics, "aisdk_model_requests_total{"))
		}, time.Second, time.Millisecond, "cancellation completes each observability stream independently")
		logical := string(encoded) + logs.String() + metrics
		for _, private := range []string{"private-prompt", "private-output", "public-answer", "primary-instance", "secondary-instance", "backend-primary", "backend-secondary", "native_usage_marker"} {
			assert.NotContains(t, logical, private)
		}
		for _, private := range []string{"private-credential", "private.example", "private-header", "private-request-body", "private-response-body", "private-error", "private-data"} {
			assert.NotContains(t, public+logical+output.String(), private)
		}
		assert.NotContains(t, logical+output.String(), "private-metadata")
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		require.Len(t, lines, len(outcomes))
		assert.Equal(t, int32(1), h.calls[0].Load())
		assert.Equal(t, int32(len(outcomes)-1), h.calls[1].Load())
		for index, line := range lines {
			var record physicalAttemptRecord
			require.NoError(t, json.Unmarshal([]byte(line), &record))
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(line), &fields))
			var keys []string
			for key := range fields {
				keys = append(keys, key)
			}
			assert.ElementsMatch(t, []string{"event", "correlation_id", "candidate_index", "provider_instance", "backend_model_id", "started_at", "decided_at", "outcome", "will_fallback", "winner"}, keys)
			assert.Equal(t, index+1, record.CandidateIndex)
			assert.Equal(t, []string{"primary-instance", "secondary-instance"}[index], record.ProviderInstance)
			assert.Equal(t, []string{"backend-primary", "backend-secondary"}[index], record.BackendModelID)
			assert.Equal(t, outcomes[index], record.Outcome)
			assert.Equal(t, index+1 < len(outcomes), record.WillFallback)
			assert.Equal(t, outcomes[index] == fallback.AttemptSelected, record.Winner)
			assert.Equal(t, "fallback-correlation", record.CorrelationID)
		}
	}
	return h
}

func (h *fallbackAcceptance) request(ctx context.Context, stream bool, bodies ...string) *http.Request {
	body := `{"prompt":[{"role":"user","content":[{"type":"text","text":"private-prompt"}]}]}`
	if len(bodies) > 0 {
		body = bodies[0]
	}
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body)).WithContext(h.context(ctx))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "private-credential")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "alias")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	if stream {
		request.Header.Set(providerv4.HeaderStreaming, "true")
	}
	return request
}

func hostileFallbackError(status int) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: status, Message: "private-error private-credential", URL: "https://private.example", ResponseHeaders: map[string][]string{"X-Private": {"private-header"}}, ResponseBody: "private-response-body", Data: json.RawMessage(`{"private":"private-data"}`)})
}

func hostileFallbackMetadata() provider.ProviderMetadata {
	return provider.ProviderMetadata{"anthropic": json.RawMessage(`{"private":"private-metadata"}`)}
}

func hostileFallbackParts() []provider.StreamPart {
	return []provider.StreamPart{
		{Type: provider.PartResponseMeta, ModelID: "backend-primary", Provider: "primary-instance", ResponseHeaders: map[string]string{"Authorization": "private-credential"}, ProviderMetadata: hostileFallbackMetadata()},
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "public-answer", ProviderMetadata: hostileFallbackMetadata()},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}, Usage: &provider.Usage{Raw: json.RawMessage(`{"native_usage_marker":true}`)}, ProviderMetadata: hostileFallbackMetadata()},
	}
}

func fallbackParts(parts ...provider.StreamPart) *provider.StreamResult {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream, Request: &provider.RequestMetadata{Body: json.RawMessage(`{"private":"private-request-body"}`)}, Response: &provider.ResponseHeaders{Headers: map[string]string{"Authorization": "private-credential"}}}
}

func awaitFallbackSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("composed fallback lifecycle exceeded its cleanup bound")
	}
}

func TestFallbackAcceptance_MetadataFailureDoesNotReplay(t *testing.T) {
	for _, selectedSecondary := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("secondary=%v/stream=%v", selectedSecondary, streaming), func(t *testing.T) {
				metadata := provider.ProviderMetadata{"future": json.RawMessage(`null`)}
				invalid := &observabilityTestModel{
					generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "paid"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, ProviderMetadata: metadata}, nil
					},
					stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						return fallbackParts(provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata}), nil
					},
				}
				primary, secondary := invalid, &observabilityTestModel{}
				outcomes := []fallback.AttemptOutcome{fallback.AttemptSelected}
				if selectedSecondary {
					primary = &observabilityTestModel{
						generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
							return nil, hostileFallbackError(503)
						},
						stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
							return nil, hostileFallbackError(503)
						},
					}
					secondary = invalid
					outcomes = []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}
				}
				h := newFallbackAcceptance(t, primary, secondary)
				w := httptest.NewRecorder()
				h.handler.ServeHTTP(w, h.request(t.Context(), streaming))
				mode := "generate"
				if streaming {
					mode = "stream"
					assert.Equal(t, http.StatusOK, w.Code)
					assert.NotContains(t, w.Body.String(), `"type":"text-start"`)
					assert.Contains(t, w.Body.String(), `"type":"error"`)
				} else {
					assert.Equal(t, http.StatusInternalServerError, w.Code)
				}
				assert.NotContains(t, w.Body.String(), "future")
				h.verify(t, mode, w.Body.String(), outcomes)
			})
		}
	}
}
