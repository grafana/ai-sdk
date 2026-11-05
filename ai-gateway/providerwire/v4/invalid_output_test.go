package v4

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lifecycleStreamCase struct {
	name     string
	history  execution.ToolHistory
	parts    []provider.StreamPart
	rejectAt int
}

func lifecyclePart(kind provider.StreamPartType, id string) provider.StreamPart {
	return provider.StreamPart{Type: kind, ID: id}
}

func lifecycleCall(id, name string, providerExecuted bool) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: `{}`, ProviderExecuted: providerExecuted}
}

func lifecycleResult(id, name, result string, preliminary bool) provider.StreamPart {
	part := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: name, Result: json.RawMessage(result)}
	if preliminary {
		part.Preliminary = new(true)
	}
	return part
}

func lifecycleApproval(callID, approvalID string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

func changed(part provider.StreamPart, change func(*provider.StreamPart)) provider.StreamPart {
	change(&part)
	return part
}

// lifecycleStreamCases holds one sequence per stream rule class, with valid
// content so that only lifecycle rules decide, and the index of the part the
// runtime rejects, or -1.
func lifecycleStreamCases() []lifecycleStreamCase {
	start := provider.StreamPart{Type: provider.PartStreamStart}
	metadata := provider.StreamPart{Type: provider.PartResponseMeta}
	raw := provider.StreamPart{Type: provider.PartRaw}
	failure := provider.StreamPart{Type: provider.PartError}
	source := provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "s", URL: "https://example.test"}}
	custom := provider.StreamPart{Type: provider.PartCustom, Kind: "vendor.kind"}
	file := provider.StreamPart{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}}
	pending := execution.ToolHistory{"hosted": "search"}
	inputStart := provider.StreamPart{Type: provider.PartToolInputStart, ID: "c", ToolName: "f"}
	inputEnd := provider.StreamPart{Type: provider.PartToolInputEnd, ID: "c"}
	textStart, textEnd := lifecyclePart(provider.PartTextStart, "0"), lifecyclePart(provider.PartTextEnd, "0")
	return []lifecycleStreamCase{
		{name: "valid start and finish", parts: []provider.StreamPart{start, finishPart()}, rejectAt: -1},
		{name: "repeated stream-start", parts: []provider.StreamPart{start, start}, rejectAt: 1},
		{name: "stream-start after raw", parts: []provider.StreamPart{raw, start}, rejectAt: 1},
		{name: "stream-start after error", parts: []provider.StreamPart{failure, start}, rejectAt: 1},
		{name: "stream-start after finish", parts: []provider.StreamPart{finishPart(), start}, rejectAt: 1},

		{name: "metadata before content", parts: []provider.StreamPart{raw, failure, metadata, finishPart()}, rejectAt: -1},
		{name: "consecutive metadata", parts: []provider.StreamPart{metadata, metadata, finishPart()}, rejectAt: -1},
		{name: "metadata after closed content", parts: []provider.StreamPart{source, custom, file, textStart, textEnd, lifecycleCall("c", "f", false), metadata, finishPart()}, rejectAt: -1},
		{name: "metadata after an approval", history: pending, parts: []provider.StreamPart{lifecycleApproval("hosted", "a"), metadata, lifecycleResult("hosted", "search", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "metadata with open text", parts: []provider.StreamPart{textStart, metadata}, rejectAt: 1},
		{name: "metadata with open reasoning", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "r"), metadata}, rejectAt: 1},
		{name: "metadata with open tool input", parts: []provider.StreamPart{inputStart, metadata}, rejectAt: 1},
		{name: "metadata with a pending preview", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleResult("c", "f", `{}`, true), metadata}, rejectAt: 2},
		{name: "metadata with a pending history preview", history: pending, parts: []provider.StreamPart{lifecycleResult("hosted", "search", `{}`, true), metadata}, rejectAt: 1},

		{name: "two segments", parts: []provider.StreamPart{metadata, textStart, textEnd, finishPart(), raw, metadata, textStart, textEnd, finishPart()}, rejectAt: -1},
		{name: "segment without finish", parts: []provider.StreamPart{lifecycleCall("c", "f", false), metadata, lifecycleCall("d", "f", false), finishPart()}, rejectAt: -1},
		{name: "reasoning ID reused in a later segment", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "r"), lifecyclePart(provider.PartReasoningEnd, "r"), metadata, lifecyclePart(provider.PartReasoningStart, "r"), lifecyclePart(provider.PartReasoningEnd, "r"), finishPart()}, rejectAt: -1},
		{name: "tool ID reused in a later segment", parts: []provider.StreamPart{lifecycleCall("c", "f", false), metadata, lifecycleCall("c", "f", false)}, rejectAt: 2},
		{name: "approval ID reused in a later segment", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleApproval("c", "a"), lifecycleResult("c", "f", `{}`, false), metadata, lifecycleCall("d", "f", true), lifecycleApproval("d", "a")}, rejectAt: 5},
		{name: "result in a later segment", parts: []provider.StreamPart{lifecycleCall("c", "f", true), finishPart(), metadata, lifecycleResult("c", "f", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "history result in a later segment", history: pending, parts: []provider.StreamPart{metadata, finishPart(), metadata, lifecycleResult("hosted", "search", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "metadata, raw and error after finish", parts: []provider.StreamPart{finishPart(), raw, failure, metadata, finishPart(), failure}, rejectAt: -1},
		{name: "content after finish", parts: []provider.StreamPart{textStart, textEnd, finishPart(), lifecyclePart(provider.PartTextStart, "late")}, rejectAt: 3},
		{name: "source after finish", parts: []provider.StreamPart{finishPart(), raw, source}, rejectAt: 2},
		{name: "second finish", parts: []provider.StreamPart{finishPart(), finishPart()}, rejectAt: 1},
		{name: "unknown part after finish", parts: []provider.StreamPart{finishPart(), {Type: "future"}}, rejectAt: 1},

		{name: "empty text ID", parts: []provider.StreamPart{lifecyclePart(provider.PartTextStart, "")}, rejectAt: 0},
		{name: "overlapping text", parts: []provider.StreamPart{lifecyclePart(provider.PartTextStart, "t"), lifecyclePart(provider.PartTextStart, "u")}, rejectAt: 1},
		{name: "reused text ID", parts: []provider.StreamPart{textStart, textEnd, textStart}, rejectAt: 2},
		{name: "text delta for another ID", parts: []provider.StreamPart{lifecyclePart(provider.PartTextStart, "t"), lifecyclePart(provider.PartTextDelta, "u")}, rejectAt: 1},
		{name: "text end without start", parts: []provider.StreamPart{lifecyclePart(provider.PartTextEnd, "t")}, rejectAt: 0},
		{name: "concurrent reasoning", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "a"), lifecyclePart(provider.PartReasoningStart, "b"), lifecyclePart(provider.PartReasoningEnd, "a"), lifecyclePart(provider.PartReasoningEnd, "b"), finishPart()}, rejectAt: -1},
		{name: "empty reasoning ID", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "")}, rejectAt: 0},
		{name: "reused reasoning ID", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "r"), lifecyclePart(provider.PartReasoningEnd, "r"), lifecyclePart(provider.PartReasoningStart, "r")}, rejectAt: 2},
		{name: "reasoning delta without start", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningDelta, "r")}, rejectAt: 0},

		{name: "streamed tool call", parts: []provider.StreamPart{inputStart, inputEnd, lifecycleCall("c", "f", false), lifecycleResult("c", "f", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "tool input without name", parts: []provider.StreamPart{changed(inputStart, func(p *provider.StreamPart) { p.ToolName = "" })}, rejectAt: 0},
		{name: "repeated tool input", parts: []provider.StreamPart{inputStart, inputStart}, rejectAt: 1},
		{name: "tool input for a history call", history: pending, parts: []provider.StreamPart{changed(inputStart, func(p *provider.StreamPart) { p.ID = "hosted" })}, rejectAt: 0},
		{name: "dynamic tool delta", parts: []provider.StreamPart{inputStart, {Type: provider.PartToolInputDelta, ID: "c", Dynamic: new(true)}}, rejectAt: 1},
		{name: "tool call with open input", parts: []provider.StreamPart{inputStart, lifecycleCall("c", "f", false)}, rejectAt: 1},
		{name: "tool call with another name", parts: []provider.StreamPart{inputStart, inputEnd, lifecycleCall("c", "g", false)}, rejectAt: 2},
		{name: "repeated tool call", parts: []provider.StreamPart{lifecycleCall("c", "f", false), lifecycleCall("c", "f", false)}, rejectAt: 1},
		{name: "result for an unknown call", parts: []provider.StreamPart{lifecycleResult("c", "f", `{}`, false)}, rejectAt: 0},
		{name: "result with another name", parts: []provider.StreamPart{lifecycleCall("c", "f", false), lifecycleResult("c", "g", `{}`, false)}, rejectAt: 1},
		{name: "null result", parts: []provider.StreamPart{lifecycleCall("c", "f", false), lifecycleResult("c", "f", `null`, false)}, rejectAt: 1},
		{name: "invalid result", parts: []provider.StreamPart{lifecycleCall("c", "f", false), lifecycleResult("c", "f", `{`, false)}, rejectAt: 1},
		{name: "preview then final result", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleResult("c", "f", `{}`, true), lifecycleResult("c", "f", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "history result", history: pending, parts: []provider.StreamPart{lifecycleResult("hosted", "search", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "tool input for a resolved history ID", history: pending, parts: []provider.StreamPart{lifecycleResult("hosted", "search", `{}`, false), changed(inputStart, func(p *provider.StreamPart) { p.ID = "hosted" })}, rejectAt: 1},
		{name: "approval with an empty ID", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleApproval("c", "")}, rejectAt: 1},
		{name: "history re-emission not provider-executed", history: pending, parts: []provider.StreamPart{lifecycleCall("hosted", "search", false)}, rejectAt: 0},
		{name: "second history re-emission", history: pending, parts: []provider.StreamPart{lifecycleCall("hosted", "search", true), lifecycleCall("hosted", "search", true)}, rejectAt: 1},
		{name: "history re-emission after its result", history: pending, parts: []provider.StreamPart{lifecycleResult("hosted", "search", `{}`, false), lifecycleCall("hosted", "search", true)}, rejectAt: 1},
		{name: "history approval", history: pending, parts: []provider.StreamPart{lifecycleApproval("hosted", "a"), lifecycleResult("hosted", "search", `{}`, false), finishPart()}, rejectAt: -1},
		{name: "approval of an unknown call", parts: []provider.StreamPart{lifecycleApproval("c", "a")}, rejectAt: 0},
		{name: "approval of a client call", parts: []provider.StreamPart{lifecycleCall("c", "f", false), lifecycleApproval("c", "a")}, rejectAt: 1},
		{name: "repeated approval of one call", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleApproval("c", "a"), lifecycleApproval("c", "b")}, rejectAt: 2},
		{name: "reused approval ID", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleCall("d", "f", true), lifecycleApproval("c", "a"), lifecycleApproval("d", "a")}, rejectAt: 3},
		{name: "approval with a dynamic flag", parts: []provider.StreamPart{lifecycleCall("c", "f", true), changed(lifecycleApproval("c", "a"), func(p *provider.StreamPart) { p.Dynamic = new(false) })}, rejectAt: 1},

		{name: "finish with open tool input", parts: []provider.StreamPart{inputStart, finishPart()}, rejectAt: 1},
		{name: "finish with a pending preview", parts: []provider.StreamPart{lifecycleCall("c", "f", true), lifecycleResult("c", "f", `{}`, true), finishPart()}, rejectAt: 2},
		{name: "finish with a pending history preview", history: pending, parts: []provider.StreamPart{lifecycleResult("hosted", "search", `{}`, true), finishPart()}, rejectAt: 1},
		{name: "finish with active text", parts: []provider.StreamPart{textStart, finishPart()}, rejectAt: 1},
		{name: "finish with active reasoning", parts: []provider.StreamPart{lifecyclePart(provider.PartReasoningStart, "r"), finishPart()}, rejectAt: 1},
		{name: "finish with warnings", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.Warnings = []provider.Warning{{Type: provider.WarnUnsupported}} })}, rejectAt: 0},
		{name: "finish without a reason", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.FinishReason = nil })}, rejectAt: 0},
		{name: "finish without usage", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.Usage = nil })}, rejectAt: 0},
		{name: "unregistered finish reason", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.FinishReason = &provider.FinishReason{Unified: "future"} })}, rejectAt: 0},
		{name: "unknown part type", parts: []provider.StreamPart{{Type: "future"}}, rejectAt: 0},
	}
}

