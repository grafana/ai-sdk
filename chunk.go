package aisdk

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// ChunkType identifies the type of a UIMessageChunk SSE event.
type ChunkType string

const (
	ChunkStart           ChunkType = "start"
	ChunkFinish          ChunkType = "finish"
	ChunkAbort           ChunkType = "abort"
	ChunkStartStep       ChunkType = "start-step"
	ChunkFinishStep      ChunkType = "finish-step"
	ChunkMessageMetadata ChunkType = "message-metadata"

	ChunkTextStart ChunkType = "text-start"
	ChunkTextDelta ChunkType = "text-delta"
	ChunkTextEnd   ChunkType = "text-end"

	ChunkReasoningStart ChunkType = "reasoning-start"
	ChunkReasoningDelta ChunkType = "reasoning-delta"
	ChunkReasoningEnd   ChunkType = "reasoning-end"
	ChunkReasoningFile  ChunkType = "reasoning-file"

	ChunkToolInputStart       ChunkType = "tool-input-start"
	ChunkToolInputDelta       ChunkType = "tool-input-delta"
	ChunkToolInputAvailable   ChunkType = "tool-input-available"
	ChunkToolInputError       ChunkType = "tool-input-error"
	ChunkToolApprovalRequest  ChunkType = "tool-approval-request"
	ChunkToolApprovalResponse ChunkType = "tool-approval-response"
	ChunkToolOutputDenied     ChunkType = "tool-output-denied"
	ChunkToolOutputAvailable  ChunkType = "tool-output-available"
	ChunkToolOutputError      ChunkType = "tool-output-error"

	ChunkSourceURL      ChunkType = "source-url"
	ChunkSourceDocument ChunkType = "source-document"

	ChunkFile  ChunkType = "file"
	ChunkData  ChunkType = "data"
	ChunkError ChunkType = "error"
)

