package execution

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func streamPart(kind provider.StreamPartType, id string) provider.StreamPart {
	return provider.StreamPart{Type: kind, ID: id}
}

func toolInput(kind provider.StreamPartType, id, name string) provider.StreamPart {
	return provider.StreamPart{Type: kind, ID: id, ToolName: name}
}

func toolCall(id, name string, providerExecuted bool) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: `{}`, ProviderExecuted: providerExecuted}
}

func toolResult(id, name, result string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: name, Result: json.RawMessage(result)}
}

func previewResult(id, name string) provider.StreamPart {
	part := toolResult(id, name, `{"partial":true}`)
	part.Preliminary = new(true)
	return part
}

func approval(callID, approvalID string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

func validFinish() provider.StreamPart {
	return provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
}

func withPart(part provider.StreamPart, change func(*provider.StreamPart)) provider.StreamPart {
	change(&part)
	return part
}

// streamRuleCase is a part sequence and the index of the first part the
// validator rejects, or -1 when it accepts every part.
type streamRuleCase struct {
	name     string
	history  ToolHistory
	parts    []provider.StreamPart
	rejectAt int
}

func streamRuleCases() []streamRuleCase {
	source := provider.StreamPart{Type: provider.PartSource}
	custom := provider.StreamPart{Type: provider.PartCustom, Kind: "vendor.kind"}
	metadata := provider.StreamPart{Type: provider.PartResponseMeta}
	raw := provider.StreamPart{Type: provider.PartRaw}
	failure := provider.StreamPart{Type: provider.PartError}
	start := provider.StreamPart{Type: provider.PartStreamStart}
	pending := ToolHistory{"hosted": "search"}
	return []streamRuleCase{
		{name: "first stream-start", parts: []provider.StreamPart{start, validFinish()}, rejectAt: -1},
		{name: "repeated stream-start", parts: []provider.StreamPart{start, start}, rejectAt: 1},
		{name: "stream-start after raw", parts: []provider.StreamPart{raw, start}, rejectAt: 1},
		{name: "stream-start after error", parts: []provider.StreamPart{failure, start}, rejectAt: 1},
		{name: "stream-start after text", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), start}, rejectAt: 1},

		{name: "metadata before content", parts: []provider.StreamPart{start, raw, failure, metadata, validFinish()}, rejectAt: -1},
		{name: "consecutive metadata", parts: []provider.StreamPart{metadata, metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after source", parts: []provider.StreamPart{source, metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after custom", parts: []provider.StreamPart{custom, metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after closed text", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t"), metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after files", parts: []provider.StreamPart{{Type: provider.PartFile}, {Type: provider.PartReasoningFile}, metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after a closed tool call", parts: []provider.StreamPart{toolCall("c", "f", false), metadata, validFinish()}, rejectAt: -1},
		{name: "metadata after an approval", history: pending, parts: []provider.StreamPart{approval("hosted", "a"), metadata}, rejectAt: -1},
		{name: "metadata with open text", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), metadata}, rejectAt: 1},
		{name: "metadata with open reasoning", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "r"), metadata}, rejectAt: 1},
		{name: "metadata with open tool input", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), metadata}, rejectAt: 1},
		{name: "metadata with a pending preview", parts: []provider.StreamPart{toolCall("c", "f", true), previewResult("c", "f"), metadata}, rejectAt: 2},
		{name: "metadata with a pending history preview", history: pending, parts: []provider.StreamPart{previewResult("hosted", "search"), metadata}, rejectAt: 1},

		{name: "two segments", parts: []provider.StreamPart{metadata, streamPart(provider.PartTextStart, "0"), streamPart(provider.PartTextEnd, "0"), validFinish(), raw, metadata, streamPart(provider.PartTextStart, "0"), streamPart(provider.PartTextEnd, "0"), validFinish()}, rejectAt: -1},
		{name: "segment without finish", parts: []provider.StreamPart{toolCall("c", "f", false), metadata, toolCall("d", "f", false), validFinish()}, rejectAt: -1},
		{name: "reasoning ID reused in a later segment", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "r"), streamPart(provider.PartReasoningEnd, "r"), metadata, streamPart(provider.PartReasoningStart, "r"), streamPart(provider.PartReasoningEnd, "r")}, rejectAt: -1},
		{name: "tool ID reused in a later segment", parts: []provider.StreamPart{toolCall("c", "f", false), metadata, toolCall("c", "f", false)}, rejectAt: 2},
		{name: "approval ID reused in a later segment", parts: []provider.StreamPart{toolCall("c", "f", true), approval("c", "a"), toolResult("c", "f", `{}`), metadata, toolCall("d", "f", true), approval("d", "a")}, rejectAt: 5},
		{name: "result in a later segment", parts: []provider.StreamPart{toolCall("c", "f", true), validFinish(), metadata, toolResult("c", "f", `{}`), validFinish()}, rejectAt: -1},
		{name: "metadata, raw and error after finish", parts: []provider.StreamPart{validFinish(), raw, failure, metadata, validFinish()}, rejectAt: -1},
		{name: "content after finish", parts: []provider.StreamPart{validFinish(), streamPart(provider.PartTextStart, "t")}, rejectAt: 1},
		{name: "source after finish", parts: []provider.StreamPart{validFinish(), raw, source}, rejectAt: 2},
		{name: "second finish", parts: []provider.StreamPart{validFinish(), validFinish()}, rejectAt: 1},
		{name: "stream-start after finish", parts: []provider.StreamPart{validFinish(), start}, rejectAt: 1},
		{name: "unknown part after finish", parts: []provider.StreamPart{validFinish(), {Type: "future"}}, rejectAt: 1},

		{name: "text block", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextDelta, "t"), streamPart(provider.PartTextEnd, "t"), streamPart(provider.PartTextStart, "u"), streamPart(provider.PartTextEnd, "u"), validFinish()}, rejectAt: -1},
		{name: "empty text ID", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "")}, rejectAt: 0},
		{name: "overlapping text", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextStart, "u")}, rejectAt: 1},
		{name: "reused text ID", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t"), streamPart(provider.PartTextStart, "t")}, rejectAt: 2},
		{name: "text ID reused in a later segment", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t"), metadata, streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t")}, rejectAt: -1},
		{name: "text delta without start", parts: []provider.StreamPart{streamPart(provider.PartTextDelta, "t")}, rejectAt: 0},
		{name: "text delta for another ID", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextDelta, "u")}, rejectAt: 1},
		{name: "text end without start", parts: []provider.StreamPart{streamPart(provider.PartTextEnd, "t")}, rejectAt: 0},
		{name: "error and raw keep text state", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), failure, raw, streamPart(provider.PartTextDelta, "t"), streamPart(provider.PartTextEnd, "t")}, rejectAt: -1},

		{name: "concurrent reasoning", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "a"), streamPart(provider.PartReasoningStart, "b"), streamPart(provider.PartReasoningDelta, "a"), streamPart(provider.PartReasoningEnd, "b"), streamPart(provider.PartReasoningEnd, "a"), validFinish()}, rejectAt: -1},
		{name: "empty reasoning ID", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "")}, rejectAt: 0},
		{name: "reused reasoning ID", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "r"), streamPart(provider.PartReasoningEnd, "r"), streamPart(provider.PartReasoningStart, "r")}, rejectAt: 2},
		{name: "repeated active reasoning ID", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "r"), streamPart(provider.PartReasoningStart, "r")}, rejectAt: 1},
		{name: "reasoning delta without start", parts: []provider.StreamPart{streamPart(provider.PartReasoningDelta, "r")}, rejectAt: 0},
		{name: "reasoning end without start", parts: []provider.StreamPart{streamPart(provider.PartReasoningEnd, "r")}, rejectAt: 0},

		{name: "streamed tool call", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolInput(provider.PartToolInputDelta, "c", ""), toolInput(provider.PartToolInputEnd, "c", ""), toolCall("c", "f", false), toolResult("c", "f", `{}`), validFinish()}, rejectAt: -1},
		{name: "standalone tool call", parts: []provider.StreamPart{toolCall("c", "f", false), validFinish()}, rejectAt: -1},
		{name: "empty tool input ID", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "", "f")}, rejectAt: 0},
		{name: "empty tool call ID", parts: []provider.StreamPart{toolCall("", "f", false)}, rejectAt: 0},
		{name: "tool input without name", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "")}, rejectAt: 0},
		{name: "preliminary tool input", parts: []provider.StreamPart{withPart(toolInput(provider.PartToolInputStart, "c", "f"), func(p *provider.StreamPart) { p.Preliminary = new(true) })}, rejectAt: 0},
		{name: "repeated tool input", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolInput(provider.PartToolInputStart, "c", "f")}, rejectAt: 1},
		{name: "tool input for a history call", history: pending, parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "hosted", "search")}, rejectAt: 0},
		{name: "tool delta without input", parts: []provider.StreamPart{toolInput(provider.PartToolInputDelta, "c", "")}, rejectAt: 0},
		{name: "provider-executed tool delta", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), withPart(toolInput(provider.PartToolInputDelta, "c", ""), func(p *provider.StreamPart) { p.ProviderExecuted = true })}, rejectAt: 1},
		{name: "dynamic tool delta", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), withPart(toolInput(provider.PartToolInputDelta, "c", ""), func(p *provider.StreamPart) { p.Dynamic = new(true) })}, rejectAt: 1},
		{name: "preliminary tool end", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), withPart(toolInput(provider.PartToolInputEnd, "c", ""), func(p *provider.StreamPart) { p.Preliminary = new(true) })}, rejectAt: 1},
		{name: "repeated tool end", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolInput(provider.PartToolInputEnd, "c", ""), toolInput(provider.PartToolInputEnd, "c", "")}, rejectAt: 2},
		{name: "tool call with open input", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolCall("c", "f", false)}, rejectAt: 1},
		{name: "tool call with another name", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolInput(provider.PartToolInputEnd, "c", ""), toolCall("c", "g", false)}, rejectAt: 2},
		{name: "tool call without name", parts: []provider.StreamPart{toolCall("c", "", false)}, rejectAt: 0},
		{name: "preliminary tool call", parts: []provider.StreamPart{withPart(toolCall("c", "f", false), func(p *provider.StreamPart) { p.Preliminary = new(true) })}, rejectAt: 0},
		{name: "repeated tool call", parts: []provider.StreamPart{toolCall("c", "f", false), toolCall("c", "f", false)}, rejectAt: 1},
		{name: "result for an unknown call", parts: []provider.StreamPart{toolResult("c", "f", `{}`)}, rejectAt: 0},
		{name: "result with another name", parts: []provider.StreamPart{toolCall("c", "f", false), toolResult("c", "g", `{}`)}, rejectAt: 1},
		{name: "result before the call", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), toolInput(provider.PartToolInputEnd, "c", ""), toolResult("c", "f", `{}`)}, rejectAt: 2},
		{name: "repeated result", parts: []provider.StreamPart{toolCall("c", "f", false), toolResult("c", "f", `{}`), toolResult("c", "f", `{}`)}, rejectAt: 2},
		{name: "empty result", parts: []provider.StreamPart{toolCall("c", "f", false), toolResult("c", "f", ``)}, rejectAt: 1},
		{name: "invalid result", parts: []provider.StreamPart{toolCall("c", "f", false), toolResult("c", "f", `{`)}, rejectAt: 1},
		{name: "null result", parts: []provider.StreamPart{toolCall("c", "f", false), toolResult("c", "f", ` null `)}, rejectAt: 1},
		{name: "preview then final result", parts: []provider.StreamPart{toolCall("c", "f", true), previewResult("c", "f"), previewResult("c", "f"), toolResult("c", "f", `{}`), validFinish()}, rejectAt: -1},

		{name: "history result", history: pending, parts: []provider.StreamPart{toolResult("hosted", "search", `{}`), validFinish()}, rejectAt: -1},
		{name: "history result with another name", history: pending, parts: []provider.StreamPart{toolResult("hosted", "other", `{}`)}, rejectAt: 0},
		{name: "history preview then result", history: pending, parts: []provider.StreamPart{previewResult("hosted", "search"), toolResult("hosted", "search", `{}`), validFinish()}, rejectAt: -1},
		{name: "history re-emission", history: pending, parts: []provider.StreamPart{toolCall("hosted", "search", true), toolResult("hosted", "search", `{}`), validFinish()}, rejectAt: -1},
		{name: "history re-emission not provider-executed", history: pending, parts: []provider.StreamPart{toolCall("hosted", "search", false)}, rejectAt: 0},
		{name: "second history re-emission", history: pending, parts: []provider.StreamPart{toolCall("hosted", "search", true), toolCall("hosted", "search", true)}, rejectAt: 1},
		{name: "history re-emission with another name", history: pending, parts: []provider.StreamPart{toolCall("hosted", "other", true)}, rejectAt: 0},
		{name: "history re-emission after its result", history: pending, parts: []provider.StreamPart{toolResult("hosted", "search", `{}`), toolCall("hosted", "search", true)}, rejectAt: 1},
		{name: "history approval", history: pending, parts: []provider.StreamPart{approval("hosted", "a"), toolResult("hosted", "search", `{}`), validFinish()}, rejectAt: -1},
		{name: "tool input for a resolved history ID", history: pending, parts: []provider.StreamPart{toolResult("hosted", "search", `{}`), toolInput(provider.PartToolInputStart, "hosted", "search")}, rejectAt: 1},
		{name: "history approval after its result", history: pending, parts: []provider.StreamPart{toolResult("hosted", "search", `{}`), approval("hosted", "a")}, rejectAt: 1},
		{name: "history result in a later segment", history: pending, parts: []provider.StreamPart{metadata, validFinish(), metadata, toolResult("hosted", "search", `{}`), validFinish()}, rejectAt: -1},
		{name: "re-emission keeps the history approval", history: pending, parts: []provider.StreamPart{approval("hosted", "a"), toolCall("hosted", "search", true), approval("hosted", "b")}, rejectAt: 2},

		{name: "approval of a provider-executed call", parts: []provider.StreamPart{toolCall("c", "f", true), approval("c", "a"), toolResult("c", "f", `{}`), validFinish()}, rejectAt: -1},
		{name: "approval of an unknown call", parts: []provider.StreamPart{approval("c", "a")}, rejectAt: 0},
		{name: "approval of a client call", parts: []provider.StreamPart{toolCall("c", "f", false), approval("c", "a")}, rejectAt: 1},
		{name: "approval of open input", parts: []provider.StreamPart{withPart(toolInput(provider.PartToolInputStart, "c", "f"), func(p *provider.StreamPart) { p.ProviderExecuted = true }), approval("c", "a")}, rejectAt: 1},
		{name: "repeated approval of one call", parts: []provider.StreamPart{toolCall("c", "f", true), approval("c", "a"), approval("c", "b")}, rejectAt: 2},
		{name: "reused approval ID", parts: []provider.StreamPart{toolCall("c", "f", true), toolCall("d", "f", true), approval("c", "a"), approval("d", "a")}, rejectAt: 3},
		{name: "empty approval ID", parts: []provider.StreamPart{toolCall("c", "f", true), approval("c", "")}, rejectAt: 1},
		{name: "provider-executed approval", parts: []provider.StreamPart{toolCall("c", "f", true), withPart(approval("c", "a"), func(p *provider.StreamPart) { p.ProviderExecuted = true })}, rejectAt: 1},
		{name: "approval with a dynamic flag", parts: []provider.StreamPart{toolCall("c", "f", true), withPart(approval("c", "a"), func(p *provider.StreamPart) { p.Dynamic = new(false) })}, rejectAt: 1},
		{name: "approval with a preliminary flag", parts: []provider.StreamPart{toolCall("c", "f", true), withPart(approval("c", "a"), func(p *provider.StreamPart) { p.Preliminary = new(false) })}, rejectAt: 1},

		{name: "finish with open tool input", parts: []provider.StreamPart{toolInput(provider.PartToolInputStart, "c", "f"), validFinish()}, rejectAt: 1},
		{name: "finish with a pending preview", parts: []provider.StreamPart{toolCall("c", "f", true), previewResult("c", "f"), validFinish()}, rejectAt: 2},
		{name: "finish with a pending history preview", history: pending, parts: []provider.StreamPart{previewResult("hosted", "search"), validFinish()}, rejectAt: 1},
		{name: "finish with active text", parts: []provider.StreamPart{streamPart(provider.PartTextStart, "t"), validFinish()}, rejectAt: 1},
		{name: "finish with active reasoning", parts: []provider.StreamPart{streamPart(provider.PartReasoningStart, "r"), validFinish()}, rejectAt: 1},
		{name: "finish with warnings", parts: []provider.StreamPart{withPart(validFinish(), func(p *provider.StreamPart) { p.Warnings = []provider.Warning{{Type: provider.WarnUnsupported}} })}, rejectAt: 0},
		{name: "finish without a reason", parts: []provider.StreamPart{withPart(validFinish(), func(p *provider.StreamPart) { p.FinishReason = nil })}, rejectAt: 0},
		{name: "finish without usage", parts: []provider.StreamPart{withPart(validFinish(), func(p *provider.StreamPart) { p.Usage = nil })}, rejectAt: 0},
		{name: "unregistered finish reason", parts: []provider.StreamPart{withPart(validFinish(), func(p *provider.StreamPart) { p.FinishReason = &provider.FinishReason{Unified: "future"} })}, rejectAt: 0},
		{name: "finish ignores a closed client call", parts: []provider.StreamPart{toolCall("c", "f", false), streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t"), validFinish()}, rejectAt: -1},

		{name: "unknown part type", parts: []provider.StreamPart{{Type: "future"}}, rejectAt: 0},
	}
}

