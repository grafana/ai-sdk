package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/internal/streamrecover"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const segmentRawRequest = `{"prompt":[],"includeRawChunks":true}`

func segmentRequest(includeRaw bool) *http.Request {
	if includeRaw {
		return streamRequest(segmentRawRequest)
	}
	return streamRequest(`{"prompt":[]}`)
}

func serveSegmentParts(t *testing.T, includeRaw bool, parts ...provider.StreamPart) string {
	t.Helper()
	return serveSegmentRequest(t, segmentRequest(includeRaw), parts...)
}

func serveSegmentRequest(t *testing.T, req *http.Request, parts ...provider.StreamPart) string {
	t.Helper()
	harness := newRuntimeHarness(t, testLimits())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(parts...)}, nil
	}
	response := harness.serve(req)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	requireStreamBodyMatchesSchema(t, body)
	return body
}

type segmentFrame struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
	Usage            json.RawMessage            `json:"usage"`
	ProviderMetadata map[string]json.RawMessage `json:"providerMetadata"`
}

func segmentFrames(t *testing.T, body string) []segmentFrame {
	t.Helper()
	var frames []segmentFrame
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		var event segmentFrame
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
		frames = append(frames, event)
	}
	return frames
}

func segmentFrameTypes(t *testing.T, body string) []string {
	t.Helper()
	var types []string
	for _, frame := range segmentFrames(t, body) {
		types = append(types, frame.Type)
	}
	return types
}

func segmentErrorCodes(t *testing.T, body string) []string {
	t.Helper()
	var codes []string
	for _, frame := range segmentFrames(t, body) {
		if frame.Type == "error" {
			require.NotNil(t, frame.Error)
			codes = append(codes, frame.Error.Code)
		}
	}
	return codes
}

func intForSegments(value int) *int { return &value }

func segmentMeta(id string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: id, ModelID: "segment-model", Timestamp: time.Date(2026, time.October, 27, 12, 0, 0, 0, time.UTC)}
}

func segmentText(id, delta string) []provider.StreamPart {
	return []provider.StreamPart{
		{Type: provider.PartTextStart, ID: id},
		{Type: provider.PartTextDelta, ID: id, Delta: delta},
		{Type: provider.PartTextEnd, ID: id},
	}
}

func segmentReasoning(id string) []provider.StreamPart {
	return []provider.StreamPart{
		{Type: provider.PartReasoningStart, ID: id},
		{Type: provider.PartReasoningDelta, ID: id, Delta: "thinking"},
		{Type: provider.PartReasoningEnd, ID: id},
	}
}

func segmentFinish(reason provider.UnifiedFinishReason, input, output int, raw string, safeguard string) provider.StreamPart {
	part := provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: reason, Raw: string(reason)},
		Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intForSegments(input)},
			OutputTokens: provider.OutputTokenUsage{Total: intForSegments(output)},
			Raw:          json.RawMessage(raw),
		},
	}
	if safeguard != "" {
		part.ProviderMetadata = provider.ProviderMetadata{"anthropic": json.RawMessage(`{"safeguardResults":` + safeguard + `}`)}
	}
	return part
}

func segmentCustom(kind string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartCustom, Kind: kind}
}

func segmentRaw(value string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(value)}
}

func joinSegmentParts(groups ...[]provider.StreamPart) []provider.StreamPart {
	var parts []provider.StreamPart
	for _, group := range groups {
		parts = append(parts, group...)
	}
	return parts
}

func anthropicShapedSegments() []provider.StreamPart {
	return joinSegmentParts(
		[]provider.StreamPart{segmentMeta("msg_1"), segmentCustom("anthropic.message_start")},
		segmentText("0", "first"),
		[]provider.StreamPart{
			{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "lookup", Input: "{}"},
			segmentFinish(provider.FinishReasonToolCalls, 10, 20, `{"input_tokens":10,"output_tokens":20}`, `[{"segment":1}]`),
			segmentRaw(`{"type":"message_start"}`),
			segmentMeta("msg_2"),
			segmentCustom("anthropic.message_start"),
		},
		segmentText("0", "second"),
		[]provider.StreamPart{segmentFinish(provider.FinishReasonStop, 30, 40, `{"input_tokens":30,"output_tokens":40}`, `[{"segment":2}]`)},
	)
}