// UIMessageChunk is a single SSE event in the UI message stream protocol.
// Use the constructor functions (TextDeltaChunk, DataChunk, etc.) to create
// valid chunks with the correct Type set.
type UIMessageChunk struct {
	Type ChunkType `json:"type"`

	// Lifecycle fields
	MessageID       string          `json:"messageId,omitempty"`
	MessageMetadata json.RawMessage `json:"messageMetadata,omitempty"`
	FinishReason    string          `json:"finishReason,omitempty"`
	Reason          string          `json:"reason,omitempty"`

	// Content fields
	ID             string `json:"id,omitempty"`
	Delta          string `json:"delta,omitempty"`
	InputTextDelta string `json:"inputTextDelta,omitempty"`

	// Tool fields
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	ApprovalID string          `json:"approvalId,omitempty"`
	Signature  string          `json:"signature,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	ErrorText  string          `json:"errorText,omitempty"`
	// Approved is intentionally not tagged with omitempty: a denial response
	// (approved=false) MUST appear on the wire. The custom MarshalJSON for
	// ChunkToolApprovalResponse always writes this field, but exposing the
	// raw bool would otherwise silently drop denials.
	Approved         bool  `json:"approved"`
	ProviderExecuted bool  `json:"providerExecuted,omitempty"`
	Dynamic          *bool `json:"dynamic,omitempty"`
	Preliminary      bool  `json:"preliminary,omitempty"`
	IsAutomatic      bool  `json:"isAutomatic,omitempty"`

	// Source fields
	SourceID string `json:"sourceId,omitempty"`
	URL      string `json:"url,omitempty"`
	Title    string `json:"title,omitempty"`

	// File fields
	MediaType string `json:"mediaType,omitempty"`
	Filename  string `json:"filename,omitempty"`

	// Data chunk fields (type is "data-{name}")
	DataName  string          `json:"-"`
	Data      json.RawMessage `json:"data,omitempty"`
	Transient bool            `json:"transient,omitempty"`

	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
	ToolMetadata     json.RawMessage           `json:"toolMetadata,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler and rejects unknown chunk discriminators.
func (c *UIMessageChunk) UnmarshalJSON(data []byte) error {
	type chunk UIMessageChunk
	var decoded chunk
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if strings.HasPrefix(string(decoded.Type), "data-") {
		decoded.DataName = strings.TrimPrefix(string(decoded.Type), "data-")
		decoded.Type = ChunkData
	} else if !isKnownChunkType(decoded.Type) {
		return fmt.Errorf("aisdk: unsupported UI message chunk type %q", decoded.Type)
	}
	*c = UIMessageChunk(decoded)
	return nil
}

func isKnownChunkType(chunkType ChunkType) bool {
	switch chunkType {
	case ChunkStart, ChunkFinish, ChunkAbort, ChunkStartStep, ChunkFinishStep, ChunkMessageMetadata,
		ChunkTextStart, ChunkTextDelta, ChunkTextEnd,
		ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd, ChunkReasoningFile,
		ChunkToolInputStart, ChunkToolInputDelta, ChunkToolInputAvailable, ChunkToolInputError,
		ChunkToolApprovalRequest, ChunkToolApprovalResponse, ChunkToolOutputDenied,
		ChunkToolOutputAvailable, ChunkToolOutputError, ChunkSourceURL, ChunkSourceDocument,
		ChunkFile, ChunkData, ChunkError:
		return true
	default:
		return false
	}
}

// MarshalJSON implements json.Marshaler.
// Each chunk type serializes only the fields defined by the UIMessageChunk schema.
func (c UIMessageChunk) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, 6)
	m["type"] = c.Type

	switch c.Type {
	case ChunkStart:
		setOpt(m, "messageId", c.MessageID)
		setOptRaw(m, "messageMetadata", c.MessageMetadata)

	case ChunkFinish:
		setOpt(m, "finishReason", c.FinishReason)
		setOptRaw(m, "messageMetadata", c.MessageMetadata)

	case ChunkAbort:
		setOpt(m, "reason", c.Reason)

	case ChunkStartStep, ChunkFinishStep:
		// no additional fields

	case ChunkMessageMetadata:
		m["messageMetadata"] = c.MessageMetadata

	case ChunkTextStart:
		m["id"] = c.ID
		setOptMeta(m, c.ProviderMetadata)

	case ChunkTextDelta:
		m["id"] = c.ID
		m["delta"] = c.Delta
		setOptMeta(m, c.ProviderMetadata)

	case ChunkTextEnd:
		m["id"] = c.ID
		setOptMeta(m, c.ProviderMetadata)

	case ChunkReasoningStart:
		m["id"] = c.ID
		setOptMeta(m, c.ProviderMetadata)

	case ChunkReasoningDelta:
		m["id"] = c.ID
		m["delta"] = c.Delta
		setOptMeta(m, c.ProviderMetadata)

	case ChunkReasoningEnd:
		m["id"] = c.ID
		setOptMeta(m, c.ProviderMetadata)

	case ChunkToolInputStart:
		m["toolCallId"] = c.ToolCallID
		m["toolName"] = c.ToolName
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptBoolP(m, c.Dynamic)
		setOpt(m, "title", c.Title)
		setOptMeta(m, c.ProviderMetadata)
		setOptRaw(m, "toolMetadata", c.ToolMetadata)

	case ChunkToolInputDelta:
		m["toolCallId"] = c.ToolCallID
		m["inputTextDelta"] = c.InputTextDelta

	case ChunkToolInputAvailable:
		m["toolCallId"] = c.ToolCallID
		m["toolName"] = c.ToolName
		m["input"] = c.Input
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptBoolP(m, c.Dynamic)
		setOpt(m, "title", c.Title)
		setOptMeta(m, c.ProviderMetadata)
		setOptRaw(m, "toolMetadata", c.ToolMetadata)

	case ChunkToolInputError:
		m["toolCallId"] = c.ToolCallID
		m["toolName"] = c.ToolName
		m["input"] = c.Input
		m["errorText"] = c.ErrorText
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptBoolP(m, c.Dynamic)
		setOpt(m, "title", c.Title)
		setOptMeta(m, c.ProviderMetadata)
		setOptRaw(m, "toolMetadata", c.ToolMetadata)

	case ChunkToolApprovalRequest:
		m["approvalId"] = c.ApprovalID
		m["toolCallId"] = c.ToolCallID
		setOptBool(m, "isAutomatic", c.IsAutomatic)
		setOpt(m, "signature", c.Signature)

	case ChunkToolApprovalResponse:
		m["approvalId"] = c.ApprovalID
		m["approved"] = c.Approved
		setOpt(m, "reason", c.Reason)
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptMeta(m, c.ProviderMetadata)

	case ChunkToolOutputDenied:
		m["toolCallId"] = c.ToolCallID

	case ChunkToolOutputAvailable:
		m["toolCallId"] = c.ToolCallID
		m["output"] = c.Output
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptBoolP(m, c.Dynamic)
		setOptBool(m, "preliminary", c.Preliminary)
		setOptMeta(m, c.ProviderMetadata)

	case ChunkToolOutputError:
		m["toolCallId"] = c.ToolCallID
		m["errorText"] = c.ErrorText
		setOptBool(m, "providerExecuted", c.ProviderExecuted)
		setOptBoolP(m, c.Dynamic)
		setOptMeta(m, c.ProviderMetadata)

	case ChunkSourceURL:
		m["sourceId"] = c.SourceID
		m["url"] = c.URL
		setOpt(m, "title", c.Title)
		setOptMeta(m, c.ProviderMetadata)

	case ChunkSourceDocument:
		m["sourceId"] = c.SourceID
		m["mediaType"] = c.MediaType
		m["title"] = c.Title
		setOpt(m, "filename", c.Filename)
		setOptMeta(m, c.ProviderMetadata)

	case ChunkFile, ChunkReasoningFile:
		m["url"] = c.URL
		m["mediaType"] = c.MediaType
		setOptMeta(m, c.ProviderMetadata)

	case ChunkError:
		m["errorText"] = c.ErrorText

	case ChunkData:
		if c.DataName != "" {
			m["type"] = "data-" + c.DataName
		}
		setOpt(m, "id", c.ID)
		if c.Data != nil {
			m["data"] = json.RawMessage(c.Data)
		}
		setOptBool(m, "transient", c.Transient)
	}

	return json.Marshal(m)
}