func TestStreamValidator_Rules(t *testing.T) {
	for _, tc := range streamRuleCases() {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewStreamValidator(tc.history)
			rejected := -1
			for i, part := range tc.parts {
				if err := validator.Check(part); err != nil {
					assert.ErrorIs(t, err, ErrInvalidOutput)
					rejected = i
					break
				}
			}
			assert.Equal(t, tc.rejectAt, rejected)
		})
	}
}

func TestStreamValidator_SegmentFinished(t *testing.T) {
	validator := NewStreamValidator(nil)
	steps := []struct {
		part     provider.StreamPart
		finished bool
	}{
		{part: provider.StreamPart{Type: provider.PartStreamStart}},
		{part: provider.StreamPart{Type: provider.PartResponseMeta}},
		{part: withPart(validFinish(), func(p *provider.StreamPart) {
			p.FinishReason = &provider.FinishReason{Unified: provider.FinishReasonToolCalls}
		}), finished: true},
		{part: provider.StreamPart{Type: provider.PartRaw}, finished: true},
		{part: provider.StreamPart{Type: provider.PartError}, finished: true},
		{part: provider.StreamPart{Type: provider.PartResponseMeta}},
		{part: validFinish(), finished: true},
	}
	for i, step := range steps {
		assert.NoError(t, validator.Check(step.part), i)
		assert.Equal(t, step.finished, validator.SegmentFinished(), i)
	}
	assert.ErrorIs(t, validator.Check(validFinish()), ErrInvalidOutput)
	assert.True(t, validator.SegmentFinished(), "a refused part changes nothing")
}

func TestStreamValidator_HistoryIsNotShared(t *testing.T) {
	history := ToolHistory{"hosted": "search"}
	first, second := NewStreamValidator(history), NewStreamValidator(history)
	assert.NoError(t, first.Check(toolResult("hosted", "search", `{}`)))
	assert.ErrorIs(t, first.Check(toolResult("hosted", "search", `{}`)), ErrInvalidOutput)
	assert.NoError(t, second.Check(toolResult("hosted", "search", `{}`)))
	assert.Equal(t, ToolHistory{"hosted": "search"}, history)
}
