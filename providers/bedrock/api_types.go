package bedrock

import "encoding/json"

// converseInput is the JSON body sent to /model/{id}/converse and
// /model/{id}/converse-stream. Mirrors the upstream
// AmazonBedrockConverseInput shape. Optional fields are omitted via
// omitempty so the wire shape matches upstream.
type converseInput struct {
	passthrough map[string]json.RawMessage

	// System is always serialized, including as an empty array, to match the
	// upstream SDK which initializes system to [] and always includes it in
	// the Converse command. It is populated as a non-nil empty slice in
	// buildRequest so it marshals to [] rather than null.
	System                            []systemContentBlock `json:"system"`
	Messages                          []converseMessage    `json:"messages"`
	ToolConfig                        *toolConfig          `json:"toolConfig,omitempty"`
	InferenceConfig                   *inferenceConfig     `json:"inferenceConfig,omitempty"`
	AdditionalModelRequestFields      map[string]any       `json:"additionalModelRequestFields,omitempty"`
	AdditionalModelResponseFieldPaths []string             `json:"additionalModelResponseFieldPaths,omitempty"`
	ServiceTier                       *serviceTier         `json:"serviceTier,omitempty"`
}

func (in converseInput) MarshalJSON() ([]byte, error) {
	type converseInputAlias converseInput

	base, err := json.Marshal(converseInputAlias(in))
	if err != nil {
		return nil, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	for key, value := range in.passthrough {
		fields[key] = value
	}
	if in.ToolConfig != nil {
		toolConfig, err := json.Marshal(in.ToolConfig)
		if err != nil {
			return nil, err
		}
		fields["toolConfig"] = toolConfig
	}
	return json.Marshal(fields)
}

// systemContentBlock is one entry under the top-level `system` array. Either
// a text block or a cachePoint block (Converse uses cachePoints to mark cache
// breakpoints between system messages).
type systemContentBlock struct {
	Text       string      `json:"text,omitempty"`
	CachePoint *cachePoint `json:"cachePoint,omitempty"`
}

// converseMessage is a `messages` entry: a role plus content blocks.
type converseMessage struct {
	Role    string         `json:"role"` // "user" or "assistant"
	Content []contentBlock `json:"content"`
}

// contentBlock is the discriminated union of message content. At most one
// inner field is populated per block. Empty fields are omitted from JSON.
type contentBlock struct {
	Text             string                 `json:"text,omitempty"`
	Image            *imageBlock            `json:"image,omitempty"`
	Video            *videoBlock            `json:"video,omitempty"`
	Document         *documentBlock         `json:"document,omitempty"`
	ToolUse          *toolUseBlock          `json:"toolUse,omitempty"`
	ToolResult       *toolResultBlock       `json:"toolResult,omitempty"`
	ReasoningContent *reasoningContentBlock `json:"reasoningContent,omitempty"`
	GuardContent     *guardContentBlock     `json:"guardContent,omitempty"`
	CachePoint       *cachePoint            `json:"cachePoint,omitempty"`
}

// imageBlock is the Converse representation of an image part.
type imageBlock struct {
	Format string      `json:"format"` // "jpeg", "png", "gif", "webp"
	Source imageSource `json:"source"`
}

type guardContentBlock struct {
	Text  *guardrailTextBlock `json:"text,omitempty"`
	Image *imageBlock         `json:"image,omitempty"`
}

type guardrailTextBlock struct {
	Text       string                  `json:"text"`
	Qualifiers []GuardContentQualifier `json:"qualifiers,omitempty"`
}

type imageSource struct {
	Bytes      string           `json:"bytes,omitempty"`
	S3Location *s3LocationBlock `json:"s3Location,omitempty"`
}

type videoBlock struct {
	Format string      `json:"format"`
	Source videoSource `json:"source"`
}

type videoSource struct {
	Bytes      string           `json:"bytes,omitempty"`
	S3Location *s3LocationBlock `json:"s3Location,omitempty"`
}

type s3LocationBlock struct {
	URI string `json:"uri"`
}

// documentBlock is the Converse representation of a document part. Supports
// optional citations.
type documentBlock struct {
	Format    string             `json:"format"` // "pdf", "csv", "doc", "docx", ...
	Name      string             `json:"name"`
	Source    documentSource     `json:"source"`
	Citations *documentCitations `json:"citations,omitempty"`
}

type documentSource struct {
	Bytes string `json:"bytes"` // base64-encoded document bytes
}

type documentCitations struct {
	Enabled bool `json:"enabled"`
}

// toolUseBlock is the assistant-side tool call.
type toolUseBlock struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

// toolResultBlock is a user-role tool result. Per Converse contract, tool
// results live in user messages (not a separate tool role).
type toolResultBlock struct {
	ToolUseID string              `json:"toolUseId"`
	Content   []toolResultContent `json:"content"`
	Status    string              `json:"status,omitempty"` // "success" or "error"
}

// toolResultContent is one entry in toolResultBlock.Content. JSON tool outputs
// are stringified into Text to match upstream, so there is no dedicated json
// field.
type toolResultContent struct {
	Text     string         `json:"text,omitempty"`
	Image    *imageBlock    `json:"image,omitempty"`
	Video    *videoBlock    `json:"video,omitempty"`
	Document *documentBlock `json:"document,omitempty"`
}

// reasoningContentBlock is the assistant-side reasoning trace.
type reasoningContentBlock struct {
	ReasoningText     *reasoningText     `json:"reasoningText,omitempty"`
	RedactedContent   string             `json:"redactedContent,omitempty"`
	RedactedReasoning *redactedReasoning `json:"redactedReasoning,omitempty"`
}

type reasoningText struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
}