func TestStreamingSegments_Lifecycle(t *testing.T) {
	t.Run("anthropic shaped", func(t *testing.T) {
		body := serveSegmentParts(t, true, anthropicShapedSegments()...)
		assert.Equal(t, []string{
			"stream-start",
			"response-metadata", "custom", "text-start", "text-delta", "text-end", "tool-call", "finish",
			"raw",
			"response-metadata", "custom", "text-start", "text-delta", "text-end", "finish",
		}, segmentFrameTypes(t, body))
		assert.Empty(t, segmentErrorCodes(t, body))
		var finishes []segmentFrame
		for _, frame := range segmentFrames(t, body) {
			if frame.Type == "finish" {
				finishes = append(finishes, frame)
			}
		}
		require.Len(t, finishes, 2)
		assert.JSONEq(t, `{"inputTokens":{"total":10},"outputTokens":{"total":20},"raw":{"input_tokens":10,"output_tokens":20}}`, string(finishes[0].Usage))
		assert.JSONEq(t, `{"inputTokens":{"total":30},"outputTokens":{"total":40},"raw":{"input_tokens":30,"output_tokens":40}}`, string(finishes[1].Usage))
		assert.JSONEq(t, `{"safeguardResults":[{"segment":1}]}`, string(finishes[0].ProviderMetadata["anthropic"]))
		assert.JSONEq(t, `{"safeguardResults":[{"segment":2}]}`, string(finishes[1].ProviderMetadata["anthropic"]))
		assert.Contains(t, body, `"id":"msg_1"`)
		assert.Contains(t, body, `"id":"msg_2"`)
		assert.Less(t, strings.Index(body, `"delta":"first"`), strings.Index(body, `"delta":"second"`))
	})

	t.Run("anthropic shaped without raw chunks", func(t *testing.T) {
		body := serveSegmentParts(t, false, anthropicShapedSegments()...)
		assert.Equal(t, []string{
			"stream-start",
			"response-metadata", "custom", "text-start", "text-delta", "text-end", "tool-call", "finish",
			"response-metadata", "custom", "text-start", "text-delta", "text-end", "finish",
		}, segmentFrameTypes(t, body))
		assert.Empty(t, segmentErrorCodes(t, body))
	})

	t.Run("openai shaped", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			[]provider.StreamPart{segmentMeta("resp_1")},
			segmentReasoning("rs_1"),
			[]provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "call_1", ToolName: "shell"},
				{Type: provider.PartToolInputDelta, ID: "call_1", Delta: "{}"},
				{Type: provider.PartToolInputEnd, ID: "call_1"},
				{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "shell", Input: "{}"},
				segmentMeta("resp_2"),
				{Type: provider.PartToolCall, ToolCallID: "call_2", ToolName: "shell", Input: "{}"},
				segmentMeta("resp_3"),
			},
			segmentText("msg_3", "done"),
			[]provider.StreamPart{segmentFinish(provider.FinishReasonStop, 5, 6, "", "")},
		)...)
		assert.Equal(t, []string{
			"stream-start",
			"response-metadata", "reasoning-start", "reasoning-delta", "reasoning-end", "tool-input-start", "tool-input-delta", "tool-input-end", "tool-call",
			"response-metadata", "tool-call",
			"response-metadata", "text-start", "text-delta", "text-end", "finish",
		}, segmentFrameTypes(t, body))
		assert.Empty(t, segmentErrorCodes(t, body))
	})

	t.Run("empty segment", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			[]provider.StreamPart{segmentMeta("empty"), segmentMeta("next")},
			segmentText("a", "x"),
			[]provider.StreamPart{finishPart()},
		)...)
		assert.Equal(t, []string{"stream-start", "response-metadata", "response-metadata", "text-start", "text-delta", "text-end", "finish"}, segmentFrameTypes(t, body))
		assert.Empty(t, segmentErrorCodes(t, body))
	})
}

