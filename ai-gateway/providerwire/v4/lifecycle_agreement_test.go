package v4

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agreementCase struct {
	name    string
	history execution.ToolHistory
	parts   []provider.StreamPart
}

func agreementPart(kind provider.StreamPartType, id string) provider.StreamPart {
	return provider.StreamPart{Type: kind, ID: id}
}

func agreementCall(id, name string, providerExecuted bool) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: `{}`, ProviderExecuted: providerExecuted}
}

func agreementResult(id, name, result string, preliminary bool) provider.StreamPart {
	part := provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: name, Result: json.RawMessage(result)}
	if preliminary {
		part.Preliminary = new(true)
	}
	return part
}

func agreementApproval(callID, approvalID string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

func changed(part provider.StreamPart, change func(*provider.StreamPart)) provider.StreamPart {
	change(&part)
	return part
}

// streamAgreementCases holds one sequence per stream rule class, with valid
// content so that only lifecycle rules decide.
func streamAgreementCases() []agreementCase {
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
	textStart, textEnd := agreementPart(provider.PartTextStart, "0"), agreementPart(provider.PartTextEnd, "0")
	return []agreementCase{
		{name: "valid start and finish", parts: []provider.StreamPart{start, finishPart()}},
		{name: "repeated stream-start", parts: []provider.StreamPart{start, start}},
		{name: "stream-start after raw", parts: []provider.StreamPart{raw, start}},
		{name: "stream-start after error", parts: []provider.StreamPart{failure, start}},
		{name: "stream-start after finish", parts: []provider.StreamPart{finishPart(), start}},

		{name: "metadata before content", parts: []provider.StreamPart{raw, failure, metadata, finishPart()}},
		{name: "consecutive metadata", parts: []provider.StreamPart{metadata, metadata, finishPart()}},
		{name: "metadata after closed content", parts: []provider.StreamPart{source, custom, file, textStart, textEnd, agreementCall("c", "f", false), metadata, finishPart()}},
		{name: "metadata after an approval", history: pending, parts: []provider.StreamPart{agreementApproval("hosted", "a"), metadata, agreementResult("hosted", "search", `{}`, false), finishPart()}},
		{name: "metadata with open text", parts: []provider.StreamPart{textStart, metadata}},
		{name: "metadata with open reasoning", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "r"), metadata}},
		{name: "metadata with open tool input", parts: []provider.StreamPart{inputStart, metadata}},
		{name: "metadata with a pending preview", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementResult("c", "f", `{}`, true), metadata}},
		{name: "metadata with a pending history preview", history: pending, parts: []provider.StreamPart{agreementResult("hosted", "search", `{}`, true), metadata}},

		{name: "two segments", parts: []provider.StreamPart{metadata, textStart, textEnd, finishPart(), raw, metadata, textStart, textEnd, finishPart()}},
		{name: "segment without finish", parts: []provider.StreamPart{agreementCall("c", "f", false), metadata, agreementCall("d", "f", false), finishPart()}},
		{name: "reasoning ID reused in a later segment", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "r"), agreementPart(provider.PartReasoningEnd, "r"), metadata, agreementPart(provider.PartReasoningStart, "r"), agreementPart(provider.PartReasoningEnd, "r"), finishPart()}},
		{name: "tool ID reused in a later segment", parts: []provider.StreamPart{agreementCall("c", "f", false), metadata, agreementCall("c", "f", false)}},
		{name: "approval ID reused in a later segment", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementApproval("c", "a"), agreementResult("c", "f", `{}`, false), metadata, agreementCall("d", "f", true), agreementApproval("d", "a")}},
		{name: "result in a later segment", parts: []provider.StreamPart{agreementCall("c", "f", true), finishPart(), metadata, agreementResult("c", "f", `{}`, false), finishPart()}},
		{name: "history result in a later segment", history: pending, parts: []provider.StreamPart{metadata, finishPart(), metadata, agreementResult("hosted", "search", `{}`, false), finishPart()}},
		{name: "metadata, raw and error after finish", parts: []provider.StreamPart{finishPart(), raw, failure, metadata, finishPart(), failure}},
		{name: "content after finish", parts: []provider.StreamPart{textStart, textEnd, finishPart(), agreementPart(provider.PartTextStart, "late")}},
		{name: "source after finish", parts: []provider.StreamPart{finishPart(), raw, source}},
		{name: "second finish", parts: []provider.StreamPart{finishPart(), finishPart()}},
		{name: "unknown part after finish", parts: []provider.StreamPart{finishPart(), {Type: "future"}}},

		{name: "empty text ID", parts: []provider.StreamPart{agreementPart(provider.PartTextStart, "")}},
		{name: "overlapping text", parts: []provider.StreamPart{agreementPart(provider.PartTextStart, "t"), agreementPart(provider.PartTextStart, "u")}},
		{name: "reused text ID", parts: []provider.StreamPart{textStart, textEnd, textStart}},
		{name: "text delta for another ID", parts: []provider.StreamPart{agreementPart(provider.PartTextStart, "t"), agreementPart(provider.PartTextDelta, "u")}},
		{name: "text end without start", parts: []provider.StreamPart{agreementPart(provider.PartTextEnd, "t")}},
		{name: "concurrent reasoning", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "a"), agreementPart(provider.PartReasoningStart, "b"), agreementPart(provider.PartReasoningEnd, "a"), agreementPart(provider.PartReasoningEnd, "b"), finishPart()}},
		{name: "empty reasoning ID", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "")}},
		{name: "reused reasoning ID", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "r"), agreementPart(provider.PartReasoningEnd, "r"), agreementPart(provider.PartReasoningStart, "r")}},
		{name: "reasoning delta without start", parts: []provider.StreamPart{agreementPart(provider.PartReasoningDelta, "r")}},

		{name: "streamed tool call", parts: []provider.StreamPart{inputStart, inputEnd, agreementCall("c", "f", false), agreementResult("c", "f", `{}`, false), finishPart()}},
		{name: "tool input without name", parts: []provider.StreamPart{changed(inputStart, func(p *provider.StreamPart) { p.ToolName = "" })}},
		{name: "repeated tool input", parts: []provider.StreamPart{inputStart, inputStart}},
		{name: "tool input for a history call", history: pending, parts: []provider.StreamPart{changed(inputStart, func(p *provider.StreamPart) { p.ID = "hosted" })}},
		{name: "dynamic tool delta", parts: []provider.StreamPart{inputStart, {Type: provider.PartToolInputDelta, ID: "c", Dynamic: new(true)}}},
		{name: "tool call with open input", parts: []provider.StreamPart{inputStart, agreementCall("c", "f", false)}},
		{name: "tool call with another name", parts: []provider.StreamPart{inputStart, inputEnd, agreementCall("c", "g", false)}},
		{name: "repeated tool call", parts: []provider.StreamPart{agreementCall("c", "f", false), agreementCall("c", "f", false)}},
		{name: "result for an unknown call", parts: []provider.StreamPart{agreementResult("c", "f", `{}`, false)}},
		{name: "result with another name", parts: []provider.StreamPart{agreementCall("c", "f", false), agreementResult("c", "g", `{}`, false)}},
		{name: "null result", parts: []provider.StreamPart{agreementCall("c", "f", false), agreementResult("c", "f", `null`, false)}},
		{name: "invalid result", parts: []provider.StreamPart{agreementCall("c", "f", false), agreementResult("c", "f", `{`, false)}},
		{name: "preview then final result", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementResult("c", "f", `{}`, true), agreementResult("c", "f", `{}`, false), finishPart()}},
		{name: "history result", history: pending, parts: []provider.StreamPart{agreementResult("hosted", "search", `{}`, false), finishPart()}},
		{name: "tool input for a resolved history ID", history: pending, parts: []provider.StreamPart{agreementResult("hosted", "search", `{}`, false), changed(inputStart, func(p *provider.StreamPart) { p.ID = "hosted" })}},
		{name: "approval with an empty ID", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementApproval("c", "")}},
		{name: "history re-emission not provider-executed", history: pending, parts: []provider.StreamPart{agreementCall("hosted", "search", false)}},
		{name: "second history re-emission", history: pending, parts: []provider.StreamPart{agreementCall("hosted", "search", true), agreementCall("hosted", "search", true)}},
		{name: "history re-emission after its result", history: pending, parts: []provider.StreamPart{agreementResult("hosted", "search", `{}`, false), agreementCall("hosted", "search", true)}},
		{name: "history approval", history: pending, parts: []provider.StreamPart{agreementApproval("hosted", "a"), agreementResult("hosted", "search", `{}`, false), finishPart()}},
		{name: "approval of an unknown call", parts: []provider.StreamPart{agreementApproval("c", "a")}},
		{name: "approval of a client call", parts: []provider.StreamPart{agreementCall("c", "f", false), agreementApproval("c", "a")}},
		{name: "repeated approval of one call", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementApproval("c", "a"), agreementApproval("c", "b")}},
		{name: "reused approval ID", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementCall("d", "f", true), agreementApproval("c", "a"), agreementApproval("d", "a")}},
		{name: "approval with a dynamic flag", parts: []provider.StreamPart{agreementCall("c", "f", true), changed(agreementApproval("c", "a"), func(p *provider.StreamPart) { p.Dynamic = new(false) })}},

		{name: "finish with open tool input", parts: []provider.StreamPart{inputStart, finishPart()}},
		{name: "finish with a pending preview", parts: []provider.StreamPart{agreementCall("c", "f", true), agreementResult("c", "f", `{}`, true), finishPart()}},
		{name: "finish with a pending history preview", history: pending, parts: []provider.StreamPart{agreementResult("hosted", "search", `{}`, true), finishPart()}},
		{name: "finish with active text", parts: []provider.StreamPart{textStart, finishPart()}},
		{name: "finish with active reasoning", parts: []provider.StreamPart{agreementPart(provider.PartReasoningStart, "r"), finishPart()}},
		{name: "finish with warnings", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.Warnings = []provider.Warning{{Type: provider.WarnUnsupported}} })}},
		{name: "finish without a reason", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.FinishReason = nil })}},
		{name: "finish without usage", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.Usage = nil })}},
		{name: "unregistered finish reason", parts: []provider.StreamPart{changed(finishPart(), func(p *provider.StreamPart) { p.FinishReason = &provider.FinishReason{Unified: "future"} })}},
		{name: "unknown part type", parts: []provider.StreamPart{{Type: "future"}}},
	}
}

