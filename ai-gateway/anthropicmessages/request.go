package anthropicmessages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type request struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	System        json.RawMessage `json:"system"`
	Messages      []message       `json:"messages"`
	Tools         []tool          `json:"tools"`
	ToolChoice    *toolChoice     `json:"tool_choice"`
	Stop          []string        `json:"stop_sequences"`
	Temperature   *float64        `json:"temperature"`
	TopP          *float64        `json:"top_p"`
	TopK          *int            `json:"top_k"`
	Stream        bool            `json:"stream"`
	InferenceGeo  json.RawMessage `json:"inference_geo"`
	Container     json.RawMessage `json:"container"`
	CacheControl  json.RawMessage `json:"cache_control"`
	Diagnostics   json.RawMessage `json:"diagnostics"`
	Metadata      json.RawMessage `json:"metadata"`
	OutputConfig  json.RawMessage `json:"output_config"`
	ServiceTier   json.RawMessage `json:"service_tier"`
	Thinking      json.RawMessage `json:"thinking"`
	UserProfileID json.RawMessage `json:"user_profile_id"`
	WorkspaceID   json.RawMessage `json:"workspace_id"`
}

type message struct {
	Role    messageRole     `json:"role"`
	Content json.RawMessage `json:"content"`
}

type messageRole string

const (
	messageRoleUser      messageRole = "user"
	messageRoleAssistant messageRole = "assistant"
)

type block struct {
	Type         blockType       `json:"type"`
	Text         string          `json:"text"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	ToolUseID    string          `json:"tool_use_id"`
	Content      json.RawMessage `json:"content"`
	IsError      bool            `json:"is_error"`
	CacheControl json.RawMessage `json:"cache_control"`
	Thinking     json.RawMessage `json:"thinking"`
	Signature    json.RawMessage `json:"signature"`
	Source       json.RawMessage `json:"source"`
	Caller       json.RawMessage `json:"caller"`
}

type blockType string

const (
	blockTypeText       blockType = "text"
	blockTypeToolUse    blockType = "tool_use"
	blockTypeToolResult blockType = "tool_result"
)

type tool struct {
	Type           serverToolType    `json:"type"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	InputSchema    json.RawMessage   `json:"input_schema"`
	Strict         *bool             `json:"strict"`
	InputExamples  []json.RawMessage `json:"input_examples"`
	DeferLoading   json.RawMessage   `json:"defer_loading"`
	AllowedCallers json.RawMessage   `json:"allowed_callers"`
	CacheControl   json.RawMessage   `json:"cache_control"`
}

type serverToolType string

type toolChoice struct {
	Type                   toolChoiceType `json:"type"`
	Name                   string         `json:"name"`
	DisableParallelToolUse *bool          `json:"disable_parallel_tool_use"`
}

type toolChoiceType string

const (
	toolChoiceAuto toolChoiceType = "auto"
	toolChoiceAny  toolChoiceType = "any"
	toolChoiceTool toolChoiceType = "tool"
	toolChoiceNone toolChoiceType = "none"
)

