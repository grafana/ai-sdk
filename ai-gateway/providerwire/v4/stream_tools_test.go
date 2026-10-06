package v4

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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

func TestStreamingProviderTools_MarkersPreviewsAndMetadata(t *testing.T) {
	metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"item-1","private":"secret"}`)}
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		no, yes := false, true
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "search", ProviderExecuted: true, Dynamic: &no},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "a", Delta: ""},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "search", Input: `{}`, ProviderExecuted: true, Dynamic: &yes, ProviderMetadata: metadata},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "search", Result: json.RawMessage(`{"preview":1}`), Preliminary: new(true), Dynamic: &yes, ProviderExecuted: true, ProviderMetadata: metadata},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "search", Result: json.RawMessage(`{"preview":2}`), Preliminary: new(true)},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "search", Result: json.RawMessage(`{"final":true}`)},
			finishPart(),
		)}, nil
	}
	response := harness.serve(streamRequest(`{"prompt":[]}`))
	body := response.Body.String()
	assert.Contains(t, body, `"type":"tool-input-start","id":"a","toolName":"search","providerExecuted":true,"dynamic":false`)
	assert.Contains(t, body, `"type":"tool-call","toolCallId":"a","toolName":"search","input":"{}","providerExecuted":true,"dynamic":true`)
	assert.Equal(t, 2, strings.Count(body, `"preliminary":true`))
	assert.Contains(t, body, `"itemId":"item-1"`)
	assert.Contains(t, body, `"private":"secret"`)
	for _, frame := range strings.Split(body, "\n\n") {
		if !strings.Contains(frame, `"type":"tool-result"`) {
			continue
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &value); err != nil {
			t.Fatal(err)
		}
		assert.NotContains(t, value, "providerExecuted")
	}
	assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
	assert.Equal(t, 0, strings.Count(body, `"type":"error"`))
	requireStreamBodyMatchesSchema(t, body)
}

func TestStreamingProviderTools_InputStartDynamicPresence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dynamic *bool
	}{
		{name: "absent"},
		{name: "false", dynamic: boolForStreamTest(false)},
		{name: "true", dynamic: boolForStreamTest(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(
					provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "echo", Dynamic: tc.dynamic},
					provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call"},
					finishPart(),
				)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			frames := strings.Split(body, "\n\n")
			var event map[string]json.RawMessage
			if len(frames) < 2 || !strings.HasPrefix(frames[1], "data: ") {
				t.Fatal("missing input-start frame")
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			marker, exists := event["dynamic"]
			assert.Equal(t, tc.dynamic != nil, exists)
			if tc.dynamic != nil {
				assert.Equal(t, string(marker), map[bool]string{true: "true", false: "false"}[*tc.dynamic])
			}
			requireStreamBodyMatchesSchema(t, body)
		})
	}
}

func boolForStreamTest(value bool) *bool { return &value }

func TestStreamingProviderTools_DeferredResultOnly(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`"failed"`), IsError: true, ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"},"private":"discard"}`)}},
			finishPart(),
		)}, nil
	}
	body := `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":{},"providerExecuted":true,"providerOptions":{"anthropic":{"caller":{"type":"direct"}}}}]}]}`
	response := harness.serve(streamRequest(body))
	output := response.Body.String()
	assert.Equal(t, 1, strings.Count(output, `"type":"tool-result"`))
	assert.NotContains(t, output, `"type":"tool-call"`)
	assert.Contains(t, output, `"isError":true`)
	assert.Contains(t, output, `"caller":{"type":"direct"}`)
	assert.NotContains(t, output, "private-token")
	assert.Contains(t, output, `"private":"discard"`)
	assert.Equal(t, 1, strings.Count(output, `"type":"finish"`))
	requireStreamBodyMatchesSchema(t, output)
}

func TestStreamingProviderTools_HistoryDoesNotConsumeProviderPartBudget(t *testing.T) {
	limits := testLimits()
	limits.StreamParts = 3
	harness := newRuntimeHarness(t, limits)
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "history-7", ToolName: "echo", Result: json.RawMessage(`"done"`)},
			finishPart(),
		)}, nil
	}
	var calls []string
	for i := range 8 {
		calls = append(calls, `{"type":"tool-call","toolCallId":"history-`+strconv.Itoa(i)+`","toolName":"echo","input":{},"providerExecuted":true}`)
	}
	body := harness.serve(streamRequest(`{"prompt":[{"role":"assistant","content":[` + strings.Join(calls, ",") + `]}]}`)).Body.String()
	assert.Equal(t, 1, strings.Count(body, `"type":"tool-result"`))
	assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
	assert.NotContains(t, body, `"type":"error"`)
	requireStreamBodyMatchesSchema(t, body)
}