// streamVerdict is the index of the rejected part, or -1, and the finished
// phase after each accepted part.
type streamVerdict struct {
	rejectAt int
	finished []bool
}

func runtimeVerdict(tc agreementCase) streamVerdict {
	verdict := streamVerdict{rejectAt: -1}
	validator := execution.NewStreamValidator(tc.history)
	for i, part := range tc.parts {
		if validator.Check(part) != nil {
			verdict.rejectAt = i
			return verdict
		}
		verdict.finished = append(verdict.finished, validator.SegmentFinished())
	}
	return verdict
}

// providerWireVerdict runs the parts through ProviderWire's own state the way
// runStream does: a real first stream-start is checked before the loop.
func providerWireVerdict(t *testing.T, tc agreementCase) streamVerdict {
	t.Helper()
	h := &handler{}
	recorder := httptest.NewRecorder()
	state := newStreamState(tc.history)
	verdict := streamVerdict{rejectAt: -1}
	parts := tc.parts
	offset := 0
	if len(parts) > 0 && parts[0].Type == provider.PartStreamStart {
		require.NoError(t, state.validator.Check(parts[0]), "runStream accepts a real first stream-start")
		verdict.finished = append(verdict.finished, false)
		parts, offset = parts[1:], 1
	}
	for i, part := range parts {
		switch h.processStreamPart(recorder, state, part) {
		case streamPartAdapterFailure:
			verdict.rejectAt = i + offset
			return verdict
		case streamPartWriterFailure:
			require.Fail(t, "writer failure")
		}
		verdict.finished = append(verdict.finished, state.phase == segmentFinished)
	}
	return verdict
}

