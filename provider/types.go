package provider

import (
	"encoding/json"
	"fmt"
)

// ProviderMetadata carries provider-specific metadata as opaque JSON values,
// keyed by provider name (e.g., "anthropic", "google").
type ProviderMetadata map[string]json.RawMessage

// UnifiedFinishReason is the standardized reason why the model stopped generating.
type UnifiedFinishReason string

const (
	FinishReasonStop          UnifiedFinishReason = "stop"
	FinishReasonLength        UnifiedFinishReason = "length"
	FinishReasonContentFilter UnifiedFinishReason = "content-filter"
	FinishReasonToolCalls     UnifiedFinishReason = "tool-calls"
	FinishReasonError         UnifiedFinishReason = "error"
	FinishReasonOther         UnifiedFinishReason = "other"
)

// FinishReason carries both the standardized (Unified) and provider-native (Raw)
// reason why the model stopped generating.
type FinishReason struct {
	Unified UnifiedFinishReason `json:"unified"`
	Raw     string              `json:"raw,omitempty"`
}

// Usage reports token consumption for a model call.
type Usage struct {
	InputTokens  InputTokenUsage  `json:"inputTokens"`
	OutputTokens OutputTokenUsage `json:"outputTokens"`
	Raw          json.RawMessage  `json:"raw,omitempty"`
}

// InputTokenUsage breaks down input token usage.
type InputTokenUsage struct {
	Total      *int `json:"total,omitempty"`
	NoCache    *int `json:"noCache,omitempty"`
	CacheRead  *int `json:"cacheRead,omitempty"`
	CacheWrite *int `json:"cacheWrite,omitempty"`
}

// OutputTokenUsage breaks down output token usage.
type OutputTokenUsage struct {
	Total     *int `json:"total,omitempty"`
	Text      *int `json:"text,omitempty"`
	Reasoning *int `json:"reasoning,omitempty"`
}

// ToolChoiceType specifies how the model should select tools.
type ToolChoiceType string

const (
	ToolChoiceAuto     ToolChoiceType = "auto"
	ToolChoiceNone     ToolChoiceType = "none"
	ToolChoiceRequired ToolChoiceType = "required"
	ToolChoiceTool     ToolChoiceType = "tool"
)

// ToolChoice specifies how the model should select tools.
type ToolChoice struct {
	Type     ToolChoiceType `json:"type"`
	ToolName string         `json:"toolName,omitempty"`
}

// WarningType identifies the kind of provider warning.
type WarningType string

const (
	WarnUnsupported   WarningType = "unsupported"
	WarnCompatibility WarningType = "compatibility"
	WarnDeprecated    WarningType = "deprecated"
	WarnOther         WarningType = "other"
)

// Warning reports a provider warning (e.g., unsupported feature).
type Warning struct {
	Type    WarningType `json:"type"`
	Feature string      `json:"feature,omitempty"`
	Setting string      `json:"setting,omitempty"`
	Message string      `json:"message,omitempty"`
	Details string      `json:"details,omitempty"`
}

// ReasoningEffort specifies the reasoning effort level for the model.
type ReasoningEffort string

const (
	ReasoningProviderDefault ReasoningEffort = "provider-default"
	ReasoningNone            ReasoningEffort = "none"
	ReasoningMinimal         ReasoningEffort = "minimal"
	ReasoningLow             ReasoningEffort = "low"
	ReasoningMedium          ReasoningEffort = "medium"
	ReasoningHigh            ReasoningEffort = "high"
	ReasoningXHigh           ReasoningEffort = "xhigh"
)

// ToolResultOutputType identifies the kind of tool result.
type ToolResultOutputType string

const (
	ToolOutputText            ToolResultOutputType = "text"
	ToolOutputJSON            ToolResultOutputType = "json"
	ToolOutputContent         ToolResultOutputType = "content"
	ToolOutputExecutionDenied ToolResultOutputType = "execution-denied"
	ToolOutputErrorText       ToolResultOutputType = "error-text"
	ToolOutputErrorJSON       ToolResultOutputType = "error-json"
)

