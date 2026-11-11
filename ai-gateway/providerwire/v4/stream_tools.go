package v4

import (
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

func (h *handler) processToolStreamPart(w http.ResponseWriter, part provider.StreamPart) streamPartResult {
	id := part.ID
	if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
		id = part.ToolCallID
	}
	if !utf8.ValidString(id) {
		return streamPartAdapterFailure
	}
	event := streamEvent{typeName: part.Type, id: id, toolName: part.ToolName, inputEncoding: part.InputEncoding, delta: part.Delta, input: part.Input, result: part.Result, isError: part.IsError, metadata: part.ProviderMetadata}
	switch part.Type {
	case provider.PartToolInputStart, provider.PartToolCall:
		event.providerExecuted, event.dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolResult:
		event.dynamic, event.preliminary = part.Dynamic, part.Preliminary
	}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	return streamPartContinue
}

type approvalRequestEvent struct {
	Type       string                    `json:"type"`
	ApprovalID string                    `json:"approvalId"`
	ToolCallID string                    `json:"toolCallId"`
	Metadata   provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

func (h *handler) processApprovalRequest(w http.ResponseWriter, part provider.StreamPart) streamPartResult {
	event := streamEvent{typeName: part.Type, id: part.ToolCallID, approvalID: part.ApprovalID, metadata: part.ProviderMetadata}
	switch h.emitStreamEvent(w, event) {
	case streamWriteEncodingFailure:
		return streamPartAdapterFailure
	case streamWriteWriterFailure:
		return streamPartWriterFailure
	}
	return streamPartContinue
}
