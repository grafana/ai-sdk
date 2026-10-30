package execution

import "github.com/grafana/ai-sdk/provider"

type toolPhase uint8

const (
	toolInputOpen toolPhase = iota + 1
	toolInputClosed
	toolCallEmitted
	toolResultPreliminary
	toolResultEmitted
)

type streamTool struct {
	reemitted        bool
	providerExecuted bool
	approvalID       string
	name             string
	phase            toolPhase
}

// StreamValidator checks the output lifecycle of one stream, part by part. It
// is not safe for concurrent use, and each stream needs its own.
//
// A stream is a sequence of response segments. A segment opens at the start
// of the stream and at every response-metadata part, and a finish ends it.
// Response metadata and finish are accepted only while quiescent: no text,
// reasoning or tool input is open and no tool result preview is pending,
// including calls pending in the prompt's [ToolHistory]. After a finish, only
// response metadata, raw and error parts may follow.
//
// The other rules are: a stream-start only as the first part; text that never
// overlaps and never reuses an ID within its segment; reasoning blocks that may
// run concurrently but never reuse an ID within their segment; the tool input,
// call, preview, result and approval phases, with tool and approval IDs unique
// within the stream; and a finish with no warnings, a finish reason, usage and
// a registered unified finish reason. Error and raw parts change no state other
// than the start position.
type StreamValidator struct {
	started         bool
	segmentFinished bool
	activeText      string
	textIDs         map[string]struct{}
	reasoning       map[string]struct{}
	reasoningIDs    map[string]struct{}
	tools           map[string]streamTool
	history         map[string]streamTool
	approvalIDs     map[string]struct{}
}

// NewStreamValidator returns a validator seeded from its own copy of history.
func NewStreamValidator(history ToolHistory) *StreamValidator {
	validator := &StreamValidator{
		textIDs:      make(map[string]struct{}),
		reasoning:    make(map[string]struct{}),
		reasoningIDs: make(map[string]struct{}),
		tools:        make(map[string]streamTool),
		history:      make(map[string]streamTool, len(history)),
		approvalIDs:  make(map[string]struct{}),
	}
	for id, name := range history {
		validator.history[id] = streamTool{name: name, phase: toolCallEmitted, providerExecuted: true}
	}
	return validator
}

// Check validates the next part and, when it is valid, records it. It returns
// [ErrInvalidOutput] for a part that breaks a rule.
func (v *StreamValidator) Check(part provider.StreamPart) error {
	if !v.check(part) {
		return ErrInvalidOutput
	}
	v.started = true
	return nil
}

// SegmentFinished reports whether the last lifecycle part accepted was a
// finish, so that a provider close now ends the stream cleanly. Raw and error
// parts are not lifecycle parts.
func (v *StreamValidator) SegmentFinished() bool { return v.segmentFinished }

func (v *StreamValidator) check(part provider.StreamPart) bool {
	if v.segmentFinished && !allowedAfterFinish(part.Type) {
		return false
	}
	switch part.Type {
	case provider.PartStreamStart:
		return !v.started
	case provider.PartResponseMeta:
		if !v.quiescent() {
			return false
		}
		v.startSegment()
	case provider.PartTextStart:
		if _, used := v.textIDs[part.ID]; used || v.activeText != "" || part.ID == "" {
			return false
		}
		v.textIDs[part.ID] = struct{}{}
		v.activeText = part.ID
	case provider.PartTextDelta, provider.PartTextEnd:
		if v.activeText == "" || part.ID != v.activeText {
			return false
		}
		if part.Type == provider.PartTextEnd {
			v.activeText = ""
		}
	case provider.PartReasoningStart:
		if _, used := v.reasoningIDs[part.ID]; used || part.ID == "" {
			return false
		}
		v.reasoningIDs[part.ID] = struct{}{}
		v.reasoning[part.ID] = struct{}{}
	case provider.PartReasoningDelta, provider.PartReasoningEnd:
		if _, active := v.reasoning[part.ID]; !active {
			return false
		}
		if part.Type == provider.PartReasoningEnd {
			delete(v.reasoning, part.ID)
		}
	case provider.PartReasoningFile, provider.PartFile, provider.PartSource, provider.PartCustom, provider.PartError, provider.PartRaw:
	case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall, provider.PartToolResult:
		return v.checkTool(part)
	case provider.PartToolApprovalRequest:
		return v.checkApproval(part)
	case provider.PartFinish:
		if !v.quiescent() || len(part.Warnings) != 0 || part.FinishReason == nil || part.Usage == nil || !registeredFinishReason(part.FinishReason.Unified) {
			return false
		}
		v.segmentFinished = true
	default:
		return false
	}
	return true
}

