package provider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

type dataContentVariant string

const (
	dataContentVariantData dataContentVariant = "data"
	dataContentVariantURL  dataContentVariant = "url"
	dataContentVariantRef  dataContentVariant = "reference"
	dataContentVariantText dataContentVariant = "text"
)

// DataContent represents file data as bytes, base64, a URL, a provider reference, or inline text.
// Exactly one of Bytes, Base64, URL, Reference, or Text should be set.
type DataContent struct {
	Bytes  []byte `json:"bytes,omitempty"`
	Base64 string `json:"base64,omitempty"`
	URL    string `json:"url,omitempty"`
	// Reference carries the upstream `{type:"reference",reference}` variant, an
	// opaque provider reference object (`{ [provider]: id }`).
	Reference json.RawMessage `json:"reference,omitempty"`
	// Text carries the upstream `{type:"text",text}` variant, an inline text
	// document.
	Text    string `json:"text,omitempty"`
	variant dataContentVariant
}

// MarshalJSON emits the upstream Vercel AI SDK LanguageModelV4 tagged file-data
// union so a stock upstream client can consume file content:
//
//   - Bytes / Base64 -> `{"type":"data","data":<base64>}`
//   - URL            -> `{"type":"url","url":<url>}`
//   - Reference      -> `{"type":"reference","reference":<obj>}`
//   - Text           -> `{"type":"text","text":<text>}`
//
// This supersedes the legacy Go `{"bytes"|"base64"|"url":...}` JSON form.
// Decoding remains tolerant of both shapes (see [DataContent.UnmarshalJSON]).
// Base64DataContent constructs inline base64 file data, including an empty payload.
func Base64DataContent(data string) DataContent {
	if data == "" {
		return DataContent{Bytes: []byte{}}
	}
	return DataContent{Base64: data}
}

// IsData reports whether d represents inline bytes or base64 data.
func (d DataContent) IsData() bool {
	return d.Bytes != nil || d.Base64 != ""
}

// IsURL reports whether d represents a file URL.
func (d DataContent) IsURL() bool {
	return d.variant == dataContentVariantURL || d.URL != ""
}

func (d DataContent) MarshalJSON() ([]byte, error) {
	switch {
	case d.IsData():
		data := d.Base64
		if d.Bytes != nil {
			data = base64.StdEncoding.EncodeToString(d.Bytes)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}{Type: "data", Data: data})
	case d.IsURL():
		return json.Marshal(struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}{Type: "url", URL: d.URL})
	case d.variant == dataContentVariantRef || len(d.Reference) > 0:
		return json.Marshal(struct {
			Type      string          `json:"type"`
			Reference json.RawMessage `json:"reference"`
		}{Type: "reference", Reference: d.Reference})
	case d.variant == dataContentVariantText || d.Text != "":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: d.Text})
	default:
		return []byte(`{}`), nil
	}
}

// UnmarshalJSON decodes a [DataContent] from the upstream Vercel AI SDK
// LanguageModelV4 tagged file-data union (`{"type":"data","data":<base64>}`,
// `{"type":"url","url":<url>}`, `{"type":"reference","reference":<obj>}`,
// `{"type":"text","text":<text>}`) and additionally tolerates the legacy Go
// JSON form (`{"bytes":...}` / `{"base64":...}` / `{"url":...}`), mapping
// either onto the canonical fields. Decoding fails closed: an unknown tagged
// `type` (not one of data, url, reference, text) returns an error rather than
// silently decoding to an empty DataContent.
func (d *DataContent) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if rawType, ok := fields["type"]; ok {
		var variant dataContentVariant
		if err := json.Unmarshal(rawType, &variant); err != nil {
			return fmt.Errorf("provider: decoding file-data variant: %w", err)
		}
		*d = DataContent{}
		switch variant {
		case dataContentVariantData:
			raw, ok := fields["data"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data variant data is required")
			}
			if err := json.Unmarshal(raw, &d.Base64); err != nil {
				return fmt.Errorf("provider: decoding file-data data: %w", err)
			}
			if d.Base64 == "" {
				d.Bytes = []byte{}
			}
		case dataContentVariantURL:
			raw, ok := fields["url"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data variant url is required")
			}
			if err := json.Unmarshal(raw, &d.URL); err != nil {
				return fmt.Errorf("provider: decoding file-data url: %w", err)
			}
			if d.URL == "" {
				d.variant = variant
			}
		case dataContentVariantRef:
			raw, ok := fields["reference"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data variant reference is required")
			}
			var reference map[string]string
			if err := json.Unmarshal(raw, &reference); err != nil {
				return fmt.Errorf("provider: decoding file-data reference: %w", err)
			}
			d.Reference = append(json.RawMessage(nil), raw...)
		case dataContentVariantText:
			raw, ok := fields["text"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data variant text is required")
			}
			if err := json.Unmarshal(raw, &d.Text); err != nil {
				return fmt.Errorf("provider: decoding file-data text: %w", err)
			}
			if d.Text == "" {
				d.variant = variant
			}
		default:
			return fmt.Errorf("provider: unsupported file-data variant %q (supported: data, url, reference, text)", variant)
		}
		return nil
	}
	type alias DataContent
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*d = DataContent(a)
	return nil
}