func TestStreamingProviderTools_PreviewsAndErrorsPreserveLifecycle(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`, ProviderExecuted: true},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"preview":true}`), Preliminary: new(true)},
			provider.StreamPart{Type: provider.PartError},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"final":true}`)},
			finishPart(),
		)}, nil
	}
	body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
	assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
	assert.Equal(t, 2, strings.Count(body, `"type":"tool-result"`))
	assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
	assert.Less(t, strings.Index(body, `"preliminary":true`), strings.Index(body, `"type":"error"`))
	assert.Less(t, strings.Index(body, `"type":"error"`), strings.Index(body, `"final":true`))
	requireStreamBodyMatchesSchema(t, body)
}

func TestStreamingProviderTools_BoundedPreviewAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits Limits
		parts  []provider.StreamPart
	}{
		{name: "preview exceeds provider part budget", limits: func() Limits { limits := testLimits(); limits.StreamParts = 3; return limits }(), parts: []provider.StreamPart{
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`0`), Preliminary: new(true)},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`1`), Preliminary: new(true)},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`2`)},
			finishPart(),
		}},
		{name: "metadata exceeds frame budget", limits: testLimits(), parts: []provider.StreamPart{
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`, ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"` + strings.Repeat("secret", int(testLimits().StreamFrameBytes)) + `"}`)}},
			finishPart(),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, tc.limits)
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
			assert.NotContains(t, body, `"type":"finish"`)
			assert.NotContains(t, body, "secret")
			requireStreamBodyMatchesSchema(t, body)
		})
	}
}

func TestStreamingProviderTools_CancelDuringPreliminaryResult(t *testing.T) {
	limits := testLimits()
	limits.ModelDuration = time.Second
	limits.StreamIdleDuration = time.Second
	harness := newRuntimeHarness(t, limits)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	producerDone := make(chan struct{})
	harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart)
		go func() {
			defer close(producerDone)
			defer close(parts)
			for _, part := range []provider.StreamPart{
				{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`},
				{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: new(true)},
			} {
				select {
				case parts <- part:
				case <-ctx.Done():
					return
				}
			}
			<-ctx.Done()
		}()
		return &provider.StreamResult{Stream: parts}, nil
	}
	writer := &responseWriterProbe{}
	writer.onFlush = func(int) {
		if strings.Contains(writer.body.String(), `"preliminary":true`) {
			cancel()
		}
	}
	harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(requestContext))
	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("provider producer retained after cancellation")
	}
	assert.Contains(t, writer.body.String(), `"preliminary":true`)
	assert.NotContains(t, writer.body.String(), `"type":"finish"`)
	assert.Equal(t, 1, strings.Count(writer.body.String(), `"type":"error"`))
}

func TestStreamingProviderTools_WriterFailureAfterPreliminaryResult(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: new(true)},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`true`)},
			finishPart(),
		)}, nil
	}
	writer := &responseWriterProbe{}
	writer.onWrite = func() {
		if writer.writes == 4 {
			writer.writeErr = errors.New("private writer failure")
		}
	}
	harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
	assert.Equal(t, 4, writer.writes)
	assert.Contains(t, writer.body.String(), `"preliminary":true`)
	assert.NotContains(t, writer.body.String(), `"type":"finish"`)
	assert.NotContains(t, writer.body.String(), `"type":"error"`)
	assert.NotContains(t, writer.body.String(), "private writer failure")
}

func TestStreamingProviderTools_DeferredSourceFailsSafely(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "search", Input: `{}`, ProviderExecuted: true},
			provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, URL: "https://private-source.example"}},
			finishPart(),
		)}, nil
	}
	body := harness.serve(streamRequest(`{"prompt":[],"tools":[{"type":"provider","id":"anthropic.web_search_20250305","name":"search","args":{}}]}`)).Body.String()
	assert.Contains(t, body, `"type":"tool-call"`)
	assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
	assert.NotContains(t, body, `"type":"finish"`)
	assert.NotContains(t, body, "private-source")
	requireStreamBodyMatchesSchema(t, body)
}