func allowedAfterFinish(partType provider.StreamPartType) bool {
	switch partType {
	case provider.PartResponseMeta, provider.PartRaw, provider.PartError:
		return true
	default:
		return false
	}
}

func (v *StreamValidator) quiescent() bool {
	if v.activeText != "" || len(v.reasoning) != 0 {
		return false
	}
	for _, tool := range v.tools {
		if tool.phase == toolInputOpen || tool.phase == toolResultPreliminary {
			return false
		}
	}
	for _, tool := range v.history {
		if tool.phase == toolResultPreliminary {
			return false
		}
	}
	return true
}

func (v *StreamValidator) startSegment() {
	v.segmentFinished = false
	clear(v.textIDs)
	clear(v.reasoningIDs)
}

func (v *StreamValidator) checkTool(part provider.StreamPart) bool {
	id := part.ID
	if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
		id = part.ToolCallID
	}
	if id == "" {
		return false
	}
	current, exists := v.tools[id]
	_, seenInHistory := v.history[id]
	historical := false
	if !exists && (part.Type == provider.PartToolResult || part.Type == provider.PartToolCall) {
		current, historical = v.history[id]
		exists = historical
	}
	next := current
	switch part.Type {
	case provider.PartToolInputStart:
		if exists || seenInHistory || part.ToolName == "" || isTrue(part.Preliminary) {
			return false
		}
		next = streamTool{name: part.ToolName, phase: toolInputOpen, providerExecuted: part.ProviderExecuted}
	case provider.PartToolInputDelta, provider.PartToolInputEnd:
		if !exists || current.phase != toolInputOpen || part.ProviderExecuted || isTrue(part.Dynamic) || isTrue(part.Preliminary) {
			return false
		}
		if part.Type == provider.PartToolInputEnd {
			next.phase = toolInputClosed
		}
	case provider.PartToolCall:
		if part.ToolName == "" || isTrue(part.Preliminary) {
			return false
		}
		if historical && (!part.ProviderExecuted || current.phase != toolCallEmitted || current.reemitted || current.name != part.ToolName) {
			return false
		}
		if !historical && exists && (current.phase != toolInputClosed || current.name != part.ToolName) {
			return false
		}
		next = streamTool{name: part.ToolName, phase: toolCallEmitted, providerExecuted: part.ProviderExecuted, reemitted: historical, approvalID: current.approvalID}
	case provider.PartToolResult:
		if !exists || part.ToolName == "" || (current.phase != toolCallEmitted && current.phase != toolResultPreliminary) || current.name != part.ToolName || !validToolResult(part.Result) {
			return false
		}
		next.phase = toolResultEmitted
		if isTrue(part.Preliminary) {
			next.phase = toolResultPreliminary
		}
	}
	if historical {
		v.history[id] = next
	} else {
		v.tools[id] = next
	}
	return true
}

func (v *StreamValidator) checkApproval(part provider.StreamPart) bool {
	current, exists := v.tools[part.ToolCallID]
	historical := false
	if !exists {
		current, exists = v.history[part.ToolCallID]
		historical = exists
	}
	if !exists || !current.providerExecuted || current.phase != toolCallEmitted || current.approvalID != "" || part.ApprovalID == "" || part.ProviderExecuted || part.Dynamic != nil || part.Preliminary != nil {
		return false
	}
	if _, used := v.approvalIDs[part.ApprovalID]; used {
		return false
	}
	v.approvalIDs[part.ApprovalID] = struct{}{}
	current.approvalID = part.ApprovalID
	if historical {
		v.history[part.ToolCallID] = current
	} else {
		v.tools[part.ToolCallID] = current
	}
	return true
}
