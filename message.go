package aisdk

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

// Role is an alias for provider.Role, used on UIMessage.
// UIMessages use system, user, and assistant roles.
type Role = provider.Role

// Role constants re-exported from the provider package.
const (
	RoleSystem    = provider.RoleSystem
	RoleUser      = provider.RoleUser
	RoleAssistant = provider.RoleAssistant
)

// UIMessage is the canonical message type for the AI SDK UI protocol.
// It is JSON-serializable for persistence and wire transfer.
type UIMessage struct {
	ID       string          `json:"id"`
	Role     Role            `json:"role"`
	Parts    []Part          `json:"parts"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Part is the interface for all UIMessage part types.
// Consumers type-switch on concrete types: TextPart, ReasoningPart,
// ToolInvocationPart, DynamicToolUIPart, FilePart, ReasoningFilePart,
// SourceURLPart, SourceDocumentPart, DataPart, CustomPart, StepStartPart.
type Part interface {
	PartType() string
}

// UIPartType identifies the wire type of a UIMessage part.
type UIPartType string

const (
	UIPartText           UIPartType = "text"
	UIPartReasoning      UIPartType = "reasoning"
	UIPartToolInvocation UIPartType = "tool-invocation"
	UIPartDynamicTool    UIPartType = "dynamic-tool"
	UIPartFile           UIPartType = "file"
	UIPartReasoningFile  UIPartType = "reasoning-file"
	UIPartSourceURL      UIPartType = "source-url"
	UIPartSourceDocument UIPartType = "source-document"
	UIPartData           UIPartType = "data"
	UIPartCustom         UIPartType = "custom"
	UIPartStepStart      UIPartType = "step-start"
)

// TextPart carries text content in a UIMessage.
type TextPart struct {
	Text             string                    `json:"text"`
	State            string                    `json:"state,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (TextPart) PartType() string { return string(UIPartText) }

// ReasoningPart carries model reasoning in a UIMessage.
type ReasoningPart struct {
	ID               string                    `json:"id,omitempty"`
	Text             string                    `json:"text"`
	State            string                    `json:"state,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (ReasoningPart) PartType() string { return string(UIPartReasoning) }

// ToolApproval carries approval status for a tool invocation. Mirrors the
// upstream `approval` object on `ToolUIPart` (see Vercel AI SDK
// `ui-messages.ts`), where ID identifies the approval request and Approved
// is unset while a request is pending and set true/false once the user
// responds.
type ToolApproval struct {
	ID          string `json:"id,omitempty"`
	Approved    *bool  `json:"approved,omitempty"`
	Reason      string `json:"reason,omitempty"`
	IsAutomatic bool   `json:"isAutomatic,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// ToolInvocationState identifies the lifecycle state of a tool invocation.
type ToolInvocationState string

const (
	ToolStateInputStreaming    ToolInvocationState = "input-streaming"
	ToolStateInputAvailable    ToolInvocationState = "input-available"
	ToolStateApprovalRequested ToolInvocationState = "approval-requested"
	ToolStateApprovalResponded ToolInvocationState = "approval-responded"
	ToolStateOutputAvailable   ToolInvocationState = "output-available"
	ToolStateOutputError       ToolInvocationState = "output-error"
	ToolStateOutputDenied      ToolInvocationState = "output-denied"
)

// ToolInvocationPart carries a tool invocation lifecycle in a UIMessage.
type ToolInvocationPart struct {
	ToolCallID             string                    `json:"toolCallId"`
	ToolName               string                    `json:"toolName"`
	State                  ToolInvocationState       `json:"state"`
	Input                  json.RawMessage           `json:"input,omitempty"`
	Output                 json.RawMessage           `json:"output,omitempty"`
	ErrorText              string                    `json:"errorText,omitempty"`
	ProviderExecuted       bool                      `json:"providerExecuted,omitempty"`
	Preliminary            bool                      `json:"preliminary,omitempty"`
	Approval               *ToolApproval             `json:"approval,omitempty"`
	CallProviderMetadata   provider.ProviderMetadata `json:"callProviderMetadata,omitempty"`
	ResultProviderMetadata provider.ProviderMetadata `json:"resultProviderMetadata,omitempty"`
}

// PartType implements Part.
func (p ToolInvocationPart) PartType() string { return "tool-" + p.ToolName }

// DynamicToolUIPart carries a dynamic/MCP tool invocation in a UIMessage.
// Same shape as ToolInvocationPart but with type "dynamic-tool" on the wire.
type DynamicToolUIPart struct {
	ToolCallID             string                    `json:"toolCallId"`
	ToolName               string                    `json:"toolName"`
	State                  ToolInvocationState       `json:"state"`
	Input                  json.RawMessage           `json:"input,omitempty"`
	Output                 json.RawMessage           `json:"output,omitempty"`
	ErrorText              string                    `json:"errorText,omitempty"`
	ProviderExecuted       bool                      `json:"providerExecuted,omitempty"`
	Preliminary            bool                      `json:"preliminary,omitempty"`
	Approval               *ToolApproval             `json:"approval,omitempty"`
	CallProviderMetadata   provider.ProviderMetadata `json:"callProviderMetadata,omitempty"`
	ResultProviderMetadata provider.ProviderMetadata `json:"resultProviderMetadata,omitempty"`
}

// PartType implements Part.
func (DynamicToolUIPart) PartType() string { return string(UIPartDynamicTool) }

// FilePart carries file data in a UIMessage. ProviderReference maps provider
// names to provider-specific file IDs and, when present, takes precedence over
// URL during conversion to model messages.
type FilePart struct {
	MediaType         string                    `json:"mediaType"`
	URL               string                    `json:"url"`
	Filename          string                    `json:"filename,omitempty"`
	ProviderReference map[string]string         `json:"providerReference,omitempty"`
	ProviderMetadata  provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (p FilePart) MarshalJSON() ([]byte, error) {
	type filePartAlias FilePart
	var providerReference *map[string]string
	if p.ProviderReference != nil {
		providerReference = &p.ProviderReference
	}
	return json.Marshal(struct {
		filePartAlias
		ProviderReference *map[string]string `json:"providerReference,omitempty"`
	}{
		filePartAlias:     filePartAlias(p),
		ProviderReference: providerReference,
	})
}

// PartType implements Part.
func (FilePart) PartType() string { return string(UIPartFile) }

// ReasoningFilePart carries a file generated as part of model reasoning.
type ReasoningFilePart struct {
	MediaType        string                    `json:"mediaType"`
	URL              string                    `json:"url"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (ReasoningFilePart) PartType() string { return string(UIPartReasoningFile) }

// SourceURLPart carries a URL source reference in a UIMessage.
type SourceURLPart struct {
	SourceID         string                    `json:"sourceId"`
	URL              string                    `json:"url"`
	Title            string                    `json:"title,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (SourceURLPart) PartType() string { return string(UIPartSourceURL) }

// SourceDocumentPart carries a document source reference in a UIMessage.
type SourceDocumentPart struct {
	SourceID         string                    `json:"sourceId"`
	MediaType        string                    `json:"mediaType"`
	Title            string                    `json:"title,omitempty"`
	Filename         string                    `json:"filename,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (SourceDocumentPart) PartType() string { return string(UIPartSourceDocument) }

// DataPart carries custom data in a UIMessage.
// On the wire, it serializes as type "data-{DataName}".
type DataPart struct {
	DataName string          `json:"dataName"`
	ID       string          `json:"id,omitempty"`
	Data     json.RawMessage `json:"data"`
}

// PartType implements Part.
func (p DataPart) PartType() string { return "data-" + p.DataName }

// CustomPart carries provider-specific custom content in a UIMessage.
type CustomPart struct {
	Kind             string                    `json:"kind"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// PartType implements Part.
func (CustomPart) PartType() string { return string(UIPartCustom) }

// StepStartPart marks the beginning of a new step in a UIMessage.
type StepStartPart struct{}

// PartType implements Part.
func (StepStartPart) PartType() string { return string(UIPartStepStart) }