func TestStreamingProviderTools_InvalidFinalization(t *testing.T) {
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`}
	preview := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: new(true)}
	final := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`)}
	for _, tc := range []struct {
		name    string
		parts   []provider.StreamPart
		request string
	}{
		{name: "preview without final", parts: []provider.StreamPart{call, preview, finishPart()}, request: `{"prompt":[]}`},
		{name: "result after final", parts: []provider.StreamPart{call, final, final, finishPart()}, request: `{"prompt":[]}`},
		{name: "unmatched result", parts: []provider.StreamPart{final, finishPart()}, request: `{"prompt":[]}`},
		{name: "mismatched historical name", parts: []provider.StreamPart{final, finishPart()}, request: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"other","input":{},"providerExecuted":true}]}]}`},
		{name: "empty historical name", parts: []provider.StreamPart{{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "", Result: json.RawMessage(`false`)}, finishPart()}, request: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"","input":{},"providerExecuted":true}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}
			body := harness.serve(streamRequest(tc.request)).Body.String()
			assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
			assert.NotContains(t, body, `"type":"finish"`)
			assert.Contains(t, body, `"message":"internal error"`)
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
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"delta without start", []provider.StreamPart{{Type: provider.PartToolInputDelta, ID: "a"}}},
		{"end without start", []provider.StreamPart{end}},
		{"duplicate start", []provider.StreamPart{start, start}},
		{"duplicate end", []provider.StreamPart{start, end, end}},
		{"reopen after call", []provider.StreamPart{call, start}},
		{"delta after end", []provider.StreamPart{start, end, {Type: provider.PartToolInputDelta, ID: "a", Delta: "{}"}}},
		{"call before end", []provider.StreamPart{start, call}},
		{"duplicate call", []provider.StreamPart{call, call}},
		{"unmatched result", []provider.StreamPart{result}},
		{"duplicate result", []provider.StreamPart{call, result, result}},
		{"mismatched name", []provider.StreamPart{start, end, {Type: provider.PartToolCall, ToolCallID: "a", ToolName: "other", Input: "{}"}}},
		{"open at finish", []provider.StreamPart{start, finishPart()}},
		{"late metadata", []provider.StreamPart{call, {Type: provider.PartResponseMeta}}},
		{"metadata after input start", []provider.StreamPart{start, {Type: provider.PartResponseMeta}, end, call}},
		{"empty input id", []provider.StreamPart{{Type: provider.PartToolInputStart, ToolName: "f"}}},
		{"empty input name", []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "a"}}},
		{"empty call id", []provider.StreamPart{{Type: provider.PartToolCall, ToolName: "f", Input: "{}"}}},
		{"empty call name", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "a", Input: "{}"}}},
		{"preliminary without final", []provider.StreamPart{call, {Type: provider.PartToolResult, ToolCallID: "a", ToolName: "f", Result: json.RawMessage("{}"), Preliminary: new(true)}, finishPart()}},
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
			assert.NotContains(t, body, `"type":"finish"`)
		})
	}
}

func TestStreamingTools_IndependentTextAndToolIDs(t *testing.T) {
	start := provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "f"}
	end := provider.StreamPart{Type: provider.PartToolInputEnd, ID: "a"}
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "f", Input: "{}"}
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"no deltas", []provider.StreamPart{start, end, call}},
		{"ended input without call", []provider.StreamPart{start, end}},
		{"equal text and tool ids", []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, start, end, call, {Type: provider.PartTextEnd, ID: "a"}}},
		{"text after standalone call", []provider.StreamPart{call, {Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextEnd, ID: "a"}}},
		{"text and parallel input", []provider.StreamPart{
			start, {Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartToolInputStart, ID: "b", ToolName: "g"},
			{Type: provider.PartToolInputDelta, ID: "a", Delta: ""}, {Type: provider.PartToolInputEnd, ID: "b"},
			{Type: provider.PartToolCall, ToolCallID: "b", ToolName: "g", Input: "{}"},
			{Type: provider.PartTextDelta, ID: "text", Delta: "checking"}, end, call, {Type: provider.PartTextEnd, ID: "text"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(append(tc.parts, finishPart())...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.NotContains(t, body, `"type":"error"`)
			assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
			requireStreamBodyMatchesSchema(t, body)
			frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
			require.Len(t, frames, len(tc.parts)+2)
			for i, part := range tc.parts {
				var event struct {
					Type provider.StreamPartType `json:"type"`
				}
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[i+1], "data: ")), &event))
				assert.Equal(t, part.Type, event.Type)
			}
		})
	}
}

func TestStreamingTools_InvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff})
	start := provider.StreamPart{Type: provider.PartToolInputStart, ID: "a", ToolName: "f"}
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"input id", []provider.StreamPart{{Type: provider.PartToolInputStart, ID: invalid, ToolName: "f"}}},
		{"input name", []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "a", ToolName: invalid}}},
		{"call id", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: invalid, ToolName: "f", Input: "{}"}}},
		{"call name", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "a", ToolName: invalid, Input: "{}"}}},
		{"call input", []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "a", ToolName: "f", Input: invalid}}},
		{"delta", []provider.StreamPart{start, {Type: provider.PartToolInputDelta, ID: "a", Delta: invalid}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(append(tc.parts, finishPart())...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			want := string(canonicalEmptyStartFrame)
			if len(tc.parts) == 2 {
				frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartToolInputStart, id: "a", toolName: "f"}, testLimits().StreamFrameBytes)
				require.True(t, ok)
				want += string(frame)
			}
			assert.Equal(t, want+string(canonicalInternalStreamErrorFrame), body)
		})
	}
}

func TestStreamingTools_FrameBoundsAndNonterminalErrors(t *testing.T) {
	for _, event := range []streamEvent{
		{typeName: provider.PartToolInputStart, id: "a", toolName: "f"},
		{typeName: provider.PartToolInputDelta, id: "a", delta: ""},
		{typeName: provider.PartToolCall, id: "a", toolName: "f", input: strings.Repeat("\x00", 32)},
		{typeName: provider.PartToolResult, id: "a", toolName: "f", result: json.RawMessage(`{"text":"<>&"}`)},
		{typeName: provider.PartToolResult, id: "a", toolName: "f", result: json.RawMessage(`false`), preliminary: new(true), metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"item-1"}`)}},
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