func TestStreamingSegments_Placement(t *testing.T) {
	late := segmentMeta("late-private")
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{name: "open text block", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "a"}, late}},
		{name: "open reasoning block", parts: []provider.StreamPart{{Type: provider.PartReasoningStart, ID: "r"}, late}},
		{name: "open tool input", parts: []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"}, late}},
		{name: "preview series without final result", parts: []provider.StreamPart{
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}", ProviderExecuted: true},
			{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "lookup", Result: json.RawMessage(`{}`), Preliminary: boolForStreamTest(true)},
			late,
		}},
		{name: "overlapping message start", parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			{Type: provider.PartTextStart, ID: "0"},
			{Type: provider.PartTextDelta, ID: "0", Delta: "partial"},
			late,
			{Type: provider.PartTextEnd, ID: "0"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := serveSegmentParts(t, false, append(tc.parts, finishPart())...)
			assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
			assert.NotContains(t, body, "late-private")
			assert.NotContains(t, body, `"type":"finish"`)
		})
	}
}

func TestStreamingSegments_Identifiers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request string
		parts   []provider.StreamPart
		failure bool
	}{
		{name: "text id reused within a segment", failure: true, parts: joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1")},
			segmentText("0", "a"),
			[]provider.StreamPart{{Type: provider.PartTextStart, ID: "0"}},
		)},
		{name: "reasoning id reused within a segment", failure: true, parts: joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1")},
			segmentReasoning("r"),
			[]provider.StreamPart{{Type: provider.PartReasoningStart, ID: "r"}},
		)},
		{name: "text and reasoning ids reused in a later segment", parts: joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1")},
			segmentReasoning("r"),
			segmentText("0", "a"),
			[]provider.StreamPart{finishPart(), segmentMeta("msg_2")},
			segmentReasoning("r"),
			segmentText("0", "b"),
			[]provider.StreamPart{finishPart()},
		)},
		{name: "tool input id reopened in a later segment", failure: true, parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"},
			{Type: provider.PartToolInputEnd, ID: "call"},
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}"},
			finishPart(),
			segmentMeta("msg_2"),
			{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"},
		}},
		{name: "tool call id reopened in a later segment", failure: true, parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}"},
			finishPart(),
			segmentMeta("msg_2"),
			{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: "{}"},
		}},
		{name: "approval id reused in a later segment", failure: true, parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "mcp", Input: "{}", ProviderExecuted: true},
			{Type: provider.PartToolApprovalRequest, ApprovalID: "approval", ToolCallID: "call_1"},
			finishPart(),
			segmentMeta("msg_2"),
			{Type: provider.PartToolCall, ToolCallID: "call_2", ToolName: "mcp", Input: "{}", ProviderExecuted: true},
			{Type: provider.PartToolApprovalRequest, ApprovalID: "approval", ToolCallID: "call_2"},
		}},
		{name: "provider executed result crosses segments", parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			{Type: provider.PartToolCall, ToolCallID: "srvtoolu", ToolName: "code_execution", Input: "{}", ProviderExecuted: true},
			finishPart(),
			segmentMeta("msg_2"),
			{Type: provider.PartToolResult, ToolCallID: "srvtoolu", ToolName: "code_execution", Result: json.RawMessage(`{"ok":true}`)},
			finishPart(),
		}},
		{name: "history seeded deferred result in a later segment", request: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"history","toolName":"search","input":{},"providerExecuted":true}]}]}`, parts: []provider.StreamPart{
			segmentMeta("msg_1"),
			finishPart(),
			segmentMeta("msg_2"),
			{Type: provider.PartToolResult, ToolCallID: "history", ToolName: "search", Result: json.RawMessage(`"done"`)},
			finishPart(),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := tc.request
			if request == "" {
				request = `{"prompt":[]}`
			}
			body := serveSegmentRequest(t, streamRequest(request), tc.parts...)
			if tc.failure {
				assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
				return
			}
			assert.Empty(t, segmentErrorCodes(t, body))
			assert.Equal(t, 2, strings.Count(body, `"type":"finish"`))
		})
	}

	t.Run("reused ids are written unchanged", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1")},
			segmentText("0", "a"),
			[]provider.StreamPart{finishPart(), segmentMeta("msg_2")},
			segmentText("0", "b"),
			[]provider.StreamPart{finishPart()},
		)...)
		assert.Equal(t, 2, strings.Count(body, `{"type":"text-start","id":"0"}`))
		assert.Equal(t, 2, strings.Count(body, `{"type":"text-end","id":"0"}`))
	})
}

func TestStreamingSegments_AfterFinish(t *testing.T) {
	source := provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "post-finish-private", URL: "https://public.example"}}
	executed := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "mcp", Input: "{}", ProviderExecuted: true}
	for _, tc := range []struct {
		name   string
		prefix []provider.StreamPart
		part   provider.StreamPart
	}{
		{name: "text", part: provider.StreamPart{Type: provider.PartTextStart, ID: "post-finish-private"}},
		{name: "reasoning", part: provider.StreamPart{Type: provider.PartReasoningStart, ID: "post-finish-private"}},
		{name: "tool", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "post-finish-private", ToolName: "lookup", Input: "{}"}},
		{name: "source", part: source},
		{name: "custom", part: segmentCustom("example.post-finish-private")},
		{name: "approval", prefix: []provider.StreamPart{executed}, part: provider.StreamPart{Type: provider.PartToolApprovalRequest, ApprovalID: "post-finish-private", ToolCallID: "call"}},
		{name: "second finish", part: segmentFinish(provider.FinishReasonStop, 1, 1, "", `"post-finish-private"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := append(append(append([]provider.StreamPart{segmentMeta("msg_1")}, tc.prefix...), finishPart()), tc.part)
			body := serveSegmentParts(t, false, parts...)
			types := segmentFrameTypes(t, body)
			require.GreaterOrEqual(t, len(types), 2)
			assert.Equal(t, []string{"finish", "error"}, types[len(types)-2:])
			assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
			assert.NotContains(t, body, "post-finish-private")
		})
	}

	providerError := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "late native failure"})}

	t.Run("provider error then close", func(t *testing.T) {
		body := serveSegmentParts(t, false, segmentMeta("msg_1"), finishPart(), providerError)
		assert.Equal(t, []string{"stream-start", "response-metadata", "finish", "error"}, segmentFrameTypes(t, body))
		assert.Equal(t, []string{"overloaded"}, segmentErrorCodes(t, body))
	})

	t.Run("provider error then a segment", func(t *testing.T) {
		body := serveSegmentParts(t, false, joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1"), finishPart(), providerError, segmentMeta("msg_2")},
			segmentText("0", "after"),
			[]provider.StreamPart{finishPart()},
		)...)
		assert.Equal(t, []string{"overloaded"}, segmentErrorCodes(t, body))
		assert.Equal(t, 2, strings.Count(body, `"type":"finish"`))
		assert.Contains(t, body, `"delta":"after"`)
	})

	for _, includeRaw := range []bool{true, false} {
		t.Run("raw then a segment includeRaw="+map[bool]string{true: "true", false: "false"}[includeRaw], func(t *testing.T) {
			body := serveSegmentParts(t, includeRaw, joinSegmentParts(
				[]provider.StreamPart{segmentMeta("msg_1"), finishPart(), segmentRaw(`{"type":"message_start"}`), segmentMeta("msg_2")},
				segmentText("0", "after"),
				[]provider.StreamPart{finishPart()},
			)...)
			assert.Empty(t, segmentErrorCodes(t, body))
			assert.Equal(t, includeRaw, strings.Contains(body, `{"type":"raw","rawValue":{"type":"message_start"}}`))
			assert.Equal(t, 2, strings.Count(body, `"type":"finish"`))
		})
	}

	t.Run("recovered panic after finish", func(t *testing.T) {
		body := serveSegmentParts(t, false, segmentMeta("msg_1"), finishPart(), provider.StreamPart{Type: provider.PartError, APICallError: streamrecover.Error()})
		assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
		types := segmentFrameTypes(t, body)
		assert.Equal(t, []string{"finish", "error"}, types[len(types)-2:])
	})
}

