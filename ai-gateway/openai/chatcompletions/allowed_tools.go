package chatcompletions

import (
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	openai "github.com/grafana/ai-sdk/providers/openai"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"slices"
)

const kindAllowedTools kind = "allowed_tools"

type namedTool struct {
	Name string `json:"name"`
}
type toolReference struct {
	Type     kind       `json:"type"`
	Function *namedTool `json:"function,omitempty"`
	Custom   *namedTool `json:"custom,omitempty"`
}

func (r mappedRequest) referenceName(ref toolReference, names map[string]bool) (string, error) {
	name := ""
	if ref.Type == kindFunction && ref.Function != nil && ref.Custom == nil {
		name = ref.Function.Name
	} else if ref.Type == kindCustom && ref.Custom != nil && ref.Function == nil {
		name = ref.Custom.Name
	} else {
		return "", errRequest
	}
	if !names[name] || (ref.Type == kindCustom) != r.customTool(name) {
		return "", errRequest
	}
	return name, nil
}
func (r *mappedRequest) mapNamedOrAllowedChoice(names map[string]bool) error {
	var choice struct {
		Type         kind       `json:"type"`
		Function     *namedTool `json:"function"`
		Custom       *namedTool `json:"custom"`
		AllowedTools *struct {
			Mode  provider.ToolChoiceType `json:"mode"`
			Tools []toolReference         `json:"tools"`
		} `json:"allowed_tools"`
	}
	if decodeStrict(r.ToolChoice, &choice) != nil {
		return errRequest
	}
	if choice.Type != kindAllowedTools {
		if choice.AllowedTools != nil {
			return errRequest
		}
		name, err := r.referenceName(toolReference{Type: choice.Type, Function: choice.Function, Custom: choice.Custom}, names)
		if err != nil {
			return err
		}
		r.options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: name}
		return nil
	}
	if choice.AllowedTools == nil || choice.Function != nil || choice.Custom != nil {
		return errRequest
	}
	mode := choice.AllowedTools.Mode
	if mode != provider.ToolChoiceAuto && mode != provider.ToolChoiceRequired {
		return errRequest
	}
	r.allowedToolNames = make([]string, 0, len(choice.AllowedTools.Tools))
	for _, ref := range choice.AllowedTools.Tools {
		name, err := r.referenceName(ref, names)
		if err != nil || slices.Contains(r.allowedToolNames, name) {
			return errRequest
		}
		r.allowedToolNames = append(r.allowedToolNames, name)
	}
	if len(r.allowedToolNames) == 0 {
		if mode == provider.ToolChoiceRequired {
			return errRequest
		}
		mode = provider.ToolChoiceNone
	}
	r.options.ToolChoice = &provider.ToolChoice{Type: mode}
	return nil
}
func (r mappedRequest) toolAllowed(name string) bool {
	return r.allowedToolNames == nil || slices.Contains(r.allowedToolNames, name)
}
func (r mappedRequest) translateAllowedTools(target execution.Target, options *provider.CallOptions, values map[string]any) error {
	if r.allowedToolNames == nil {
		return nil
	}
	if len(r.allowedToolNames) == 0 {
		if target.Type == execution.TargetOpenAI || target.Type == execution.TargetOpenAICompatible {
			values["allowedTools"] = nil
		}
		return nil
	}
	if options.ToolChoice == nil || options.ToolChoice.Type != r.options.ToolChoice.Type {
		return provider.ErrUnsupportedFunctionality
	}
	for _, name := range r.allowedToolNames {
		found := false
		for _, tool := range options.Tools {
			if tool.Name == name {
				if found || r.customTool(name) && (tool.Type != provider.ToolTypeProvider || tool.ID != "openai.custom") || !r.customTool(name) && tool.Type != provider.ToolTypeFunction {
					return provider.ErrUnsupportedFunctionality
				}
				found = true
			}
		}
		if !found {
			return provider.ErrUnsupportedFunctionality
		}
	}
	switch target.Type {
	case execution.TargetOpenAI:
		values["allowedTools"] = &openai.AllowedToolsOption{ToolNames: slices.Clone(r.allowedToolNames), Mode: string(r.options.ToolChoice.Type)}
	case execution.TargetOpenAICompatible:
		values["allowedTools"] = &compatible.AllowedToolsOption{ToolNames: slices.Clone(r.allowedToolNames), Mode: r.options.ToolChoice.Type}
	case execution.TargetAnthropic:
		options.Tools = slices.DeleteFunc(slices.Clone(options.Tools), func(t provider.Tool) bool { return !r.toolAllowed(t.Name) })
	}
	return nil
}