type redactedReasoning struct {
	Data string `json:"data"`
}

// cachePoint marks a prompt-cache breakpoint. Bedrock places the block where
// the breakpoint should appear within the message stream.
type cachePoint struct {
	Type string `json:"type"`          // "default"
	TTL  string `json:"ttl,omitempty"` // "5m" or "1h"
}

// toolConfig groups tools and tool choice. Sent only when at least one tool
// is defined.
type toolConfig struct {
	Tools      []toolDefinition `json:"tools,omitempty"`
	ToolChoice *toolChoiceUnion `json:"toolChoice,omitempty"`
}

// toolDefinition wraps a function tool (toolSpec). Cache breakpoints between
// tools are also valid as cachePoint entries; we currently only emit toolSpec.
type toolDefinition struct {
	ToolSpec   *toolSpec   `json:"toolSpec,omitempty"`
	CachePoint *cachePoint `json:"cachePoint,omitempty"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
	InputSchema toolInputSchema `json:"inputSchema"`
}

type toolInputSchema struct {
	JSON json.RawMessage `json:"json"`
}

// toolChoiceUnion mirrors the upstream tool choice discriminated union.
// Exactly one field is set per value.
type toolChoiceUnion struct {
	Auto *struct{}               `json:"auto,omitempty"`
	Any  *struct{}               `json:"any,omitempty"`
	Tool *toolChoiceSpecificTool `json:"tool,omitempty"`
}

type toolChoiceSpecificTool struct {
	Name string `json:"name"`
}

// inferenceConfig groups the scalar sampling/length parameters that Converse
// accepts natively.
type inferenceConfig struct {
	MaxTokens     *int     `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	TopK          *int     `json:"topK,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// serviceTier optionally pins the Bedrock service tier (priority/standard).
type serviceTier struct {
	Type string `json:"type"`
}

// converseResponse is the non-streaming Converse response shape. Only the
// fields we read are modeled; unknown fields are dropped silently. Mirrors
// upstream AmazonBedrockResponseSchema.
type converseResponse struct {
	Output                        *converseResponseOutput   `json:"output,omitempty"`
	StopReason                    string                    `json:"stopReason,omitempty"`
	Usage                         *converseUsage            `json:"usage,omitempty"`
	Metrics                       *converseMetrics          `json:"metrics,omitempty"`
	Trace                         json.RawMessage           `json:"trace,omitempty"`
	PerformanceConfig             json.RawMessage           `json:"performanceConfig,omitempty"`
	ServiceTier                   json.RawMessage           `json:"serviceTier,omitempty"`
	AdditionalModelResponseFields *additionalResponseFields `json:"additionalModelResponseFields,omitempty"`
}

type converseResponseOutput struct {
	Message converseResponseMessage `json:"message"`
}

type converseResponseMessage struct {
	Role    string                 `json:"role"`
	Content []responseContentBlock `json:"content"`
}

// responseContentBlock is the response-side content block. Same union as
// request side, with the addition of reasoning + tool use without input
// streaming.
type responseContentBlock struct {
	Text             string                 `json:"text,omitempty"`
	ToolUse          *responseToolUseBlock  `json:"toolUse,omitempty"`
	ReasoningContent *reasoningContentBlock `json:"reasoningContent,omitempty"`
}

type responseToolUseBlock struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

// converseUsage mirrors upstream usage block, including optional cache fields
// emitted on cached calls.
type converseUsage struct {
	Raw                   json.RawMessage `json:"-"`
	InputTokens           int             `json:"inputTokens"`
	OutputTokens          int             `json:"outputTokens"`
	TotalTokens           int             `json:"totalTokens,omitempty"`
	CacheReadInputTokens  *int            `json:"cacheReadInputTokens,omitempty"`
	CacheWriteInputTokens *int            `json:"cacheWriteInputTokens,omitempty"`
	CacheDetails          json.RawMessage `json:"cacheDetails,omitempty"`
}

func (u *converseUsage) UnmarshalJSON(data []byte) error {
	type alias converseUsage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = converseUsage(decoded)
	u.Raw = json.RawMessage(append([]byte(nil), data...))
	return nil
}

type converseMetrics struct {
	LatencyMs int `json:"latencyMs,omitempty"`
}

type additionalResponseFields struct {
	Delta *additionalResponseDelta `json:"delta,omitempty"`
}

type additionalResponseDelta struct {
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// converseError is the JSON error body returned on non-2xx Converse responses
// and inside event-stream exception frames.
type converseError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

type streamContentBlockStart struct {
	ContentBlockIndex int                      `json:"contentBlockIndex"`
	Start             *streamContentStartUnion `json:"start,omitempty"`
}

type streamContentStartUnion struct {
	ToolUse *streamToolUseStart `json:"toolUse,omitempty"`
}

type streamToolUseStart struct {
	ToolUseID string `json:"toolUseId,omitempty"`
	Name      string `json:"name,omitempty"`
}

type streamContentBlockDelta struct {
	ContentBlockIndex int                      `json:"contentBlockIndex"`
	Delta             *streamContentDeltaUnion `json:"delta,omitempty"`
}

type streamContentDeltaUnion struct {
	Text             string                       `json:"text,omitempty"`
	ToolUse          *streamToolUseDelta          `json:"toolUse,omitempty"`
	ReasoningContent *streamReasoningContentDelta `json:"reasoningContent,omitempty"`
}

type streamToolUseDelta struct {
	Input string `json:"input,omitempty"`
}

type streamReasoningContentDelta struct {
	Text            string `json:"text,omitempty"`
	Signature       string `json:"signature,omitempty"`
	RedactedContent string `json:"redactedContent,omitempty"`
	Data            string `json:"data,omitempty"`
}

type streamContentBlockStop struct {
	ContentBlockIndex int `json:"contentBlockIndex"`
}

type streamMessageStop struct {
	StopReason                    string                    `json:"stopReason,omitempty"`
	AdditionalModelResponseFields *additionalResponseFields `json:"additionalModelResponseFields,omitempty"`
}

type streamMetadata struct {
	Usage             *converseUsage   `json:"usage,omitempty"`
	Metrics           *converseMetrics `json:"metrics,omitempty"`
	Trace             json.RawMessage  `json:"trace,omitempty"`
	PerformanceConfig json.RawMessage  `json:"performanceConfig,omitempty"`
	ServiceTier       json.RawMessage  `json:"serviceTier,omitempty"`
}