// Validate returns an error if DataContent has no data or multiple data sources set.
func (d DataContent) Validate() error {
	n := 0
	if d.Bytes != nil {
		n++
	}
	if d.Base64 != "" {
		n++
	}
	if d.URL != "" {
		n++
	}
	if len(d.Reference) > 0 {
		n++
	}
	if d.Text != "" {
		n++
	}
	if d.variant != "" {
		n++
	}
	if n == 0 {
		return errors.New("provider: DataContent has no data set")
	}
	if n > 1 {
		return errors.New("provider: DataContent has multiple data sources set (exactly one of Bytes, Base64, URL, Reference, Text should be set)")
	}
	return nil
}

// ContentPartType identifies the variant of a [ContentPart] in a [Message].
//
// The type is a typed string so the JSON form is human-readable and the
// in-process discriminator is type-safe. Constants enumerate every defined
// variant; producers MUST set [ContentPart.Type] to one of these values.
type ContentPartType string

const (
	// ContentPartTypeText carries plain text in any role.
	ContentPartTypeText ContentPartType = "text"
	// ContentPartTypeFile carries a file (image, document, etc.) in user or assistant content.
	ContentPartTypeFile ContentPartType = "file"
	// ContentPartTypeReasoning carries assistant model reasoning text.
	ContentPartTypeReasoning ContentPartType = "reasoning"
	// ContentPartTypeReasoningFile carries an assistant reasoning artifact (e.g. an image).
	ContentPartTypeReasoningFile ContentPartType = "reasoning-file"
	// ContentPartTypeSource carries a reference source used by generated content.
	ContentPartTypeSource ContentPartType = "source"
	// ContentPartTypeToolCall carries an assistant-issued tool call.
	ContentPartTypeToolCall ContentPartType = "tool-call"
	// ContentPartTypeToolResult carries the result of a tool call. Valid in tool messages
	// and assistant messages (for provider-executed tool results in multi-turn).
	ContentPartTypeToolResult ContentPartType = "tool-result"
	// ContentPartTypeCustom carries provider-specific custom assistant content.
	// The Kind field follows the convention "{provider}.{type}".
	ContentPartTypeCustom ContentPartType = "custom"
	// ContentPartTypeToolApprovalRequest carries an assistant-side request for
	// user approval before executing a tool call.
	ContentPartTypeToolApprovalRequest ContentPartType = "tool-approval-request"
	// ContentPartTypeToolApprovalResponse carries a user's approval/denial of a
	// tool call, only valid in tool messages.
	ContentPartTypeToolApprovalResponse ContentPartType = "tool-approval-response"
)