type lifecycleUnaryCase struct {
	name    string
	history execution.ToolHistory
	result  *provider.GenerateResult
	invalid bool
}

func lifecycleGenerate(reason provider.UnifiedFinishReason, content ...provider.GenerateContentPart) *provider.GenerateResult {
	return &provider.GenerateResult{Content: content, FinishReason: provider.FinishReason{Unified: reason}}
}

func unaryLifecycleCall(id, name string, providerExecuted bool) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: id, ToolName: name, Input: json.RawMessage(`{}`), ProviderExecuted: providerExecuted}
}

func unaryLifecycleResult(id, name, result string, preliminary bool) provider.GenerateContentPart {
	part := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: id, ToolName: name, Result: json.RawMessage(result)}
	if preliminary {
		part.Preliminary = new(true)
	}
	return part
}

func unaryLifecycleApproval(callID, approvalID string) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

func lifecycleUnaryCases() []lifecycleUnaryCase {
	stop := provider.FinishReasonStop
	text := provider.GenerateContentPart{Type: provider.ContentText, Text: "ok"}
	pending := execution.ToolHistory{"hosted": "search"}
	return []lifecycleUnaryCase{
		{name: "valid text", result: lifecycleGenerate(stop, text)},
		{name: "unregistered finish reason", result: lifecycleGenerate("future", text), invalid: true},
		{name: "unknown content type", result: lifecycleGenerate(stop, provider.GenerateContentPart{Type: "future"}), invalid: true},
		{name: "provider-executed text", result: lifecycleGenerate(stop, provider.GenerateContentPart{Type: provider.ContentText, Text: "ok", ProviderExecuted: true}), invalid: true},
		{name: "dynamic custom part", result: lifecycleGenerate(stop, provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "vendor.kind", Dynamic: new(true)}), invalid: true},
		{name: "call without ID", result: lifecycleGenerate(stop, unaryLifecycleCall("", "f", false)), invalid: true},
		{name: "repeated call", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", true), unaryLifecycleCall("c", "f", true)), invalid: true},
		{name: "result for an unknown call", result: lifecycleGenerate(stop, unaryLifecycleResult("c", "f", `{}`, false)), invalid: true},
		{name: "result with another name", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", false), unaryLifecycleResult("c", "g", `{}`, false)), invalid: true},
		{name: "null result", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", false), unaryLifecycleResult("c", "f", `null`, false)), invalid: true},
		{name: "preview without final", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", true), unaryLifecycleResult("c", "f", `{}`, true)), invalid: true},
		{name: "preview then final", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", true), unaryLifecycleResult("c", "f", `{}`, true), unaryLifecycleResult("c", "f", `{}`, false))},
		{name: "history result", history: pending, result: lifecycleGenerate(stop, unaryLifecycleResult("hosted", "search", `{}`, false))},
		{name: "history re-emission", history: pending, result: lifecycleGenerate(stop, unaryLifecycleCall("hosted", "search", true), unaryLifecycleResult("hosted", "search", `{}`, false))},
		{name: "second history re-emission", history: pending, result: lifecycleGenerate(stop, unaryLifecycleCall("hosted", "search", true), unaryLifecycleCall("hosted", "search", true)), invalid: true},
		{name: "history approval", history: pending, result: lifecycleGenerate(stop, unaryLifecycleApproval("hosted", "approval"))},
		{name: "approval of a client call", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", false), unaryLifecycleApproval("c", "a")), invalid: true},
		{name: "reused approval ID", result: lifecycleGenerate(stop, unaryLifecycleCall("c", "f", true), unaryLifecycleCall("d", "f", true), unaryLifecycleApproval("c", "a"), unaryLifecycleApproval("d", "a")), invalid: true},
	}
}

func newLifecycleHarness(t *testing.T) *runtimeHarness {
	t.Helper()
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.Candidates = []catalog.ConfiguredCandidate{{Provider: "native", ProviderInstance: "configured", ModelID: "backend"}}
	return harness
}

func newLifecycleStreamHarness(t *testing.T, parts []provider.StreamPart) *runtimeHarness {
	t.Helper()
	harness := newLifecycleHarness(t)
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(parts...)}, nil
	}
	return harness
}

// lifecyclePrompt encodes history as a prompt whose pending provider-executed
// calls the runtime rebuilds into that history.
func lifecyclePrompt(t *testing.T, history execution.ToolHistory) string {
	t.Helper()
	type toolCall struct {
		Type             string          `json:"type"`
		ToolCallID       string          `json:"toolCallId"`
		ToolName         string          `json:"toolName"`
		Input            json.RawMessage `json:"input"`
		ProviderExecuted bool            `json:"providerExecuted"`
	}
	type message struct {
		Role    string     `json:"role"`
		Content []toolCall `json:"content"`
	}
	prompt := []message{}
	if len(history) != 0 {
		calls := make([]toolCall, 0, len(history))
		for _, id := range slices.Sorted(maps.Keys(history)) {
			calls = append(calls, toolCall{Type: "tool-call", ToolCallID: id, ToolName: history[id], Input: json.RawMessage(`{}`), ProviderExecuted: true})
		}
		prompt = append(prompt, message{Role: "assistant", Content: calls})
	}
	body, err := json.Marshal(map[string]any{"prompt": prompt})
	require.NoError(t, err)
	return string(body)
}

func serveLifecycleStream(t *testing.T, history execution.ToolHistory, parts []provider.StreamPart) string {
	t.Helper()
	response := newLifecycleStreamHarness(t, parts).serve(streamRequest(lifecyclePrompt(t, history)))
	require.Equal(t, http.StatusOK, response.Code)
	return response.Body.String()
}

func serveLifecycleUnary(t *testing.T, history execution.ToolHistory, result *provider.GenerateResult) (int, string) {
	t.Helper()
	harness := newLifecycleHarness(t)
	harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }
	response := harness.serve(validRequest(lifecyclePrompt(t, history)))
	return response.Code, response.Body.String()
}

// acceptedFrameTypes is the frame for each part ProviderWire writes when the
// runtime accepts every part: an empty stream-start when the stream does not
// open with one, and nothing for unrequested raw parts.
func acceptedFrameTypes(parts []provider.StreamPart) []string {
	var types []string
	if len(parts) == 0 || parts[0].Type != provider.PartStreamStart {
		types = append(types, string(provider.PartStreamStart))
	}
	for _, part := range parts {
		if part.Type != provider.PartRaw {
			types = append(types, string(part.Type))
		}
	}
	return types
}

// TestInvalidOutput_StreamStopsAtRejectedPart shows that ProviderWire ends a
// stream at the part the runtime rejects, with the same frames as a content
// rejection it owns at that part: an invalid source.
func TestInvalidOutput_StreamStopsAtRejectedPart(t *testing.T) {
	invalidSource := provider.StreamPart{Type: provider.PartSource}
	for _, tc := range lifecycleStreamCases() {
		t.Run(tc.name, func(t *testing.T) {
			body := serveLifecycleStream(t, tc.history, tc.parts)
			if tc.rejectAt < 0 {
				assert.Equal(t, acceptedFrameTypes(tc.parts), segmentFrameTypes(t, body))
				return
			}
			reference := serveLifecycleStream(t, tc.history, append(slices.Clone(tc.parts[:tc.rejectAt]), invalidSource))
			frames := segmentFrames(t, reference)
			last := frames[len(frames)-1]
			require.NotNil(t, last.Error)
			require.Equal(t, string(codeInternal), last.Error.Code)
			assert.Equal(t, reference, body)
		})
	}
}

// TestInvalidOutput_UnaryDocument shows that ProviderWire writes the same 500
// document for unary output the runtime rejects as for a result it cannot
// represent.
func TestInvalidOutput_UnaryDocument(t *testing.T) {
	status, reference := serveLifecycleUnary(t, nil, lifecycleGenerate(provider.FinishReasonStop, provider.GenerateContentPart{Type: provider.ContentText, Text: string([]byte{0xff})}))
	require.Equal(t, http.StatusInternalServerError, status)
	for _, tc := range lifecycleUnaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			status, body := serveLifecycleUnary(t, tc.history, tc.result)
			if !tc.invalid {
				assert.Equal(t, http.StatusOK, status, body)
				return
			}
			assert.Equal(t, http.StatusInternalServerError, status)
			assert.Equal(t, reference, body)
		})
	}
}

