package v4

import (
	"bytes"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type streamToolStartEvent struct {
	InputEncoding    provider.ToolInputEncoding `json:"inputEncoding,omitempty"`
	Type             provider.StreamPartType    `json:"type"`
	ID               string                     `json:"id"`
	ToolName         string                     `json:"toolName"`
	ProviderExecuted bool                       `json:"providerExecuted,omitempty"`
	Dynamic          *bool                      `json:"dynamic,omitempty"`
	Metadata         provider.ProviderMetadata  `json:"providerMetadata,omitzero"`
}
type streamToolDeltaEvent struct {
	Type     provider.StreamPartType   `json:"type"`
	ID       string                    `json:"id"`
	Delta    string                    `json:"delta"`
	Metadata provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}
type streamToolEndEvent struct {
	Type     provider.StreamPartType   `json:"type"`
	ID       string                    `json:"id"`
	Metadata provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}
type streamToolCallEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            string                    `json:"input"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Metadata         provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}
type streamToolResultEvent struct {
	Type        provider.StreamPartType   `json:"type"`
	ToolCallID  string                    `json:"toolCallId"`
	ToolName    string                    `json:"toolName"`
	Result      json.RawMessage           `json:"result"`
	IsError     bool                      `json:"isError,omitempty"`
	Dynamic     *bool                     `json:"dynamic,omitempty"`
	Preliminary *bool                     `json:"preliminary,omitempty"`
	Metadata    provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}
type toolStreamPhase uint8

const (
	toolInputOpen toolStreamPhase = iota + 1
	toolInputClosed
	toolCallEmitted
	toolResultPreliminary
	toolResultEmitted
)

type toolStreamState struct {
	reemitted        bool
	providerExecuted bool
	approvalID       string
	name             string
	phase            toolStreamPhase
}

func (h *handler) processToolStreamPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	id := part.ID
	if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
		id = part.ToolCallID
	}
	if id == "" || !utf8.ValidString(id) {
		return streamPartAdapterFailure
	}
	current, exists := state.tools[id]
	_, seenInHistory := state.history[id]
	historical := false
	if !exists && (part.Type == provider.PartToolResult || part.Type == provider.PartToolCall) {
		current, historical = state.history[id]
		exists = historical
	}
	next := current
	event := streamEvent{typeName: part.Type, id: id, toolName: part.ToolName, inputEncoding: part.InputEncoding, delta: part.Delta, input: part.Input, result: part.Result, isError: part.IsError, metadata: part.ProviderMetadata}
	switch part.Type {
	case provider.PartToolInputStart:
		if exists || seenInHistory || part.ToolName == "" || (part.Preliminary != nil && *part.Preliminary) {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolInputOpen, providerExecuted: part.ProviderExecuted}
		event.providerExecuted, event.dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolInputDelta, provider.PartToolInputEnd:
		if !exists || current.phase != toolInputOpen || part.ProviderExecuted || (part.Dynamic != nil && *part.Dynamic) || (part.Preliminary != nil && *part.Preliminary) {
			return streamPartAdapterFailure
		}
		if part.Type == provider.PartToolInputEnd {
			next.phase = toolInputClosed
		}
	case provider.PartToolCall:
		if part.ToolName == "" || (part.Preliminary != nil && *part.Preliminary) || (historical && (!part.ProviderExecuted || current.phase != toolCallEmitted || current.reemitted || current.name != part.ToolName)) || (!historical && exists && (current.phase != toolInputClosed || current.name != part.ToolName)) {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolCallEmitted, providerExecuted: part.ProviderExecuted, reemitted: historical, approvalID: current.approvalID}
		event.providerExecuted, event.dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolResult:
		if !exists || part.ToolName == "" || (current.phase != toolCallEmitted && current.phase != toolResultPreliminary) || current.name != part.ToolName {
			return streamPartAdapterFailure
		}
		if !json.Valid(part.Result) || bytes.Equal(bytes.TrimSpace(part.Result), []byte("null")) {
			return streamPartAdapterFailure
		}
		if part.Preliminary != nil && *part.Preliminary {
			next.phase = toolResultPreliminary
		} else {
			next.phase = toolResultEmitted
		}
		event.dynamic, event.preliminary = part.Dynamic, part.Preliminary
	}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	if historical {
		state.history[id] = next
	} else {
		state.tools[id] = next
	}
	return streamPartContinue
}

type approvalRequestEvent struct {
	Type       string                    `json:"type"`
	ApprovalID string                    `json:"approvalId"`
	ToolCallID string                    `json:"toolCallId"`
	Metadata   provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

func (h *handler) processApprovalRequest(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	current, exists := state.tools[part.ToolCallID]
	historical := false
	if !exists {
		current, exists = state.history[part.ToolCallID]
		historical = exists
	}
	if !exists || !current.providerExecuted || current.phase != toolCallEmitted || current.approvalID != "" || part.ApprovalID == "" || part.ProviderExecuted || part.Dynamic != nil || part.Preliminary != nil {
		return streamPartAdapterFailure
	}
	if _, exists := state.approvalIDs[part.ApprovalID]; exists {
		return streamPartAdapterFailure
	}
	event := streamEvent{typeName: part.Type, id: part.ToolCallID, approvalID: part.ApprovalID, metadata: part.ProviderMetadata}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	if state.approvalIDs == nil {
		state.approvalIDs = make(map[string]struct{})
	}
	state.approvalIDs[part.ApprovalID] = struct{}{}
	current.approvalID = part.ApprovalID
	if historical {
		state.history[part.ToolCallID] = current
	} else {
		state.tools[part.ToolCallID] = current
	}
	return streamPartContinue
}
