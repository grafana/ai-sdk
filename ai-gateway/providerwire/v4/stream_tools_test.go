package v4

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingTools_OpenInputTermination(t *testing.T) {
	for _, scenario := range []string{"cancel", "idle timeout", "total timeout", "write failure", "part budget", "premature EOF"} {
		t.Run(scenario, func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = time.Second
			limits.StreamIdleDuration = time.Second
			if scenario == "idle timeout" {
				limits.StreamIdleDuration = 25 * time.Millisecond
			}
			if scenario == "total timeout" {
				limits.ModelDuration = 25 * time.Millisecond
			}
			if scenario == "part budget" {
				limits.StreamParts = 3
			}
			harness := newRuntimeHarness(t, limits)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			producerDone := make(chan struct{})
			handlerDone := make(chan struct{})
			var modelContext context.Context
			harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				modelContext = ctx
				parts := make(chan provider.StreamPart)
				go func() {
					defer close(producerDone)
					defer close(parts)
					parts <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "open", ToolName: "weather"}
					if scenario == "premature EOF" {
						return
					}
					if scenario == "part budget" || scenario == "write failure" {
						for i := 0; i < 8; i++ {
							if ctx.Err() != nil {
								break
							}
							select {
							case parts <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "open", Delta: ""}:
							case <-ctx.Done():
								i = 8
							}
						}
					}
					<-ctx.Done()
					if scenario == "cancel" {
						<-handlerDone
					}
					if scenario != "part budget" {
						parts <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "open", Delta: "late"}
					}
				}()
				return &provider.StreamResult{Stream: parts}, nil
			}
			writer := &responseWriterProbe{}
			writer.onWrite = func() {
				if scenario == "write failure" && writer.writes == 3 {
					writer.writeErr = errors.New("private writer failure")
				}
			}
			writer.onFlush = func(int) {
				if scenario == "cancel" && strings.Contains(writer.body.String(), `"type":"tool-input-start"`) {
					cancel()
				}
			}
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(ctx))
			close(handlerDone)
			select {
			case <-producerDone:
			case <-time.After(time.Second):
				t.Fatal("tool producer retained after terminal handling")
			}
			require.Error(t, modelContext.Err())
			body := writer.body.String()
			assert.Contains(t, body, `"type":"tool-input-start"`)
			assert.NotContains(t, body, `"type":"finish"`)
			assert.NotContains(t, body, "late")
			assert.NotContains(t, body, "private")
			if scenario == "write failure" {
				assert.Equal(t, 3, writer.writes)
				assert.NotContains(t, body, `"type":"error"`)
			} else {
				assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
				code := "internal_error"
				if scenario == "cancel" {
					code = "canceled"
				} else if strings.Contains(scenario, "timeout") {
					code = "timeout"
				}
				assert.Contains(t, body, `"code":"`+code+`"`)
			}
		})
	}
}

func TestStreamingTools_ConcurrentReadyCancellation(t *testing.T) {
	for _, delta := range []string{"", "{}"} {
		t.Run("delta="+delta, func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = time.Second
			limits.StreamIdleDuration = time.Second
			harness := newRuntimeHarness(t, limits)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "open", ToolName: "weather"}
			handlerDone := make(chan struct{})
			producerDone := make(chan struct{})
			var modelContext context.Context
			harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				modelContext = ctx
				go func() {
					defer close(producerDone)
					defer close(parts)
					<-ctx.Done()
					<-handlerDone
					for range 2 {
						parts <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "open", Delta: "post-terminal"}
					}
				}()
				return &provider.StreamResult{Stream: parts}, nil
			}
			writer := &responseWriterProbe{}
			writer.onFlush = func(int) {
				if writer.writes == 2 {
					parts <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "open", Delta: delta}
					cancel()
				}
			}
			go func() {
				harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(ctx))
				close(handlerDone)
			}()
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Fatal("concurrent-ready cancellation retained handler")
			}
			select {
			case <-producerDone:
			case <-time.After(time.Second):
				t.Fatal("post-terminal tool producer was not drained")
			}
			require.Eventually(t, func() bool { return len(parts) == 0 }, time.Second, time.Millisecond)
			require.ErrorIs(t, modelContext.Err(), context.Canceled)
			prefix := string(canonicalEmptyStartFrame) + "data: {\"type\":\"tool-input-start\",\"id\":\"open\",\"toolName\":\"weather\"}\n\n"
			deltaFrame := "data: {\"type\":\"tool-input-delta\",\"id\":\"open\",\"delta\":\"" + delta + "\"}\n\n"
			terminal := string(canonicalCancellationStreamErrorFrame)
			assert.Contains(t, []string{prefix + terminal, prefix + deltaFrame + terminal}, writer.body.String())
			requireStreamBodyMatchesSchema(t, writer.body.String())
		})
	}
}