func TestLifecycleAgreement_Stream(t *testing.T) {
	for _, tc := range streamAgreementCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, runtimeVerdict(tc), providerWireVerdict(t, tc))
		})
	}
}

func newAgreementHarness(t *testing.T, parts []provider.StreamPart) *runtimeHarness {
	t.Helper()
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.Candidates = []catalog.ConfiguredCandidate{{Provider: "native", ProviderInstance: "configured", ModelID: "backend"}}
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(parts...)}, nil
	}
	return harness
}

// TestLifecycleAgreement_StreamFrames compares a lifecycle rejection with a
// content rejection that ProviderWire still owns at the same part, which
// writes the frames it wrote before this change.
func TestLifecycleAgreement_StreamFrames(t *testing.T) {
	invalidSource := provider.StreamPart{Type: provider.PartSource}
	invalidMetadata := provider.StreamPart{Type: provider.PartResponseMeta, Timestamp: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		name      string
		lifecycle []provider.StreamPart
		content   []provider.StreamPart
	}{
		{name: "first part", lifecycle: []provider.StreamPart{agreementPart(provider.PartTextDelta, "t")}, content: []provider.StreamPart{invalidSource}},
		{name: "later part", lifecycle: []provider.StreamPart{agreementPart(provider.PartTextStart, "t"), agreementPart(provider.PartTextStart, "u")}, content: []provider.StreamPart{agreementPart(provider.PartTextStart, "t"), invalidSource}},
		{name: "after a written finish", lifecycle: []provider.StreamPart{finishPart(), agreementPart(provider.PartTextStart, "late")}, content: []provider.StreamPart{finishPart(), invalidMetadata}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := newAgreementHarness(t, tc.lifecycle).serve(streamRequest(`{"prompt":[]}`))
			content := newAgreementHarness(t, tc.content).serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, content.Code, lifecycle.Code)
			assert.Equal(t, content.Header(), lifecycle.Header())
			assert.Equal(t, content.Body.String(), lifecycle.Body.String())
			assert.Contains(t, lifecycle.Body.String(), `"outcome":"selected"`)
		})
	}
}