// ContentPart is a single piece of content inside a [Message]. It is a flat
// discriminated struct: the populated subset of fields depends on Type.
//
// This shape mirrors the LanguageModelV4 prompt content union from upstream
// (Vercel AI SDK), where TypeScript discriminated unions serialize directly
// to JSON. In Go we model the union as one struct with a typed Type field
// plus the union of all variant fields, matching how [StreamPart] is modeled.
//
// Producer-side validation (e.g. "user messages MUST NOT contain tool-call
// parts") lives at orchestration boundaries; the wire layer trusts Type.
//
// Populated fields by Type:
//   - text: Text, ProviderOptions
//   - file: Data, MediaType, Filename, ProviderOptions
//   - reasoning: Text, ProviderOptions
//   - reasoning-file: Data, MediaType, ProviderOptions
//   - source: SourceType, ID, URL, Title, MediaType, Filename, ProviderOptions
//   - tool-call: ToolCallID, ToolName, Input, ProviderExecuted, ProviderOptions
//   - tool-result: ToolCallID, ToolName, Output, ProviderOptions
//   - custom: Kind, ProviderOptions
//   - tool-approval-request: ApprovalID, ToolCallID, IsAutomatic, ProviderOptions
//   - tool-approval-response: ApprovalID, Approved, Reason, ProviderExecuted, ProviderOptions;
//     ToolCallID and ToolName MAY also be set to associate the approval with the
//     pending tool call (see field docs)
type ContentPart struct {
	Type ContentPartType `json:"type"`

	// Text is populated for ContentPartTypeText and ContentPartTypeReasoning.
	Text string `json:"text,omitempty"`

	// Data is populated for ContentPartTypeFile and ContentPartTypeReasoningFile.
	Data *DataContent `json:"data,omitempty"`

	// Filename is optional for ContentPartTypeFile.
	Filename string `json:"filename,omitempty"`

	// MediaType is required for ContentPartTypeFile and ContentPartTypeReasoningFile.
	MediaType string `json:"mediaType,omitempty"`

	// Kind is populated for ContentPartTypeCustom and follows "{provider}.{type}".
	Kind string `json:"kind,omitempty"`

	// SourceType is populated for ContentPartTypeSource.
	SourceType SourceType `json:"sourceType,omitempty"`

	// ID is populated for ContentPartTypeSource.
	ID string `json:"id,omitempty"`

	// URL is populated for ContentPartTypeSource when SourceType is SourceTypeURL.
	URL string `json:"url,omitempty"`

	// Title is populated for ContentPartTypeSource when provided by the model.
	Title string `json:"title,omitempty"`

	// ToolCallID is populated for ContentPartTypeToolCall and ContentPartTypeToolResult.
	// It MAY also be set on ContentPartTypeToolApprovalResponse to identify the
	// tool call the approval refers to; downstream helpers in the orchestration
	// layer use it when synthesizing an execution-denied tool-result for a
	// denied approval.
	ToolCallID string `json:"toolCallId,omitempty"`

	// ToolName is populated for ContentPartTypeToolCall and ContentPartTypeToolResult.
	// It MAY also be set on ContentPartTypeToolApprovalResponse alongside ToolCallID
	// (see ToolCallID).
	ToolName string `json:"toolName,omitempty"`

	// Input is the JSON-encoded tool-call argument blob, populated for ContentPartTypeToolCall.
	Input json.RawMessage `json:"input,omitempty"`

	// Output is the structured tool result, populated for ContentPartTypeToolResult.
	Output *ToolResultOutput `json:"output,omitempty"`

	// ProviderExecuted is set on ContentPartTypeToolCall when the call was
	// executed by the provider rather than the consumer.
	ProviderExecuted bool `json:"providerExecuted,omitempty"`

	// ApprovalID is populated for tool approval request/response parts.
	ApprovalID string `json:"approvalId,omitempty"`

	// Signature is populated for tool approval requests when the provider signs
	// approval payloads for client round-tripping.
	Signature string `json:"signature,omitempty"`

	// IsAutomatic is populated for ContentPartTypeToolApprovalRequest when the
	// request records an automatic approval or denial.
	IsAutomatic bool `json:"isAutomatic,omitempty"`

	// Approved is populated for ContentPartTypeToolApprovalResponse.
	Approved *bool `json:"approved,omitempty"`

	// Reason is populated for ContentPartTypeToolApprovalResponse to explain a denial.
	Reason string `json:"reason,omitempty"`

	// ProviderOptions carries provider-specific options keyed by provider name.
	ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}

