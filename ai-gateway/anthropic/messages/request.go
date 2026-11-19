package messages

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

var (
	errInvalidRequest     = errors.New("anthropic messages: invalid request")
	errUnsupportedRequest = errors.New("anthropic messages: unsupported request")
)

type request struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       json.RawMessage `json:"system"`
	Messages     []message       `json:"messages"`
	Tools        []tool          `json:"tools"`
	ToolChoice   *toolChoice     `json:"tool_choice"`
	OutputConfig *outputConfig   `json:"output_config"`
	Stop         []string        `json:"stop_sequences"`
	Temperature  *float64        `json:"temperature"`
	TopP         *float64        `json:"top_p"`
	TopK         *int            `json:"top_k"`
	Stream       bool            `json:"stream"`
}

type message struct {
	Role    provider.Role   `json:"role"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type      contentBlockType `json:"type"`
	Text      string           `json:"text"`
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Input     json.RawMessage  `json:"input"`
	ToolUseID string           `json:"tool_use_id"`
	Content   json.RawMessage  `json:"content"`
	IsError   bool             `json:"is_error"`
}

type tool struct {
	Type        toolType        `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Strict      *bool           `json:"strict"`
}

type toolChoice struct {
	Type                   toolChoiceType `json:"type"`
	Name                   string         `json:"name"`
	DisableParallelToolUse *bool          `json:"disable_parallel_tool_use"`
}

type outputConfig struct {
	Effort *provider.ReasoningEffort `json:"effort"`
}

// mapRequest maps an Anthropic Messages request to provider call options.
// Fields that tune only the Anthropic wire, such as cache_control, metadata,
// service_tier and the thinking configuration, are accepted and not applied.
func mapRequest(body []byte) (request, provider.CallOptions, error) {
	var in request
	if err := json.Unmarshal(body, &in); err != nil {
		return in, provider.CallOptions{}, errInvalidRequest
	}
	if in.Model == "" || in.MaxTokens <= 0 || len(in.Messages) == 0 {
		return in, provider.CallOptions{}, errInvalidRequest
	}
	if in.OutputConfig != nil && in.OutputConfig.Effort != nil {
		switch *in.OutputConfig.Effort {
		case provider.ReasoningLow, provider.ReasoningMedium, provider.ReasoningHigh, provider.ReasoningXHigh, provider.ReasoningMax:
		default:
			return in, provider.CallOptions{}, errInvalidRequest
		}
	}
	options := provider.CallOptions{MaxOutputTokens: &in.MaxTokens, Temperature: in.Temperature, TopP: in.TopP, TopK: in.TopK, StopSequences: in.Stop}
	if isPresent(in.System) {
		text, err := textContent(in.System)
		if err != nil {
			return in, options, err
		}
		if text != "" {
			options.Prompt = append(options.Prompt, provider.NewSystemMessage(text))
		}
	}
	toolNames := make(map[string]string)
	for _, msg := range in.Messages {
		messages, err := mapMessage(msg, toolNames)
		if err != nil {
			return in, options, err
		}
		options.Prompt = append(options.Prompt, messages...)
	}
	for _, t := range in.Tools {
		if t.Type != toolTypeDefault && t.Type != toolTypeCustom {
			return in, options, errUnsupportedRequest
		}
		if !isPresent(t.InputSchema) {
			t.InputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		if t.Name == "" || !isObject(t.InputSchema) {
			return in, options, errInvalidRequest
		}
		options.Tools = append(options.Tools, provider.Tool{Type: provider.ToolTypeFunction, Name: t.Name, Description: t.Description, InputSchema: t.InputSchema, Strict: t.Strict})
	}
	if in.ToolChoice != nil {
		choice := &provider.ToolChoice{}
		switch in.ToolChoice.Type {
		case toolChoiceAuto:
			choice.Type = provider.ToolChoiceAuto
		case toolChoiceAny:
			choice.Type = provider.ToolChoiceRequired
		case toolChoiceTool:
			if in.ToolChoice.Name == "" {
				return in, options, errInvalidRequest
			}
			choice.Type, choice.ToolName = provider.ToolChoiceTool, in.ToolChoice.Name
		case toolChoiceNone:
			choice.Type = provider.ToolChoiceNone
		default:
			return in, options, errUnsupportedRequest
		}
		options.ToolChoice = choice
	}
	return in, options, nil
}

func mapMessage(msg message, toolNames map[string]string) ([]provider.Message, error) {
	if msg.Role == provider.RoleSystem {
		text, err := textContent(msg.Content)
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return []provider.Message{provider.NewSystemMessage(text)}, nil
	}
	var role provider.Role
	switch msg.Role {
	case provider.RoleUser:
		role = provider.RoleUser
	case provider.RoleAssistant:
		role = provider.RoleAssistant
	default:
		return nil, errInvalidRequest
	}
	var blocks []block
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		var text string
		if err := json.Unmarshal(msg.Content, &text); err != nil {
			return nil, errInvalidRequest
		}
		blocks = []block{{Type: contentBlockText, Text: text}}
	}
	var out []provider.Message
	var parts []provider.ContentPart
	flush := func() {
		if len(parts) != 0 {
			out = append(out, provider.Message{Role: role, Content: parts})
			parts = nil
		}
	}
	for _, item := range blocks {
		switch item.Type {
		case contentBlockText:
			if item.Text != "" {
				parts = append(parts, provider.TextPart(item.Text))
			}
		case contentBlockThinking, contentBlockRedactedThinking:
		case contentBlockToolUse:
			if role != provider.RoleAssistant || item.ID == "" || item.Name == "" || !isObject(item.Input) {
				return nil, errInvalidRequest
			}
			toolNames[item.ID] = item.Name
			parts = append(parts, provider.ToolCallPart(item.ID, item.Name, item.Input))
		case contentBlockToolResult:
			name, known := toolNames[item.ToolUseID]
			if role != provider.RoleUser || item.ToolUseID == "" || !known {
				return nil, errInvalidRequest
			}
			text, err := textContent(item.Content)
			if err != nil {
				return nil, err
			}
			outputType := provider.ToolOutputText
			if item.IsError {
				outputType = provider.ToolOutputErrorText
			}
			flush()
			out = append(out, provider.NewToolMessage(provider.ToolResultPart(item.ToolUseID, name, &provider.ToolResultOutput{Type: outputType, Text: text})))
		default:
			return nil, errUnsupportedRequest
		}
	}
	flush()
	return out, nil
}

func isPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null"))
}

func isObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func textContent(raw json.RawMessage) (string, error) {
	if !isPresent(raw) {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var items []block
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", errInvalidRequest
	}
	var out bytes.Buffer
	for _, item := range items {
		if item.Type != contentBlockText {
			return "", errUnsupportedRequest
		}
		out.WriteString(item.Text)
	}
	return out.String(), nil
}