func TestStreamingSegments_Completion(t *testing.T) {
	t.Run("close after finish is clean", func(t *testing.T) {
		body := serveSegmentParts(t, false, append(append([]provider.StreamPart{segmentMeta("msg_1")}, segmentText("0", "x")...), finishPart())...)
		assert.Empty(t, segmentErrorCodes(t, body))
		types := segmentFrameTypes(t, body)
		assert.Equal(t, "finish", types[len(types)-1])
	})

	t.Run("close before any finish", func(t *testing.T) {
		body := serveSegmentParts(t, false, segmentMeta("msg_1"), provider.StreamPart{Type: provider.PartTextStart, ID: "0"})
		assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
		assert.NotContains(t, body, `"type":"text-end"`)
		assert.NotContains(t, body, `"type":"finish"`)
	})

	t.Run("close after finish followed by metadata", func(t *testing.T) {
		body := serveSegmentParts(t, false, segmentMeta("msg_1"), finishPart(), segmentMeta("msg_2"))
		assert.Equal(t, []string{"stream-start", "response-metadata", "finish", "response-metadata", "error"}, segmentFrameTypes(t, body))
		assert.Equal(t, []string{"internal_error"}, segmentErrorCodes(t, body))
	})
}

type stalledSegmentProvider struct {
	parts     []provider.StreamPart
	context   atomic.Pointer[context.Context]
	cancelled chan struct{}
}

