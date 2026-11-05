package openaicompatible

import (
	"fmt"
	"github.com/grafana/ai-sdk/provider"
)

func allowedToolChoice(selection *AllowedToolsOption, options provider.CallOptions) (any, error) {
	mode := selection.Mode
	if mode == "" {
		mode = provider.ToolChoiceAuto
	}
	if mode != provider.ToolChoiceAuto && mode != provider.ToolChoiceRequired {
		return nil, fmt.Errorf("openai: invalid allowed tools mode: %w", provider.ErrUnsupportedFunctionality)
	}
	if options.ToolChoice != nil && options.ToolChoice.Type != mode {
		return nil, fmt.Errorf("openai: allowed tools conflicts with common tool choice: %w", provider.ErrUnsupportedFunctionality)
	}
	if len(selection.ToolNames) == 0 {
		if mode == provider.ToolChoiceRequired {
			return nil, fmt.Errorf("openai: required allowed tools is empty: %w", provider.ErrUnsupportedFunctionality)
		}
		return "none", nil
	}
	declarations := map[string]provider.Tool{}
	for _, tool := range options.Tools {
		if _, exists := declarations[tool.Name]; exists {
			return nil, fmt.Errorf("openai: ambiguous tool name: %w", provider.ErrUnsupportedFunctionality)
		}
		declarations[tool.Name] = tool
	}
	seen := map[string]bool{}
	entries := make([]map[string]any, 0, len(selection.ToolNames))
	for _, name := range selection.ToolNames {
		tool, ok := declarations[name]
		if !ok || seen[name] {
			return nil, fmt.Errorf("openai: allowed tool is missing or duplicated: %w", provider.ErrUnsupportedFunctionality)
		}
		seen[name] = true
		switch {
		case tool.Type == provider.ToolTypeFunction:
			entries = append(entries, map[string]any{"type": "function", "function": map[string]any{"name": name}})
		case tool.Type == provider.ToolTypeProvider && tool.ID == "openai.custom":
			entries = append(entries, map[string]any{"type": "custom", "custom": map[string]any{"name": name}})
		default:
			return nil, fmt.Errorf("openai: unsupported allowed tool: %w", provider.ErrUnsupportedFunctionality)
		}
	}
	return map[string]any{"type": "allowed_tools", "allowed_tools": map[string]any{"mode": mode, "tools": entries}}, nil
}
