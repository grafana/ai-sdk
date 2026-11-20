package messages

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

var (
	errInvalidRequest     = errors.New("anthropic messages: invalid request")
	errUnsupportedRequest = errors.New("anthropic messages: unsupported request")
)

type request struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      json.RawMessage `json:"system"`
	Messages    []message       `json:"messages"`
	Tools       []tool          `json:"tools"`
	ToolChoice  *toolChoice     `json:"tool_choice"`
	Stop        []string        `json:"stop_sequences"`
	Temperature *float64        `json:"temperature"`
	TopP        *float64        `json:"top_p"`
	TopK        *int            `json:"top_k"`
	Stream      bool            `json:"stream"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Strict      *bool           `json:"strict"`
}

type toolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name"`
	DisableParallelToolUse *bool  `json:"disable_parallel_tool_use"`
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
	options := provider.CallOptions{MaxOutputTokens: &in.MaxTokens, Temperature: in.Temperature, TopP: in.TopP, TopK: in.TopK, StopSequences: in.Stop}
	system, err := systemMessages(in.System)
	if err != nil {
		return in, options, err
	}
	options.Prompt = append(options.Prompt, system...)
	toolNames := make(map[string]string)
	for _, msg := range in.Messages {
		messages, err := mapMessage(msg, toolNames)
		if err != nil {
			return in, options, err
		}
		options.Prompt = append(options.Prompt, messages...)
	}
	for _, t := range in.Tools {
		if t.Type != "" && t.Type != "custom" {
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
		case "auto":
			choice.Type = provider.ToolChoiceAuto
		case "any":
			choice.Type = provider.ToolChoiceRequired
		case "tool":
			if in.ToolChoice.Name == "" {
				return in, options, errInvalidRequest
			}
			choice.Type, choice.ToolName = provider.ToolChoiceTool, in.ToolChoice.Name
		case "none":
			choice.Type = provider.ToolChoiceNone
		default:
			return in, options, errUnsupportedRequest
		}
		options.ToolChoice = choice
	}
	return in, options, nil
}

func mapMessage(msg message, toolNames map[string]string) ([]provider.Message, error) {
	if msg.Role == "system" {
		return systemMessages(msg.Content)
	}
	var role provider.Role
	switch msg.Role {
	case "user":
		role = provider.RoleUser
	case "assistant":
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
		blocks = []block{{Type: "text", Text: text}}
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
		case "text":
			if item.Text != "" {
				parts = append(parts, provider.TextPart(item.Text))
			}
		case "thinking", "redacted_thinking":
		case "tool_use":
			if role != provider.RoleAssistant || item.ID == "" || item.Name == "" || !isObject(item.Input) {
				return nil, errInvalidRequest
			}
			toolNames[item.ID] = item.Name
			parts = append(parts, provider.ToolCallPart(item.ID, item.Name, item.Input))
		case "tool_result":
			name, known := toolNames[item.ToolUseID]
			if role != provider.RoleUser || item.ToolUseID == "" || !known {
				return nil, errInvalidRequest
			}
			texts, err := textBlocks(item.Content)
			if err != nil {
				return nil, err
			}
			outputType := provider.ToolOutputText
			if item.IsError {
				outputType = provider.ToolOutputErrorText
			}
			flush()
			out = append(out, provider.NewToolMessage(provider.ToolResultPart(item.ToolUseID, name, &provider.ToolResultOutput{Type: outputType, Text: strings.Join(texts, "\n")})))
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

func systemMessages(raw json.RawMessage) ([]provider.Message, error) {
	texts, err := textBlocks(raw)
	if err != nil {
		return nil, err
	}
	var messages []provider.Message
	for _, text := range texts {
		if text != "" {
			messages = append(messages, provider.NewSystemMessage(text))
		}
	}
	return messages, nil
}

func textBlocks(raw json.RawMessage) ([]string, error) {
	if !isPresent(raw) {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []string{text}, nil
	}
	var items []block
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errInvalidRequest
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type != "text" {
			return nil, errUnsupportedRequest
		}
		texts = append(texts, item.Text)
	}
	return texts, nil
}