// ToolResultOutput carries the output of a tool execution. The Type field
// determines which value fields are populated. The Go representation uses
// split Text/JSON/Content/Reason fields while its JSON codec emits and accepts
// upstream's single `value` union.
type ToolResultOutput struct {
	Type            ToolResultOutputType     `json:"type"`
	Text            string                   `json:"text,omitempty"`
	JSON            json.RawMessage          `json:"json,omitempty"`
	Content         []ToolResultContentValue `json:"content,omitempty"`
	Reason          string                   `json:"reason,omitempty"`
	ProviderOptions ProviderOptions          `json:"providerOptions,omitempty"`
}

// MarshalJSON emits the upstream Vercel AI SDK LanguageModelV4 single-`value`
// tool-result output shape (`{"type":"text","value":...}`,
// `{"type":"json","value":...}`, `{"type":"content","value":[...]}`,
// `{"type":"execution-denied","reason":...}`), superseding the split
// `text`/`json`/`content`/`reason` emitted form. Decoding remains tolerant of
// both shapes (see [ToolResultOutput.UnmarshalJSON]). See openspec change
// provider-wire-upstream-full-compat.
func (t ToolResultOutput) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	typeB, err := json.Marshal(t.Type)
	if err != nil {
		return nil, err
	}
	out["type"] = typeB
	switch t.Type {
	case ToolOutputText, ToolOutputErrorText:
		b, err := json.Marshal(t.Text)
		if err != nil {
			return nil, err
		}
		out["value"] = b
	case ToolOutputJSON, ToolOutputErrorJSON:
		if len(t.JSON) > 0 {
			out["value"] = t.JSON
		} else {
			out["value"] = json.RawMessage("null")
		}
	case ToolOutputContent:
		b, err := json.Marshal(t.Content)
		if err != nil {
			return nil, err
		}
		out["value"] = b
	case ToolOutputExecutionDenied:
		if t.Reason != "" {
			b, err := json.Marshal(t.Reason)
			if err != nil {
				return nil, err
			}
			out["reason"] = b
		}
	}
	if len(t.ProviderOptions) > 0 {
		b, err := json.Marshal(t.ProviderOptions)
		if err != nil {
			return nil, err
		}
		out["providerOptions"] = b
	}
	return json.Marshal(out)
}