func TestStreamingTools_InterleavedInputAndBasicResults(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "first"},
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "b", ToolName: "second"},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "a", Delta: ""},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "b", Delta: "{}"},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "b"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "b", ToolName: "second", Input: "{}"},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "b", ToolName: "second", Result: json.RawMessage("false")},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "first", Input: ""},
			finishPart(),
		)}, nil
	}
	response := harness.serve(streamRequest(`{"prompt":[],"tools":[{"type":"function","name":"first","inputSchema":{}}]}`))
	body := response.Body.String()
	require.Contains(t, body, `"type":"tool-input-start","id":"a","toolName":"first"`)
	require.Contains(t, body, `"type":"tool-input-delta","id":"a","delta":""`)
	require.Contains(t, body, `"type":"tool-result","toolCallId":"b","toolName":"second","result":false`)
	assert.NotContains(t, body, `"type":"error"`)
	requireStreamBodyMatchesSchema(t, body)
}

func TestStreamingTools_InvalidTransitions(t *testing.T) {
	start := provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "f"}
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "f", Input: "{}"}
	end := provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"}
	result := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "f", Result: json.RawMessage("{}")}
	yes := true
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"delta without start", []provider.StreamPart{{Type: provider.PartToolInputDelta, ID: "a"}}},
		{"end without start", []provider.StreamPart{end}},
		{"duplicate start", []provider.StreamPart{start, start}},
		{"call before end", []provider.StreamPart{start, call}},
		{"duplicate call", []provider.StreamPart{call, call}},
		{"unmatched result", []provider.StreamPart{result}},
		{"duplicate result", []provider.StreamPart{call, result, result}},
		{"mismatched name", []provider.StreamPart{start, end, {Type: provider.PartToolCall, ToolCallID: "a", ToolName: "other", Input: "{}"}}},
		{"open at finish", []provider.StreamPart{start, finishPart()}},
		{"late metadata", []provider.StreamPart{call, {Type: provider.PartResponseMeta}}},
		{"enabled execution", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "f", Input: "{}", ProviderExecuted: true}}},
		{"enabled dynamic", []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "a", ToolName: "f", Dynamic: &yes}}},
		{"preliminary result", []provider.StreamPart{call, {Type: provider.PartToolResult, ToolCallID: "a", ToolName: "f", Result: json.RawMessage("{}"), Preliminary: &yes}}},
		{"null result", []provider.StreamPart{call, {Type: provider.PartToolResult, ToolCallID: "a", ToolName: "f", Result: json.RawMessage("null")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
			assert.Contains(t, body, `"message":"internal error"`)
		})
	}
}

func TestStreamingTools_FrameBoundsAndNonterminalErrors(t *testing.T) {
	for _, event := range []streamEvent{
		{typeName: provider.PartToolInputStart, id: "a", toolName: "f"},
		{typeName: provider.PartToolInputDelta, id: "a", delta: ""},
		{typeName: provider.PartToolCall, id: "a", toolName: "f", input: strings.Repeat("\x00", 32)},
		{typeName: provider.PartToolResult, id: "a", toolName: "f", result: json.RawMessage(`{"text":"<>&"}`)},
	} {
		frame, ok := encodeStreamFrame(event, 1<<20)
		require.True(t, ok)
		_, ok = encodeStreamFrame(event, int64(len(frame)))
		assert.True(t, ok)
		_, ok = encodeStreamFrame(event, int64(len(frame)-1))
		assert.False(t, ok)
	}
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "f"},
			provider.StreamPart{Type: provider.PartError},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "a", Delta: ""},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "f", Input: "{}"},
			finishPart(),
		)}, nil
	}
	body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
	assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
	assert.Contains(t, body, `"type":"finish"`)
	requireStreamBodyMatchesSchema(t, body)
}