func setOpt(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func setOptBool(m map[string]any, key string, val bool) {
	if val {
		m[key] = val
	}
}

func setOptBoolP(m map[string]any, val *bool) {
	if val != nil {
		m["dynamic"] = *val
	}
}

func setOptRaw(m map[string]any, key string, val json.RawMessage) {
	if len(val) > 0 {
		m[key] = val
	}
}

func setOptMeta(m map[string]any, meta provider.ProviderMetadata) {
	if len(meta) > 0 {
		m["providerMetadata"] = meta
	}
}

// Chunk constructors

// TextStartChunk creates a text-start chunk.
func TextStartChunk(id string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkTextStart, ID: id}
}

// TextDeltaChunk creates a text-delta chunk.
func TextDeltaChunk(id, delta string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkTextDelta, ID: id, Delta: delta}
}

// TextEndChunk creates a text-end chunk.
func TextEndChunk(id string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkTextEnd, ID: id}
}

// ReasoningStartChunk creates a reasoning-start chunk.
func ReasoningStartChunk(id string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkReasoningStart, ID: id}
}

// ReasoningDeltaChunk creates a reasoning-delta chunk.
func ReasoningDeltaChunk(id, delta string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkReasoningDelta, ID: id, Delta: delta}
}

// ReasoningEndChunk creates a reasoning-end chunk.
func ReasoningEndChunk(id string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkReasoningEnd, ID: id}
}

// ToolInputStartChunk creates a tool-input-start chunk.
func ToolInputStartChunk(toolCallID, toolName string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: toolCallID, ToolName: toolName}
}

// ToolInputDeltaChunk creates a tool-input-delta chunk.
func ToolInputDeltaChunk(toolCallID, inputTextDelta string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: toolCallID, InputTextDelta: inputTextDelta}
}

// ToolInputAvailableChunk creates a tool-input-available chunk.
func ToolInputAvailableChunk(toolCallID, toolName string, input json.RawMessage) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: toolCallID, ToolName: toolName, Input: input}
}

// ToolInputErrorChunk creates a tool-input-error chunk.
func ToolInputErrorChunk(toolCallID, toolName string, input json.RawMessage, errorText string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolInputError, ToolCallID: toolCallID, ToolName: toolName, Input: input, ErrorText: errorText}
}

// ToolOutputAvailableChunk creates a tool-output-available chunk.
func ToolOutputAvailableChunk(toolCallID string, output json.RawMessage) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: toolCallID, Output: output}
}

// ToolOutputErrorChunk creates a tool-output-error chunk.
func ToolOutputErrorChunk(toolCallID, errorText string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkToolOutputError, ToolCallID: toolCallID, ErrorText: errorText}
}

// SourceURLChunk creates a source-url chunk.
func SourceURLChunk(sourceID, url string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkSourceURL, SourceID: sourceID, URL: url}
}

// SourceDocumentChunk creates a source-document chunk.
func SourceDocumentChunk(sourceID, mediaType, title string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkSourceDocument, SourceID: sourceID, MediaType: mediaType, Title: title}
}

// FileChunk creates a file chunk.
func FileChunk(url, mediaType string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkFile, URL: url, MediaType: mediaType}
}

// ReasoningFileChunk creates a reasoning-file chunk.
func ReasoningFileChunk(url, mediaType string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkReasoningFile, URL: url, MediaType: mediaType}
}

// ErrorChunk creates an error chunk.
func ErrorChunk(errorText string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkError, ErrorText: errorText}
}

// DataChunk creates a data chunk. Set transient to true for ephemeral data
// that should not be persisted in message history.
func DataChunk(name string, data json.RawMessage, transient bool) UIMessageChunk {
	return UIMessageChunk{Type: ChunkData, DataName: name, Data: data, Transient: transient}
}

// StartChunk creates a start chunk.
func StartChunk(messageID string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkStart, MessageID: messageID}
}

// FinishChunk creates a finish chunk.
func FinishChunk(finishReason string) UIMessageChunk {
	return UIMessageChunk{Type: ChunkFinish, FinishReason: finishReason}
}