// TestInvalidOutput_StreamFrames compares a lifecycle rejection with a
// content rejection that ProviderWire still owns at the same part, which
// writes the frames it wrote before this change.
func TestInvalidOutput_StreamFrames(t *testing.T) {
	invalidSource := provider.StreamPart{Type: provider.PartSource}
	invalidMetadata := provider.StreamPart{Type: provider.PartResponseMeta, Timestamp: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		name      string
		lifecycle []provider.StreamPart
		content   []provider.StreamPart
	}{
		{name: "first part", lifecycle: []provider.StreamPart{lifecyclePart(provider.PartTextDelta, "t")}, content: []provider.StreamPart{invalidSource}},
		{name: "later part", lifecycle: []provider.StreamPart{lifecyclePart(provider.PartTextStart, "t"), lifecyclePart(provider.PartTextStart, "u")}, content: []provider.StreamPart{lifecyclePart(provider.PartTextStart, "t"), invalidSource}},
		{name: "after a written finish", lifecycle: []provider.StreamPart{finishPart(), lifecyclePart(provider.PartTextStart, "late")}, content: []provider.StreamPart{finishPart(), invalidMetadata}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := newLifecycleStreamHarness(t, tc.lifecycle).serve(streamRequest(`{"prompt":[]}`))
			content := newLifecycleStreamHarness(t, tc.content).serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, content.Code, lifecycle.Code)
			assert.Equal(t, content.Header(), lifecycle.Header())
			assert.Equal(t, content.Body.String(), lifecycle.Body.String())
			assert.Contains(t, lifecycle.Body.String(), `"outcome":"selected"`)
		})
	}
}