// UnmarshalJSON decodes a [ToolResultOutput] from the upstream Vercel AI SDK
// LanguageModelV4 single-`value` shape and additionally tolerates the legacy
// Go-to-Go split `text`/`json`/`content`/`reason` fields.
//
// Decoding fails closed: if an upstream `value` cannot be mapped onto the
// canonical field for its type (e.g. a malformed value, or a `content` value
// carrying a shape this build cannot represent), the error is returned rather
// than silently dropping the payload.
func (t *ToolResultOutput) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type            ToolResultOutputType     `json:"type"`
		Text            string                   `json:"text"`
		JSON            json.RawMessage          `json:"json"`
		Content         []ToolResultContentValue `json:"content"`
		Reason          string                   `json:"reason"`
		Value           json.RawMessage          `json:"value"`
		ProviderOptions ProviderOptions          `json:"providerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = ToolResultOutput{
		Type:            raw.Type,
		Text:            raw.Text,
		JSON:            raw.JSON,
		Content:         raw.Content,
		Reason:          raw.Reason,
		ProviderOptions: raw.ProviderOptions,
	}
	// Upstream single-`value` shape: map onto the canonical split fields.
	if len(raw.Value) != 0 && string(raw.Value) != "null" {
		switch raw.Type {
		case ToolOutputText, ToolOutputErrorText:
			var s string
			if err := json.Unmarshal(raw.Value, &s); err != nil {
				return fmt.Errorf("provider: decoding tool-result %q value as string: %w", raw.Type, err)
			}
			t.Text = s
		case ToolOutputJSON, ToolOutputErrorJSON:
			t.JSON = raw.Value
		case ToolOutputContent:
			var cv []ToolResultContentValue
			if err := json.Unmarshal(raw.Value, &cv); err != nil {
				return fmt.Errorf("provider: decoding tool-result content value: %w", err)
			}
			t.Content = cv
		}
	}
	return nil
}

// ToolResultContentType identifies the kind of content in a tool result.
type ToolResultContentType string

const (
	ToolContentText   ToolResultContentType = "text"
	ToolContentFile   ToolResultContentType = "file"
	ToolContentCustom ToolResultContentType = "custom"

	ToolContentFileData      ToolResultContentType = "file-data"
	ToolContentFileURL       ToolResultContentType = "file-url"
	ToolContentFileReference ToolResultContentType = "file-reference"
)

// ToolResultContentValue is a single piece of multimodal tool result content.
// File content uses [DataContent], matching the LanguageModelV4 tagged data
// union. The legacy file-data, file-url, and file-reference discriminators are
// accepted during decoding and normalized to [ToolContentFile].
type ToolResultContentValue struct {
	Type            ToolResultContentType `json:"type"`
	Text            string                `json:"text,omitempty"`
	Data            *DataContent          `json:"data,omitempty"`
	MediaType       string                `json:"mediaType,omitempty"`
	Filename        string                `json:"filename,omitempty"`
	ProviderOptions ProviderOptions       `json:"providerOptions,omitempty"`
}

// MarshalJSON emits the upstream LanguageModelV4 tool-result content union.
func (v ToolResultContentValue) MarshalJSON() ([]byte, error) {
	type wireValue struct {
		Type            ToolResultContentType `json:"type"`
		Text            string                `json:"text,omitempty"`
		Data            *DataContent          `json:"data,omitempty"`
		MediaType       string                `json:"mediaType,omitempty"`
		Filename        string                `json:"filename,omitempty"`
		ProviderOptions ProviderOptions       `json:"providerOptions,omitempty"`
	}

	typeValue := v.Type
	switch typeValue {
	case ToolContentFileData, ToolContentFileURL, ToolContentFileReference:
		typeValue = ToolContentFile
	}
	return json.Marshal(wireValue{
		Type:            typeValue,
		Text:            v.Text,
		Data:            v.Data,
		MediaType:       v.MediaType,
		Filename:        v.Filename,
		ProviderOptions: v.ProviderOptions,
	})
}

// UnmarshalJSON decodes canonical LanguageModelV4 tool-result content and the
// legacy flat file-data, file-url, and file-reference variants.
func (v *ToolResultContentValue) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type              ToolResultContentType `json:"type"`
		Text              string                `json:"text"`
		Data              json.RawMessage       `json:"data"`
		MediaType         string                `json:"mediaType"`
		Filename          string                `json:"filename"`
		URL               string                `json:"url"`
		ProviderReference map[string]string     `json:"providerReference"`
		ProviderOptions   ProviderOptions       `json:"providerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*v = ToolResultContentValue{
		Type:            raw.Type,
		Text:            raw.Text,
		MediaType:       raw.MediaType,
		Filename:        raw.Filename,
		ProviderOptions: raw.ProviderOptions,
	}

	switch raw.Type {
	case ToolContentFile:
		if len(raw.Data) != 0 && string(raw.Data) != "null" {
			var fileData DataContent
			if err := json.Unmarshal(raw.Data, &fileData); err != nil {
				return fmt.Errorf("provider: decoding tool-result file data: %w", err)
			}
			v.Data = &fileData
		}
	case ToolContentFileData:
		var base64Data string
		if len(raw.Data) != 0 && string(raw.Data) != "null" {
			if err := json.Unmarshal(raw.Data, &base64Data); err != nil {
				return fmt.Errorf("provider: decoding legacy tool-result file data: %w", err)
			}
		}
		v.Type = ToolContentFile
		v.Data = &DataContent{Base64: base64Data}
	case ToolContentFileURL:
		v.Type = ToolContentFile
		v.Data = &DataContent{URL: raw.URL}
	case ToolContentFileReference:
		v.Type = ToolContentFile
		if raw.ProviderReference != nil {
			reference, err := json.Marshal(raw.ProviderReference)
			if err != nil {
				return fmt.Errorf("provider: encoding legacy tool-result file reference: %w", err)
			}
			v.Data = &DataContent{Reference: reference}
		}
	}
	return nil
}
