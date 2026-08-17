package openaicompatible

import "encoding/json"

func marshalWithExtra(value any, extra map[string]any) ([]byte, error) {
	if len(extra) == 0 {
		return json.Marshal(value)
	}

	base, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	for k, v := range extra {
		fields[k] = v
	}
	return json.Marshal(fields)
}

type chatMessage struct {
	Role             string         `json:"role"`
	Content          any            `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ExtraFields      map[string]any `json:"-"`
}

func (m chatMessage) MarshalJSON() ([]byte, error) {
	type alias chatMessage
	return marshalWithExtra(alias(m), m.ExtraFields)
}

type chatContentPart struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	ImageURL    *imageURLPart   `json:"image_url,omitempty"`
	InputAudio  *inputAudioPart `json:"input_audio,omitempty"`
	File        *filePart       `json:"file,omitempty"`
	ExtraFields map[string]any  `json:"-"`
}

func (p chatContentPart) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"type": p.Type}
	if p.Type == "text" {
		fields["text"] = p.Text
	}
	if p.ImageURL != nil {
		fields["image_url"] = p.ImageURL
	}
	if p.InputAudio != nil {
		fields["input_audio"] = p.InputAudio
	}
	if p.File != nil {
		fields["file"] = p.File
	}
	for k, v := range p.ExtraFields {
		fields[k] = v
	}
	return json.Marshal(fields)
}

type imageURLPart struct {
	URL string `json:"url"`
}

type inputAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type filePart struct {
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type chatToolCall struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Function     toolCallFunction `json:"function"`
	ExtraContent *extraContent    `json:"extra_content,omitempty"`
	ExtraFields  map[string]any   `json:"-"`
}

func (tc chatToolCall) MarshalJSON() ([]byte, error) {
	type alias chatToolCall
	out, err := marshalWithExtra(alias(tc), tc.ExtraFields)
	if err != nil || tc.ExtraContent == nil {
		return out, err
	}

	var fields map[string]any
	if err := json.Unmarshal(out, &fields); err != nil {
		return nil, err
	}
	extra, err := json.Marshal(tc.ExtraContent)
	if err != nil {
		return nil, err
	}
	var extraValue any
	if err := json.Unmarshal(extra, &extraValue); err != nil {
		return nil, err
	}
	fields["extra_content"] = extraValue
	return json.Marshal(fields)
}

type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type extraContent struct {
	Google *googleExtraContent `json:"google,omitempty"`
}

type googleExtraContent struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type responseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *jsonSchemaFormat `json:"json_schema,omitempty"`
}

type jsonSchemaFormat struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatCompletionResponse struct {
	ID      string              `json:"id"`
	Created *int64              `json:"created"`
	Model   string              `json:"model"`
	Choices []*chatChoice       `json:"choices"`
	Usage   *openAIUsage        `json:"usage,omitempty"`
	Error   *openAIErrorPayload `json:"error,omitempty"`
}

type chatChoice struct {
	Message      chatResponseMessage `json:"message"`
	Delta        chatDeltaMessage    `json:"delta"`
	FinishReason *string             `json:"finish_reason"`
}

type chatResponseMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	Reasoning        string         `json:"reasoning"`
	ToolCalls        []chatToolCall `json:"tool_calls"`
}

type chatDeltaMessage struct {
	Role             string              `json:"role"`
	Content          string              `json:"content"`
	ReasoningContent string              `json:"reasoning_content"`
	Reasoning        string              `json:"reasoning"`
	ToolCalls        []chatToolCallDelta `json:"tool_calls"`
}

type chatToolCallDelta struct {
	Index        *int               `json:"index"`
	ID           *string            `json:"id"`
	Type         string             `json:"type"`
	Function     *toolCallDeltaFunc `json:"function"`
	ExtraContent *extraContent      `json:"extra_content,omitempty"`
}

type toolCallDeltaFunc struct {
	Name      *string `json:"name"`
	Arguments *string `json:"arguments"`
}

type openAIUsage struct {
	Raw                     json.RawMessage          `json:"-"`
	PromptTokens            *int                     `json:"prompt_tokens,omitempty"`
	CompletionTokens        *int                     `json:"completion_tokens,omitempty"`
	TotalTokens             *int                     `json:"total_tokens,omitempty"`
	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
}

func (u *openAIUsage) UnmarshalJSON(data []byte) error {
	type alias openAIUsage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = openAIUsage(decoded)
	u.Raw = json.RawMessage(append([]byte(nil), data...))
	return nil
}

type promptTokensDetails struct {
	CachedTokens *int `json:"cached_tokens,omitempty"`
}

type completionTokensDetails struct {
	ReasoningTokens          *int `json:"reasoning_tokens,omitempty"`
	AcceptedPredictionTokens *int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens *int `json:"rejected_prediction_tokens,omitempty"`
}

type openAIErrorResponse struct {
	Error openAIErrorPayload `json:"error"`
}

type openAIErrorPayload struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Param   string `json:"param,omitempty"`
	Code    any    `json:"code,omitempty"`
}