func newStalledSegmentProvider(parts ...provider.StreamPart) *stalledSegmentProvider {
	return &stalledSegmentProvider{parts: parts, cancelled: make(chan struct{})}
}

func (p *stalledSegmentProvider) stream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	p.context.Store(&ctx)
	stream := make(chan provider.StreamPart, len(p.parts))
	for _, part := range p.parts {
		stream <- part
	}
	go func() {
		<-ctx.Done()
		close(p.cancelled)
	}()
	return &provider.StreamResult{Stream: stream}, nil
}

func (p *stalledSegmentProvider) canceled() bool {
	ctx := p.context.Load()
	return ctx != nil && (*ctx).Err() != nil
}

func TestStreamingSegments_StalledAfterFinish(t *testing.T) {
	anthropicFirst := joinSegmentParts(
		[]provider.StreamPart{segmentMeta("msg_1")},
		segmentText("0", "first"),
		[]provider.StreamPart{segmentFinish(provider.FinishReasonToolCalls, 1, 2, "", "")},
	)
	for _, tc := range []struct {
		name     string
		parts    []provider.StreamPart
		idle     time.Duration
		deadline time.Duration
		cancel   bool
		want     []string
		budget   time.Duration
	}{
		{name: "idle after finish", parts: []provider.StreamPart{segmentMeta("msg_1"), finishPart()}, idle: 50 * time.Millisecond, want: []string{"timeout"}, budget: 550 * time.Millisecond},
		{name: "deadline after finish", parts: []provider.StreamPart{segmentMeta("msg_1"), finishPart()}, idle: time.Second, deadline: 50 * time.Millisecond, want: []string{"timeout"}, budget: 550 * time.Millisecond},
		{name: "stall between segments", parts: anthropicFirst, idle: 50 * time.Millisecond, want: []string{"timeout"}, budget: 550 * time.Millisecond},
		{name: "idle after a segment opens", parts: []provider.StreamPart{segmentMeta("msg_1"), finishPart(), segmentMeta("msg_2")}, idle: 50 * time.Millisecond, want: []string{"timeout"}, budget: 550 * time.Millisecond},
		{name: "cancellation after finish", parts: []provider.StreamPart{segmentMeta("msg_1"), finishPart()}, idle: time.Second, cancel: true, want: []string{string(codeCancellation)}, budget: 550 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.StreamIdleDuration = tc.idle
			harness := newRuntimeHarness(t, limits)
			stalled := newStalledSegmentProvider(tc.parts...)
			harness.model.stream = stalled.stream
			req := streamRequest(`{"prompt":[]}`)
			if tc.deadline > 0 {
				req = withRequestDeadline(t, req, tc.deadline)
			}
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			writer := &responseWriterProbe{}
			var canceledAtError atomic.Bool
			writer.onWrite = func() {
				if writer.writes == len(tc.parts)+2 {
					canceledAtError.Store(stalled.canceled())
				}
				if tc.cancel && writer.writes == len(tc.parts)+1 {
					time.AfterFunc(25*time.Millisecond, cancel)
				}
			}
			start := time.Now()
			harness.handler.ServeHTTP(writer, req)
			assert.Less(t, time.Since(start), tc.budget)
			select {
			case <-stalled.cancelled:
			case <-time.After(time.Second):
				require.FailNow(t, "provider was not canceled")
			}
			body := writer.body.String()
			requireStreamBodyMatchesSchema(t, body)
			assert.Equal(t, tc.want, segmentErrorCodes(t, body))
			types := segmentFrameTypes(t, body)
			require.Len(t, types, len(tc.parts)+2)
			assert.Equal(t, "error", types[len(types)-1])
			assert.Contains(t, types, "finish")
			assert.True(t, canceledAtError.Load(), "provider context must be canceled before the terminal error is written")
		})
	}
}