// TextPart constructs a [ContentPart] of type [ContentPartTypeText].
func TextPart(text string) ContentPart {
	return ContentPart{Type: ContentPartTypeText, Text: text}
}

// FilePart constructs a [ContentPart] of type [ContentPartTypeFile]. The
// caller MUST set exactly one of [DataContent.Bytes], [DataContent.Base64],
// [DataContent.URL], [DataContent.Reference], or [DataContent.Text] on data;
// see [DataContent.Validate].
func FilePart(mediaType string, data DataContent) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, MediaType: mediaType, Data: &data}
}

// ReasoningPart constructs a [ContentPart] of type [ContentPartTypeReasoning]
// carrying assistant model reasoning text.
func ReasoningPart(text string) ContentPart {
	return ContentPart{Type: ContentPartTypeReasoning, Text: text}
}

// ReasoningFilePart constructs a [ContentPart] of type
// [ContentPartTypeReasoningFile] carrying an assistant reasoning artifact
// (e.g. an image of the model's reasoning trace).
func ReasoningFilePart(mediaType string, data DataContent) ContentPart {
	return ContentPart{Type: ContentPartTypeReasoningFile, MediaType: mediaType, Data: &data}
}

// SourcePart constructs a [ContentPart] of type [ContentPartTypeSource].
func SourcePart(source SourceInfo) ContentPart {
	return ContentPart{
		Type:            ContentPartTypeSource,
		SourceType:      source.SourceType,
		ID:              source.ID,
		URL:             source.URL,
		Title:           source.Title,
		MediaType:       source.MediaType,
		Filename:        source.Filename,
		ProviderOptions: sourceProviderOptions(source.ProviderMetadata),
	}
}

func sourceProviderOptions(meta ProviderMetadata) ProviderOptions {
	if len(meta) == 0 {
		return nil
	}
	opts := make(ProviderOptions, len(meta))
	for k, v := range meta {
		opts[k] = RawProviderOption{Key: k, Raw: v}
	}
	return opts
}

// ToolCallPart constructs a [ContentPart] of type [ContentPartTypeToolCall].
// input is the JSON-encoded argument blob for the tool.
func ToolCallPart(toolCallID, toolName string, input json.RawMessage) ContentPart {
	return ContentPart{
		Type:       ContentPartTypeToolCall,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Input:      input,
	}
}

// ToolResultPart constructs a [ContentPart] of type
// [ContentPartTypeToolResult] carrying the result of a tool execution.
func ToolResultPart(toolCallID, toolName string, output *ToolResultOutput) ContentPart {
	return ContentPart{
		Type:       ContentPartTypeToolResult,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Output:     output,
	}
}

// CustomPart constructs a [ContentPart] of type [ContentPartTypeCustom] for
// provider-specific assistant content. Kind follows the convention
// "{provider}.{type}" (e.g. "anthropic.cache-control").
func CustomPart(kind string) ContentPart {
	return ContentPart{Type: ContentPartTypeCustom, Kind: kind}
}

// ToolApprovalRequestPart constructs a [ContentPart] of type
// [ContentPartTypeToolApprovalRequest] carrying an assistant-side request for
// user approval of a tool call.
func ToolApprovalRequestPart(approvalID, toolCallID string, isAutomatic bool) ContentPart {
	return ContentPart{
		Type:        ContentPartTypeToolApprovalRequest,
		ApprovalID:  approvalID,
		ToolCallID:  toolCallID,
		IsAutomatic: isAutomatic,
	}
}

// ToolApprovalResponsePart constructs a [ContentPart] of type
// [ContentPartTypeToolApprovalResponse] carrying a user's approval or denial
// of a tool call.
func ToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart {
	return ContentPart{
		Type:       ContentPartTypeToolApprovalResponse,
		ApprovalID: approvalID,
		Approved:   &approved,
		Reason:     reason,
	}
}

// ProviderExecutedToolApprovalResponsePart constructs a provider-executed
// [ContentPartTypeToolApprovalResponse].
func ProviderExecutedToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart {
	part := ToolApprovalResponsePart(approvalID, approved, reason)
	part.ProviderExecuted = true
	return part
}
