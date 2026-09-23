package v4

import (
	"bytes"
	"encoding/json"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type streamToolStartEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ID               string                    `json:"id"`
	ToolName         string                    `json:"toolName"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}
type streamToolDeltaEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ID               string                    `json:"id"`
	Delta            string                    `json:"delta"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}
type streamToolEndEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ID               string                    `json:"id"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}
type streamToolCallEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            string                    `json:"input"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          bool                      `json:"dynamic,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}
type streamToolResultEvent struct {
	Type             provider.StreamPartType   `json:"type"`
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Result           json.RawMessage           `json:"result"`
	IsError          bool                      `json:"isError,omitempty"`
	Dynamic          bool                      `json:"dynamic,omitempty"`
	Preliminary      bool                      `json:"preliminary,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
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
	name  string
	phase toolStreamPhase
}

func (h *handler) processToolStreamPart(w http.ResponseWriter, state *streamState, part provider.StreamPart) streamPartResult {
	id := part.ID
	if part.Type == provider.PartToolCall || part.Type == provider.PartToolResult {
		id = part.ToolCallID
	}
	if id == "" || int64(len(id)) > h.limits.StreamFrameBytes || !utf8.ValidString(id) {
		return streamPartAdapterFailure
	}
	current, exists := state.tools[id]
	_, seenInHistory := state.history[id]
	historical := false
	if !exists && part.Type == provider.PartToolResult {
		current, historical = state.history[id]
		exists = historical
	}
	next := current
	metadata, err := mapToolMetadata(part.ProviderMetadata, state.mcpNames, h.limits.StreamFrameBytes)
	if err != nil {
		return streamPartAdapterFailure
	}
	event := streamEvent{typeName: part.Type, id: id, toolName: part.ToolName, delta: part.Delta, input: part.Input, result: part.Result, isError: part.IsError, providerMetadata: metadata}
	switch part.Type {
	case provider.PartToolInputStart:
		if exists || seenInHistory || part.ToolName == "" || part.Preliminary {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolInputOpen}
		event.providerExecuted, event.dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolInputDelta, provider.PartToolInputEnd:
		if !exists || current.phase != toolInputOpen || part.ProviderExecuted || (part.Dynamic != nil && *part.Dynamic) || part.Preliminary {
			return streamPartAdapterFailure
		}
		if part.Type == provider.PartToolInputEnd {
			next.phase = toolInputClosed
		}
	case provider.PartToolCall:
		if seenInHistory || part.ToolName == "" || part.Preliminary || (exists && (current.phase != toolInputClosed || current.name != part.ToolName)) {
			return streamPartAdapterFailure
		}
		next = toolStreamState{name: part.ToolName, phase: toolCallEmitted}
		event.providerExecuted, event.dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolResult:
		if !exists || part.ToolName == "" || (current.phase != toolCallEmitted && current.phase != toolResultPreliminary) || current.name != part.ToolName {
			return streamPartAdapterFailure
		}
		if int64(len(part.Result)) > h.limits.StreamFrameBytes || !json.Valid(part.Result) || bytes.Equal(bytes.TrimSpace(part.Result), []byte("null")) {
			return streamPartAdapterFailure
		}
		if part.Preliminary {
			next.phase = toolResultPreliminary
		} else {
			next.phase = toolResultEmitted
		}
		event.dynamic, event.preliminary = part.Dynamic, part.Preliminary
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
	if historical {
		state.history[id] = next
	} else {
		state.tools[id] = next
	}
	state.textStarted = true
	return streamPartContinue
}