func TestStreamingSegments_ExecutionOverview(t *testing.T) {
	second := finishPart()
	second.ProviderMetadata = provider.ProviderMetadata{"gateway": json.RawMessage(`{"native":true}`)}
	first := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(joinSegmentParts(
			[]provider.StreamPart{segmentMeta("msg_1")},
			segmentText("0", "a"),
			[]provider.StreamPart{finishPart(), segmentMeta("msg_2")},
			segmentText("0", "b"),
			[]provider.StreamPart{second},
		)...)}, nil
	}}
	response := invocationHarness(t, testLimits(), first).serve(streamRequest(`{"prompt":[]}`))
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	requireStreamBodyMatchesSchema(t, body)
	var overviews []*execution.Overview
	var natives []json.RawMessage
	for _, frame := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		payload := []byte(strings.TrimPrefix(frame, "data: "))
		if !strings.Contains(string(payload), `"type":"finish"`) {
			continue
		}
		overviews = append(overviews, readOverview(t, payload))
		var value struct {
			Metadata map[string]struct {
				Native json.RawMessage `json:"nativeMetadata"`
			} `json:"providerMetadata"`
		}
		require.NoError(t, json.Unmarshal(payload, &value))
		natives = append(natives, value.Metadata["gateway"].Native)
	}
	require.Len(t, overviews, 2)
	require.NotNil(t, overviews[0])
	assert.Equal(t, overviews[0], overviews[1])
	assert.Empty(t, natives[0])
	assert.JSONEq(t, `{"native":true}`, string(natives[1]))
}

