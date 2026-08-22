package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"time"
)

// LanguageModel is the provider interface that all LLM providers implement.
// It mirrors the AI SDK's LanguageModelV4 specification.
type LanguageModel interface {
	// SpecificationVersion returns the provider spec version (e.g., "v4").
	SpecificationVersion() string
	// Provider returns the provider identifier (e.g., "openai", "anthropic").
	Provider() string
	// ModelID returns the model identifier (e.g., "gpt-4o", "claude-3-opus").
	ModelID() string
	// SupportedURLs returns URL patterns the model can handle, keyed by media type.
	SupportedURLs() map[string][]*regexp.Regexp
	// DoStream performs a streaming model call.
	DoStream(ctx context.Context, params CallOptions) (*StreamResult, error)
	// DoGenerate performs a non-streaming model call.
	DoGenerate(ctx context.Context, params CallOptions) (*GenerateResult, error)
}

// CallOptions configures a model call. Passed to DoStream and DoGenerate.
//
// Every field carries a JSON tag so [CallOptions] round-trips losslessly
// through encoding/json. The JSON representation mirrors
// LanguageModelV4CallOptions from upstream (Vercel AI SDK).
type CallOptions struct {
	Prompt           []Message         `json:"prompt,omitempty"`
	Tools            []Tool            `json:"tools,omitempty"`
	ToolChoice       *ToolChoice       `json:"toolChoice,omitempty"`
	MaxOutputTokens  *int              `json:"maxOutputTokens,omitempty"`
	Temperature      *float64          `json:"temperature,omitempty"`
	TopP             *float64          `json:"topP,omitempty"`
	TopK             *int              `json:"topK,omitempty"`
	PresencePenalty  *float64          `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64          `json:"frequencyPenalty,omitempty"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	ResponseFormat   *ResponseFormat   `json:"responseFormat,omitempty"`
	Seed             *int              `json:"seed,omitempty"`
	Reasoning        *ReasoningEffort  `json:"reasoning,omitempty"`
	IncludeRawChunks bool              `json:"includeRawChunks,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	ProviderOptions  ProviderOptions   `json:"providerOptions,omitempty"`
}

// ResponseFormatType specifies the desired output format.
type ResponseFormatType string

const (
	ResponseFormatText ResponseFormatType = "text"
	ResponseFormatJSON ResponseFormatType = "json"
)

// ResponseFormat specifies the desired output format (text or JSON).
type ResponseFormat struct {
	Type        ResponseFormatType `json:"type"`
	Schema      json.RawMessage    `json:"schema,omitempty"`
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
}

// ToolType identifies the kind of provider-level tool.
type ToolType string

const (
	ToolTypeFunction ToolType = "function"
	ToolTypeProvider ToolType = "provider"
)

// Tool is the provider-level tool representation. It is a flat discriminated
// struct: [Tool.Type] selects the variant ([ToolTypeFunction] or
// [ToolTypeProvider]). The function-tool fields (Description, InputSchema,
// InputExamples, Strict) and provider-tool fields (ID, Args) live on the
// same struct; populated subset depends on Type.
//
// This shape mirrors how LanguageModelV4FunctionTool / V4ProviderTool serialize
// directly to JSON in upstream (Vercel AI SDK). In Go we keep them in one
// struct so the runtime type is the wire payload (no DTO layer needed).
type Tool struct {
	Type ToolType `json:"type"`
	Name string   `json:"name"`

	// Function-tool fields.
	Description   string          `json:"description,omitempty"`
	InputSchema   json.RawMessage `json:"inputSchema,omitempty"`
	InputExamples []InputExample  `json:"inputExamples,omitempty"`
	Strict        *bool           `json:"strict,omitempty"`

	// Provider-tool fields.
	ID   string                     `json:"id,omitempty"`
	Args map[string]json.RawMessage `json:"args,omitempty"`

	// ProviderOptions carries provider-specific options keyed by provider name.
	// Both function tools and provider tools may carry options; producers MAY
	// leave this nil for provider tools.
	ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}

// InputExample wraps a tool input example.
type InputExample struct {
	Input json.RawMessage `json:"input"`
}

// GenerateContentType identifies the kind of content in a GenerateResult.
type GenerateContentType string

const (
	ContentText                GenerateContentType = "text"
	ContentReasoning           GenerateContentType = "reasoning"
	ContentToolCall            GenerateContentType = "tool-call"
	ContentToolResult          GenerateContentType = "tool-result"
	ContentSource              GenerateContentType = "source"
	ContentFile                GenerateContentType = "file"
	ContentReasoningFile       GenerateContentType = "reasoning-file"
	ContentCustom              GenerateContentType = "custom"
	ContentToolApprovalRequest GenerateContentType = "tool-approval-request"
)

// GenerateResult is the result of a non-streaming DoGenerate call.
type GenerateResult struct {
	Content          []GenerateContentPart `json:"content"`
	FinishReason     FinishReason          `json:"finishReason"`
	Usage            Usage                 `json:"usage"`
	ProviderMetadata ProviderMetadata      `json:"providerMetadata,omitempty"`
	Warnings         []Warning             `json:"warnings,omitempty"`
	Request          *RequestMetadata      `json:"request,omitempty"`
	Response         *GenerateResponse     `json:"response,omitempty"`
}

// GenerateContentPart is a piece of content in a GenerateResult.
// It is a flat union keyed by Type, mirroring LanguageModelV4Content.
type GenerateContentPart struct {
	Type             GenerateContentType `json:"type"`
	ID               string              `json:"id,omitempty"`
	Kind             string              `json:"kind,omitempty"`
	Text             string              `json:"text,omitempty"`
	ApprovalID       string              `json:"approvalId,omitempty"`
	ToolCallID       string              `json:"toolCallId,omitempty"`
	ToolName         string              `json:"toolName,omitempty"`
	Input            json.RawMessage     `json:"input,omitempty"`
	Result           json.RawMessage     `json:"result,omitempty"`
	IsError          bool                `json:"isError,omitempty"`
	Preliminary      *bool               `json:"preliminary,omitempty"`
	ProviderExecuted bool                `json:"providerExecuted,omitempty"`
	Dynamic          *bool               `json:"dynamic,omitempty"`
	SourceType       SourceType          `json:"sourceType,omitempty"`
	URL              string              `json:"url,omitempty"`
	// Title is populated for ContentSource (optional for URL sources, required
	// for document sources) and mirrors the upstream LanguageModelV4Source
	// `title` field.
	Title            string           `json:"title,omitempty"`
	Data             *DataContent     `json:"data,omitempty"`
	MediaType        string           `json:"mediaType,omitempty"`
	Filename         string           `json:"filename,omitempty"`
	ProviderMetadata ProviderMetadata `json:"providerMetadata,omitempty"`
}

// MarshalJSON emits the upstream Vercel AI SDK LanguageModelV4 content shape.
// For tool-call parts the `input` field is emitted as a stringified JSON
// string (matching upstream `LanguageModelV4ToolCall.input: string`), which is
// what `ai` core's tool-call parser dereferences. All other parts marshal via
// their struct tags.
func (p GenerateContentPart) MarshalJSON() ([]byte, error) {
	type alias GenerateContentPart
	if p.Type != ContentToolCall || len(p.Input) == 0 {
		return json.Marshal(alias(p))
	}
	// Encode the raw JSON input object as a stringified JSON string.
	inputStr, err := json.Marshal(string(p.Input))
	if err != nil {
		return nil, err
	}
	cp := p
	cp.Input = nil
	base, err := json.Marshal(alias(cp))
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(base, &m); err != nil {
		return nil, err
	}
	m["input"] = inputStr
	return json.Marshal(m)
}

// UnmarshalJSON decodes a [GenerateContentPart], tolerating a tool-call `input`
// encoded either as the canonical raw JSON object or as the upstream
// stringified JSON string.
func (p *GenerateContentPart) UnmarshalJSON(data []byte) error {
	type alias GenerateContentPart
	var raw struct {
		alias
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = GenerateContentPart(raw.alias)
	p.Input = decodeToolCallInput(raw.Input)
	return nil
}

// decodeToolCallInput normalizes a tool-call `input` field into raw JSON. When
// the value is a JSON string containing stringified JSON (the upstream shape),
// the inner JSON is unwrapped; otherwise the raw value is returned unchanged.
func decodeToolCallInput(input json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return json.RawMessage(s)
		}
	}
	return input
}

// ResponseMetadata carries metadata from the provider response.
type ResponseMetadata struct {
	ID        string    `json:"id,omitempty"`
	ModelID   string    `json:"modelId,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// GenerateResponse extends ResponseMetadata with the full response body
// and headers, available on GenerateResult.
type GenerateResponse struct {
	ResponseMetadata
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// RequestMetadata carries metadata about the provider request.
type RequestMetadata struct {
	Body json.RawMessage `json:"body,omitempty"`
}

// ResponseHeaders carries HTTP response headers from the provider.
type ResponseHeaders struct {
	Headers map[string]string `json:"headers,omitempty"`
}
