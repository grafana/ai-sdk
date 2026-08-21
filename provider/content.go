package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// DataContentType identifies the selected [DataContent] arm.
type DataContentType string

const (
	DataContentTypeData      DataContentType = "data"
	DataContentTypeURL       DataContentType = "url"
	DataContentTypeReference DataContentType = "reference"
	DataContentTypeText      DataContentType = "text"
)

// DataContent represents file data as bytes, base64, a URL, a provider reference, or inline text.
type DataContent struct {
	Bytes     []byte          `json:"bytes,omitempty"`
	Base64    string          `json:"base64,omitempty"`
	URL       string          `json:"url,omitempty"`
	Reference json.RawMessage `json:"reference,omitempty"`
	Text      string          `json:"text,omitempty"`
	variant   DataContentType
}

// BytesDataContent constructs inline byte data and copies the input.
func BytesDataContent(data []byte) DataContent {
	copied := make([]byte, len(data))
	copy(copied, data)
	return DataContent{Bytes: copied}
}

// Base64DataContent constructs inline base64 data, including an empty payload.
func Base64DataContent(data string) DataContent {
	if data == "" {
		return BytesDataContent(nil)
	}
	return DataContent{Base64: data}
}

// URLDataContent constructs URL data, including an empty URL.
func URLDataContent(url string) DataContent {
	content := DataContent{URL: url}
	if url == "" {
		content.variant = DataContentTypeURL
	}
	return content
}

// ReferenceDataContent constructs provider-reference data and copies the input.
func ReferenceDataContent(reference json.RawMessage) DataContent {
	return DataContent{Reference: append(json.RawMessage(nil), reference...)}
}

// TextDataContent constructs inline text data, including empty text.
func TextDataContent(text string) DataContent {
	content := DataContent{Text: text}
	if text == "" {
		content.variant = DataContentTypeText
	}
	return content
}

// DataType returns the selected or inferred data arm and whether it is unique.
// On conflicting arms, it returns the selected or first inferred candidate with
// false; callers must not treat that candidate as valid.
func (d DataContent) DataType() (DataContentType, bool) {
	selected := d.variant
	valid := true
	set := func(dataType DataContentType, present bool) {
		if !present {
			return
		}
		if selected == "" {
			selected = dataType
		}
		if selected != dataType {
			valid = false
		}
	}
	set(DataContentTypeData, d.Bytes != nil || d.Base64 != "")
	set(DataContentTypeURL, d.URL != "")
	set(DataContentTypeReference, len(d.Reference) > 0)
	set(DataContentTypeText, d.Text != "")
	return selected, valid && selected != ""
}

// IsData reports whether d selects inline bytes or base64 data.
func (d DataContent) IsData() bool {
	dataType, ok := d.DataType()
	return ok && dataType == DataContentTypeData
}

// IsURL reports whether d selects a file URL.
func (d DataContent) IsURL() bool {
	dataType, ok := d.DataType()
	return ok && dataType == DataContentTypeURL
}

// MarshalJSON emits the compatibility tagged file-data union.
func (d DataContent) MarshalJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	dataType, _ := d.DataType()
	switch dataType {
	case DataContentTypeData:
		data := d.Base64
		if d.Bytes != nil {
			data = base64.StdEncoding.EncodeToString(d.Bytes)
		}
		return json.Marshal(struct {
			Type DataContentType `json:"type"`
			Data string          `json:"data"`
		}{Type: dataType, Data: data})
	case DataContentTypeURL:
		return json.Marshal(struct {
			Type DataContentType `json:"type"`
			URL  string          `json:"url"`
		}{Type: dataType, URL: d.URL})
	case DataContentTypeReference:
		return json.Marshal(struct {
			Type      DataContentType `json:"type"`
			Reference json.RawMessage `json:"reference"`
		}{Type: dataType, Reference: d.Reference})
	case DataContentTypeText:
		return json.Marshal(struct {
			Type DataContentType `json:"type"`
			Text string          `json:"text"`
		}{Type: dataType, Text: d.Text})
	default:
		return nil, errors.New("provider: unsupported DataContent type")
	}
}

