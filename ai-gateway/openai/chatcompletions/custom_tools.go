package chatcompletions

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type customCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

func (c toolCall) name() string {
	if c.Type == kindCustom && c.Custom != nil {
		return c.Custom.Name
	}
	return c.Function.Name
}
func (c toolCall) arguments() string {
	if c.Type == kindCustom && c.Custom != nil {
		return c.Custom.Input
	}
	return c.Function.Arguments
}
func validHistoryCall(c toolCall) bool {
	if c.Type == kindCustom {
		return c.Custom != nil && c.Function.Name == "" && c.Function.Arguments == "" && utf8.ValidString(c.Custom.Input)
	}
	return c.Type == kindFunction && c.Custom == nil && jsonObject([]byte(c.Function.Arguments))
}
func customHistoryMarker() provider.ProviderOptions {
	return provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"customTool":true}`)}, "openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"customTool":true}`)}}
}
func (r mappedRequest) customTool(name string) bool {
	for _, tool := range r.options.Tools {
		if tool.Name == name {
			return tool.Type == provider.ToolTypeProvider && tool.ID == "openai.custom"
		}
	}
	return false
}
func (r mappedRequest) hasCustomTools() bool {
	for _, tool := range r.options.Tools {
		if tool.Type == provider.ToolTypeProvider && tool.ID == "openai.custom" {
			return true
		}
	}
	for _, message := range r.Messages {
		for _, call := range message.ToolCalls {
			if call.Type == kindCustom {
				return true
			}
		}
	}
	return false
}
func (r mappedRequest) outputCall(id, name, input string) (toolCall, error) {
	if r.customTool(name) {
		var text string
		if json.Unmarshal([]byte(input), &text) != nil {
			return toolCall{}, errOutput
		}
		return toolCall{ID: id, Type: kindCustom, Custom: &customCall{Name: name, Input: text}}, nil
	}
	return toolCall{ID: id, Type: kindFunction, Function: callFunction{Name: name, Arguments: input}}, nil
}
