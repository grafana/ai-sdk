package v4

import (
	"bytes"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type streamToolStartEvent struct {
	Type     provider.StreamPartType `json:"type"`
	ID       string                  `json:"id"`
	ToolName string                  `json:"toolName"`
}
type streamToolCallEvent struct {
	Type       provider.StreamPartType `json:"type"`
	ToolCallID string                  `json:"toolCallId"`
	ToolName   string                  `json:"toolName"`
	Input      string                  `json:"input"`
}
type streamToolResultEvent struct {
	Type       provider.StreamPartType `json:"type"`
	ToolCallID string                  `json:"toolCallId"`
	ToolName   string                  `json:"toolName"`
	Result     json.RawMessage         `json:"result"`
	IsError    bool                    `json:"isError,omitempty"`
}
type toolStreamPhase uint8

const (
	toolInputOpen toolStreamPhase = iota + 1
	toolInputClosed
	toolCallEmitted
	toolResultEmitted
)

type toolStreamState struct {
	name  string
	phase toolStreamPhase
}

func (h *handler) processToolStreamPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	if part.ProviderExecuted || (part.Dynamic != nil && *part.Dynamic) || (part.Preliminary != nil && *part.Preliminary) {
		return streamPartAdapterFailure
	}
	id := part.ID
	if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
		id = part.ToolCallID
	}
	if id == "" || int64(len(id)) > h.limits.StreamFrameBytes || !utf8.ValidString(id) {
		return streamPartAdapterFailure
	}
	current, exists := state.tools[id]
	next := current
	event := streamEvent{typeName: part.Type, id: id, toolName: part.ToolName, delta: part.Delta, input: part.Input, result: part.Result, isError: part.IsError}
	switch part.Type {
	case provider.PartToolInputStart:
		if exists || part.ToolName == "" {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolInputOpen}
	case provider.PartToolInputDelta:
		if !exists || current.phase != toolInputOpen {
			return streamPartAdapterFailure
		}
	case provider.PartToolInputEnd:
		if !exists || current.phase != toolInputOpen {
			return streamPartAdapterFailure
		}
		next.phase = toolInputClosed
	case provider.PartToolCall:
		if part.ToolName == "" || (exists && (current.phase != toolInputClosed || current.name != part.ToolName)) {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolCallEmitted}
	case provider.PartToolResult:
		if !exists || current.phase != toolCallEmitted || current.name != part.ToolName {
			return streamPartAdapterFailure
		}
		if int64(len(part.Result)) > h.limits.StreamFrameBytes || !json.Valid(part.Result) || bytes.Equal(bytes.TrimSpace(part.Result), []byte("null")) {
			return streamPartAdapterFailure
		}
		next.phase = toolResultEmitted
	}
	if !exists && len(state.tools) >= h.limits.StreamParts {
		return streamPartAdapterFailure
	}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	state.tools[id] = next
	state.textStarted = true
	return streamPartContinue
}