// UnmarshalJSON accepts the compatibility tagged union and legacy flat data fields.
func (d *DataContent) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if rawType, ok := fields["type"]; ok {
		var dataType DataContentType
		if err := json.Unmarshal(rawType, &dataType); err != nil {
			return fmt.Errorf("provider: decoding file-data type: %w", err)
		}
		var activeField string
		switch dataType {
		case DataContentTypeData:
			activeField = "data"
		case DataContentTypeURL:
			activeField = "url"
		case DataContentTypeReference:
			activeField = "reference"
		case DataContentTypeText:
			activeField = "text"
		default:
			return fmt.Errorf("provider: unsupported file-data type %q", dataType)
		}
		for _, field := range []string{"data", "bytes", "base64", "url", "reference", "text"} {
			if field != activeField {
				if _, present := fields[field]; present {
					return fmt.Errorf("provider: file-data type %q has inactive field %q", dataType, field)
				}
			}
		}
		var decoded DataContent
		switch dataType {
		case DataContentTypeData:
			raw, ok := fields["data"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data type data is required")
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("provider: decoding file-data data: %w", err)
			}
			decoded = Base64DataContent(value)
		case DataContentTypeURL:
			raw, ok := fields["url"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data type url is required")
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("provider: decoding file-data url: %w", err)
			}
			decoded = URLDataContent(value)
		case DataContentTypeReference:
			raw, ok := fields["reference"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data type reference is required")
			}
			decoded = ReferenceDataContent(raw)
		case DataContentTypeText:
			raw, ok := fields["text"]
			if !ok || string(raw) == "null" {
				return errors.New("provider: file-data type text is required")
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("provider: decoding file-data text: %w", err)
			}
			decoded = TextDataContent(value)
		default:
			return fmt.Errorf("provider: unsupported file-data type %q", dataType)
		}
		if err := decoded.Validate(); err != nil {
			return err
		}
		*d = decoded
		return nil
	}
	var decoded struct {
		Bytes     []byte          `json:"bytes"`
		Base64    string          `json:"base64"`
		URL       string          `json:"url"`
		Reference json.RawMessage `json:"reference"`
		Text      string          `json:"text"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*d = DataContent{Bytes: decoded.Bytes, Base64: decoded.Base64, URL: decoded.URL, Reference: decoded.Reference, Text: decoded.Text}
	return d.Validate()
}

// Validate verifies exactly one valid data arm is selected.
func (d DataContent) Validate() error {
	dataType, ok := d.DataType()
	if !ok {
		return errors.New("provider: DataContent must select exactly one data source")
	}
	if dataType == DataContentTypeData && d.Bytes != nil && d.Base64 != "" {
		return errors.New("provider: DataContent data cannot contain both bytes and base64")
	}
	if dataType == DataContentTypeReference {
		var values map[string]json.RawMessage
		if len(d.Reference) == 0 || string(d.Reference) == "null" || json.Unmarshal(d.Reference, &values) != nil || values == nil {
			return errors.New("provider: DataContent reference must be a non-null JSON object with string values")
		}
		for _, raw := range values {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return errors.New("provider: DataContent reference values must be strings")
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return errors.New("provider: DataContent reference values must be strings")
			}
		}
	}
	return nil
}

// ContentPartType identifies the variant of a [ContentPart] in a [Message].
//
// The type is a typed string so the wire form is human-readable and the
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
//   - file request: Data, MediaType, FilePartFilename, ProviderOptions
//   - file response: Data, MediaType, Filename, ProviderOptions
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

	// FilePartFilename is optional for request ContentPartTypeFile values.
	FilePartFilename *string `json:"-"`

	// Filename belongs to generated response files and source content.
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
	ProviderExecuted *bool `json:"providerExecuted,omitempty"`

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
	Reason *string `json:"reason,omitempty"`

	// ProviderOptions carries provider-specific options keyed by provider name.
	ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}

// MarshalJSON preserves arm-aware filename ownership for compatibility JSON.
func (p ContentPart) MarshalJSON() ([]byte, error) {
	if p.FilePartFilename != nil && p.Type != ContentPartTypeFile {
		return nil, fmt.Errorf("provider: request file filename is invalid for content type %q", p.Type)
	}
	if p.FilePartFilename != nil && p.Filename != "" {
		return nil, errors.New("provider: content part has both request and response/source filenames")
	}

	type alias ContentPart
	copy := p
	copy.Filename = ""
	base, err := json.Marshal(alias(copy))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	var filename *string
	if p.Type == ContentPartTypeFile && p.FilePartFilename != nil {
		filename = p.FilePartFilename
	} else if p.Filename != "" {
		filename = &p.Filename
	}
	if filename != nil {
		encoded, err := json.Marshal(*filename)
		if err != nil {
			return nil, err
		}
		fields["filename"] = encoded
	}
	return json.Marshal(fields)
}

// UnmarshalJSON decodes file filenames into request ownership and source
// filenames into response/source ownership.
func (p *ContentPart) UnmarshalJSON(data []byte) error {
	type alias ContentPart
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*p = ContentPart(decoded)
	p.FilePartFilename = nil
	rawFilename, present := fields["filename"]
	if !present || string(rawFilename) == "null" {
		return nil
	}
	var filename string
	if err := json.Unmarshal(rawFilename, &filename); err != nil {
		return fmt.Errorf("provider: decoding content filename: %w", err)
	}
	if p.Type == ContentPartTypeFile {
		p.Filename = ""
		p.FilePartFilename = &filename
	}
	return nil
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

// FilePartWithFilename constructs a request file with a present filename.
func FilePartWithFilename(mediaType string, data DataContent, filename string) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, MediaType: mediaType, Data: &data, FilePartFilename: &filename}
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
	part := ContentPart{
		Type:       ContentPartTypeToolApprovalResponse,
		ApprovalID: approvalID,
		Approved:   &approved,
	}
	if reason != "" {
		part.Reason = &reason
	}
	return part
}

// ProviderExecutedToolApprovalResponsePart constructs a provider-executed
// [ContentPartTypeToolApprovalResponse].
func ProviderExecutedToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart {
	part := ToolApprovalResponsePart(approvalID, approved, reason)
	providerExecuted := true
	part.ProviderExecuted = &providerExecuted
	return part
}
