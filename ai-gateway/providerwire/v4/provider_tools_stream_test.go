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
)

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
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "search", Result: json.RawMessage(`{"preview":1}`), Preliminary: true, Dynamic: &yes, ProviderExecuted: true, ProviderMetadata: metadata},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "a", ToolName: "search", Result: json.RawMessage(`{"preview":2}`), Preliminary: true},
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
	assert.NotContains(t, body, `"private"`)
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
	assert.NotContains(t, output, "discard")
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
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"preview":true}`), Preliminary: true},
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
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`0`), Preliminary: true},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`1`), Preliminary: true},
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
				{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: true},
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
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: true},
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
	preview := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: true}
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
		{name: "unconfigured MCP metadata", parts: []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`, ProviderExecuted: true, ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo"}`)}}, finishPart()}, request: `{"prompt":[]}`},
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
