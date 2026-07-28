package openai

import "github.com/grafana/ai-sdk/provider"

var providerToolNames = map[string]string{
	toolIDCodeInterpreter:  "code_interpreter",
	toolIDComputer:         "computer",
	toolIDFileSearch:       "file_search",
	toolIDImageGeneration:  "image_generation",
	toolIDLocalShell:       "local_shell",
	toolIDShell:            "shell",
	toolIDWebSearch:        "web_search",
	toolIDWebSearchPreview: "web_search_preview",
	toolIDMCP:              "mcp",
	toolIDApplyPatch:       "apply_patch",
	toolIDToolSearch:       "tool_search",
	toolIDProgrammatic:     "programmatic_tool_calling",
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