func TestStreamingSegments_Resources(t *testing.T) {
	t.Run("continuously ready provider ends at the request deadline", func(t *testing.T) {
		limits := testLimits()
		limits.StreamDrainDuration = 50 * time.Millisecond
		harness := newRuntimeHarness(t, limits)
		stop := make(chan struct{})
		producerDone := make(chan struct{})
		var produced atomic.Int64
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart)
			go func() {
				defer close(producerDone)
				segment := joinSegmentParts([]provider.StreamPart{segmentMeta("msg")}, segmentText("0", "x"), []provider.StreamPart{finishPart()})
				for {
					for _, part := range segment {
						select {
						case stream <- part:
							produced.Add(1)
						case <-stop:
							return
						}
					}
				}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		}
		t.Cleanup(func() { close(stop); <-producerDone })
		start := time.Now()
		response := harness.serve(withRequestDeadline(t, streamRequest(`{"prompt":[]}`), 100*time.Millisecond))
		assert.Less(t, time.Since(start), 600*time.Millisecond)
		body := response.Body.String()
		assert.Greater(t, strings.Count(body, `"type":"finish"`), 1)
		assert.Equal(t, []string{"timeout"}, segmentErrorCodes(t, body))
		time.Sleep(limits.StreamDrainDuration + 100*time.Millisecond)
		settled := produced.Load()
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, settled, produced.Load(), "drain must stop consuming after its duration")
	})

	t.Run("non-cooperative provider after finish", func(t *testing.T) {
		limits := testLimits()
		limits.StreamIdleDuration = 50 * time.Millisecond
		limits.StreamDrainDuration = 50 * time.Millisecond
		harness := newRuntimeHarness(t, limits)
		stop := make(chan struct{})
		producerDone := make(chan struct{})
		var late atomic.Int64
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 2)
			stream <- segmentMeta("msg_1")
			stream <- finishPart()
			go func() {
				defer close(producerDone)
				select {
				case <-stop:
					return
				case <-time.After(limits.StreamIdleDuration + limits.StreamDrainDuration + 150*time.Millisecond):
				}
				select {
				case stream <- segmentRaw(`{}`):
				case <-stop:
					return
				}
				select {
				case stream <- segmentRaw(`{}`):
				case <-stop:
					return
				}
				for {
					select {
					case stream <- segmentRaw(`{}`):
						late.Add(1)
					case <-stop:
						return
					}
				}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		}
		t.Cleanup(func() { close(stop); <-producerDone })
		start := time.Now()
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Less(t, time.Since(start), 550*time.Millisecond)
		assert.Equal(t, []string{"timeout"}, segmentErrorCodes(t, body))
		time.Sleep(limits.StreamIdleDuration + limits.StreamDrainDuration + 300*time.Millisecond)
		assert.Zero(t, late.Load(), "drain must exit within its duration")
	})

	t.Run("cancellation races the second segment", func(t *testing.T) {
		for range 20 {
			harness := newRuntimeHarness(t, testLimits())
			ctx, cancel := context.WithCancel(context.Background())
			stopped := make(chan struct{})
			harness.model.stream = func(modelContext context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				stream := make(chan provider.StreamPart, 3)
				stream <- segmentMeta("msg_1")
				stream <- finishPart()
				go func() {
					defer close(stopped)
					defer close(stream)
					select {
					case stream <- segmentMeta("msg_2"):
					case <-modelContext.Done():
						return
					}
					<-modelContext.Done()
				}()
				return &provider.StreamResult{Stream: stream}, nil
			}
			writer := &responseWriterProbe{}
			writer.onWrite = func() {
				if writer.writes == 3 {
					go cancel()
				}
			}
			harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`).WithContext(ctx))
			cancel()
			select {
			case <-stopped:
			case <-time.After(time.Second):
				require.FailNow(t, "provider was not canceled")
			}
			body := writer.body.String()
			requireStreamBodyMatchesSchema(t, body)
			codes := segmentErrorCodes(t, body)
			require.LessOrEqual(t, len(codes), 1)
			for _, code := range codes {
				assert.Contains(t, []string{string(codeCancellation), "internal_error"}, code)
			}
			assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
		}
	})

	t.Run("writer failure on the second metadata frame", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		stopped := make(chan struct{})
		harness.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 4)
			stream <- segmentMeta("msg_1")
			stream <- finishPart()
			stream <- segmentMeta("msg_2")
			stream <- finishPart()
			go func() { defer close(stopped); <-ctx.Done() }()
			return &provider.StreamResult{Stream: stream}, nil
		}
		writer := &responseWriterProbe{}
		writer.onWrite = func() {
			if writer.writes == 4 {
				writer.writeErr = errors.New("test writer failure")
			}
		}
		harness.handler.ServeHTTP(writer, streamRequest(`{"prompt":[]}`))
		assert.Equal(t, 4, writer.writes)
		assert.NotContains(t, writer.body.String(), "msg_2")
		select {
		case <-stopped:
		case <-time.After(time.Second):
			require.FailNow(t, "provider was not canceled")
		}
	})
}