func mapRequest(body []byte) (request, provider.CallOptions, error) {
	var input request
	if err := json.Unmarshal(body, &input); err != nil {
		return input, provider.CallOptions{}, errInvalidRequest
	}
	if input.Model == "" || input.MaxTokens <= 0 || len(input.Messages) == 0 {
		return input, provider.CallOptions{}, errInvalidRequest
	}
	if anyPresent(input.InferenceGeo, input.Container, input.CacheControl, input.Diagnostics, input.Metadata, input.OutputConfig, input.ServiceTier, input.Thinking, input.UserProfileID, input.WorkspaceID) {
		return input, provider.CallOptions{}, errUnsupportedRequest
	}
	options := provider.CallOptions{MaxOutputTokens: &input.MaxTokens, Temperature: input.Temperature, TopP: input.TopP, TopK: input.TopK, StopSequences: input.Stop}
	if len(input.System) != 0 && !bytes.Equal(bytes.TrimSpace(input.System), []byte("null")) {
		text, err := textContent(input.System)
		if err != nil {
			return input, provider.CallOptions{}, errUnsupportedRequest
		}
		options.Prompt = append(options.Prompt, provider.NewSystemMessage(text))
	}
	toolNames := make(map[string]string)
	for _, msg := range input.Messages {
		var role provider.Role
		switch msg.Role {
		case messageRoleUser:
			role = provider.RoleUser
		case messageRoleAssistant:
			role = provider.RoleAssistant
		default:
			return input, provider.CallOptions{}, errInvalidRequest
		}
		var blocks []block
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			var text string
			if err := json.Unmarshal(msg.Content, &text); err != nil {
				return input, provider.CallOptions{}, errUnsupportedRequest
			}
			blocks = []block{{Type: blockTypeText, Text: text}}
		}
		if len(blocks) == 0 {
			return input, provider.CallOptions{}, errInvalidRequest
		}
		var parts []provider.ContentPart
		var mappedMessages []provider.Message
		for _, item := range blocks {
			if anyPresent(item.CacheControl, item.Thinking, item.Signature, item.Source, item.Caller) {
				return input, provider.CallOptions{}, errUnsupportedRequest
			}
			switch item.Type {
			case blockTypeText:
				parts = append(parts, provider.TextPart(item.Text))
			case blockTypeToolUse:
				if msg.Role != messageRoleAssistant || item.ID == "" || item.Name == "" || !isJSONObject(item.Input) {
					return input, provider.CallOptions{}, errInvalidRequest
				}
				if _, exists := toolNames[item.ID]; exists {
					return input, provider.CallOptions{}, errInvalidRequest
				}
				parts = append(parts, provider.ToolCallPart(item.ID, item.Name, item.Input))
				toolNames[item.ID] = item.Name
			case blockTypeToolResult:
				if msg.Role != messageRoleUser || item.ToolUseID == "" {
					return input, provider.CallOptions{}, errInvalidRequest
				}
				result, err := textContent(item.Content)
				if err != nil {
					return input, provider.CallOptions{}, errUnsupportedRequest
				}
				name, exists := toolNames[item.ToolUseID]
				if !exists {
					return input, provider.CallOptions{}, errInvalidRequest
				}
				delete(toolNames, item.ToolUseID)
				outputType := provider.ToolOutputText
				if item.IsError {
					outputType = provider.ToolOutputErrorText
				}
				if len(parts) != 0 {
					mappedMessages = append(mappedMessages, provider.Message{Role: provider.RoleUser, Content: parts})
					parts = nil
				}
				mappedMessages = append(mappedMessages, provider.NewToolMessage(provider.ToolResultPart(item.ToolUseID, name, &provider.ToolResultOutput{Type: outputType, Text: result})))
			default:
				return input, provider.CallOptions{}, errUnsupportedRequest
			}
		}
		if len(parts) != 0 {
			mappedMessages = append(mappedMessages, provider.Message{Role: role, Content: parts})
		}
		options.Prompt = append(options.Prompt, mappedMessages...)
	}
	for _, inputTool := range input.Tools {
		if anyPresent(inputTool.DeferLoading, inputTool.AllowedCallers, inputTool.CacheControl) {
			return input, provider.CallOptions{}, errUnsupportedRequest
		}
		if inputTool.Type != "" {
			return input, provider.CallOptions{}, errUnsupportedRequest
		}
		if inputTool.Name == "" || !json.Valid(inputTool.InputSchema) {
			return input, provider.CallOptions{}, errInvalidRequest
		}
		var examples []provider.InputExample
		for _, example := range inputTool.InputExamples {
			if !isJSONObject(example) {
				return input, provider.CallOptions{}, errInvalidRequest
			}
			examples = append(examples, provider.InputExample{Input: example})
		}
		options.Tools = append(options.Tools, provider.Tool{Type: provider.ToolTypeFunction, Name: inputTool.Name, Description: inputTool.Description, InputSchema: inputTool.InputSchema, Strict: inputTool.Strict, InputExamples: examples})
	}
	if input.ToolChoice != nil {
		choice := &provider.ToolChoice{}
		switch input.ToolChoice.Type {
		case toolChoiceAuto:
			choice.Type = provider.ToolChoiceAuto
		case toolChoiceAny:
			choice.Type = provider.ToolChoiceRequired
		case toolChoiceTool:
			if input.ToolChoice.Name == "" {
				return input, provider.CallOptions{}, errInvalidRequest
			}
			choice.Type, choice.ToolName = provider.ToolChoiceTool, input.ToolChoice.Name
		case toolChoiceNone:
			choice.Type = provider.ToolChoiceNone
		default:
			return input, provider.CallOptions{}, errUnsupportedRequest
		}
		options.ToolChoice = choice
		if input.ToolChoice.DisableParallelToolUse != nil {
			options.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{
				Key: "anthropic", Raw: json.RawMessage(fmt.Sprintf(`{"disableParallelToolUse":%t}`, *input.ToolChoice.DisableParallelToolUse)),
			}}
		}
	}
	return input, options, nil
}

func anyPresent(values ...json.RawMessage) bool {
	for _, value := range values {
		if len(value) != 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return true
		}
	}
	return false
}

func isJSONObject(value json.RawMessage) bool {
	if len(value) == 0 || !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

func textContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var items []block
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", fmt.Errorf("anthropic messages: invalid text content")
	}
	var parts []string
	for _, item := range items {
		if item.Type != blockTypeText || anyPresent(item.CacheControl, item.Thinking, item.Signature, item.Source, item.Caller) {
			return "", errUnsupportedRequest
		}
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, ""), nil
}