// TestInvalidOutput_UnaryDocuments shows that the order of lifecycle and
// representation checks cannot be observed.
func TestInvalidOutput_UnaryDocuments(t *testing.T) {
	invalidText := provider.GenerateContentPart{Type: provider.ContentText, Text: string([]byte{0xff})}
	results := map[string]*provider.GenerateResult{
		"lifecycle":      lifecycleGenerate("future", provider.GenerateContentPart{Type: provider.ContentText, Text: "ok"}),
		"representation": lifecycleGenerate(provider.FinishReasonStop, invalidText),
		"both":           lifecycleGenerate("future", invalidText),
	}
	bodies := make(map[string]string, len(results))
	for name, result := range results {
		harness := newRuntimeHarness(t, testLimits())
		harness.resolver.resolved.Candidates = []catalog.ConfiguredCandidate{{Provider: "native", ProviderInstance: "configured", ModelID: "backend"}}
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }
		response := harness.serve(validRequest(`{"prompt":[]}`))
		require.Equal(t, 500, response.Code, name)
		bodies[name] = response.Body.String()
	}
	assert.Contains(t, bodies["lifecycle"], `"outcome":"selected"`)
	assert.Equal(t, bodies["representation"], bodies["lifecycle"])
	assert.Equal(t, bodies["representation"], bodies["both"])
}
