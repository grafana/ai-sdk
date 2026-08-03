package anthropic

import "github.com/grafana/ai-sdk/provider"

var providerToolNames = map[string]string{
	"anthropic.web_search_20250305":        "web_search",
	"anthropic.tool_search_bm25_20251119":  "tool_search_tool_bm25",
	"anthropic.tool_search_regex_20251119": "tool_search_tool_regex",
	"anthropic.code_execution_20250522":    "code_execution",
	"anthropic.code_execution_20250825":    "code_execution",
	"anthropic.code_execution_20260120":    "code_execution",
	"anthropic.computer_20241022":          "computer",
	"anthropic.computer_20250124":          "computer",
	"anthropic.computer_20251124":          "computer",
	"anthropic.text_editor_20241022":       "str_replace_editor",
	"anthropic.text_editor_20250124":       "str_replace_editor",
	"anthropic.text_editor_20250429":       "str_replace_based_edit_tool",
	"anthropic.text_editor_20250728":       "str_replace_based_edit_tool",
	"anthropic.bash_20241022":              "bash",
	"anthropic.bash_20250124":              "bash",
	"anthropic.memory_20250818":            "memory",
	"anthropic.web_search_20260209":        "web_search",
	"anthropic.web_fetch_20250910":         "web_fetch",
	"anthropic.web_fetch_20260209":         "web_fetch",
	"anthropic.advisor_20260301":           "advisor",
}

type toolNameMapping struct {
	customToolNameToProviderToolName map[string]string
	providerToolNameToCustomToolName map[string]string
}

func newToolNameMapping(tools []provider.Tool) toolNameMapping {
	mapping := toolNameMapping{
		customToolNameToProviderToolName: make(map[string]string),
		providerToolNameToCustomToolName: make(map[string]string),
	}

	for _, tool := range tools {
		if tool.Type != provider.ToolTypeProvider {
			continue
		}

		providerToolName, ok := providerToolNames[tool.ID]
		if !ok {
			continue
		}

		mapping.customToolNameToProviderToolName[tool.Name] = providerToolName
		mapping.providerToolNameToCustomToolName[providerToolName] = tool.Name
	}

	return mapping
}

func (m toolNameMapping) toCustomToolName(providerToolName string) string {
	if customToolName, ok := m.providerToolNameToCustomToolName[providerToolName]; ok {
		return customToolName
	}

	return providerToolName
}

func (m toolNameMapping) toProviderToolName(customToolName string) string {
	if providerToolName, ok := m.customToolNameToProviderToolName[customToolName]; ok {
		return providerToolName
	}

	return customToolName
}

func resolveToolSearchProviderToolName(mapping toolNameMapping, serverToolCalls map[string]string, toolUseID string) string {
	if providerToolName, ok := serverToolCalls[toolUseID]; ok && providerToolName != "" {
		return providerToolName
	}

	bm25CustomName := mapping.toCustomToolName("tool_search_tool_bm25")
	regexCustomName := mapping.toCustomToolName("tool_search_tool_regex")

	if bm25CustomName != "tool_search_tool_bm25" {
		return "tool_search_tool_bm25"
	}
	if regexCustomName != "tool_search_tool_regex" {
		return "tool_search_tool_regex"
	}

	return "tool_search_tool_regex"
}
