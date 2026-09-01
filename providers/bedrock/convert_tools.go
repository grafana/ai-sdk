package bedrock

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

const (
	// jsonResponseToolName is the synthetic tool injected when a non-native
	// structured-output model receives a JSON ResponseFormat. The tool
	// signals to the model "respond by calling json with your structured
	// output". Mirrors upstream behavior.
	jsonResponseToolName = "json"

	// unsupportedWebSearchToolID is the upstream Anthropic web_search tool
	// that Bedrock does not support. We filter it out with a warning.
	unsupportedWebSearchToolID = "anthropic.web_search_20250305"
)

// preparedTools is the output of [prepareTools]: the toolConfig sent in the
// request, an optional additionalTools map (Anthropic provider tools route
// their tool_choice through `additionalModelRequestFields`), the union of
// beta flags needed by selected tools, and any warnings.
type preparedTools struct {
	toolConfig      *toolConfig
	additionalTools map[string]any
	betas           map[string]struct{}
	warnings        []provider.Warning
}

// prepareTools converts the caller's `tools` and `toolChoice` into Bedrock
// `toolConfig`. Anthropic provider-tool support on Bedrock is limited and
// requires routing the `tool_choice` through `additionalModelRequestFields`
// while still describing the tools in `toolConfig.tools` for validation.
// Non-Anthropic provider tools are reported as unsupported.
func prepareTools(tools []provider.Tool, toolChoice *provider.ToolChoice, modelID string, disableParallelToolUse bool) preparedTools {
	res := preparedTools{
		betas: map[string]struct{}{},
	}
	if len(tools) == 0 {
		return res
	}

	// Filter out unsupported provider tools and emit warnings.
	supported := make([]provider.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Type == provider.ToolTypeProvider && t.ID == unsupportedWebSearchToolID {
			res.warnings = append(res.warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "web_search_20250305 tool",
				Details: "The web_search_20250305 tool is not supported on Amazon Bedrock.",
			})
			continue
		}
		supported = append(supported, t)
	}
	if len(supported) == 0 {
		return res
	}

	isAnthropic := isAnthropicModel(modelID)

	providerTools := make([]provider.Tool, 0)
	functionTools := make([]provider.Tool, 0)
	for _, t := range supported {
		switch t.Type {
		case provider.ToolTypeProvider:
			providerTools = append(providerTools, t)
		case provider.ToolTypeFunction:
			functionTools = append(functionTools, t)
		}
	}

	tc := &toolConfig{}

	// Anthropic-on-Bedrock provider tools: tool_choice goes through
	// additionalModelRequestFields; the tool itself is described in
	// toolConfig.tools with its inputSchema.
	if isAnthropic && len(providerTools) > 0 {
		// We accept the caller's provider tools by name + inputSchema. Beta
		// flags are propagated unconditionally for now; per-tool beta
		// catalogues can be added later as recorded conformance fixtures
		// exercise specific Anthropic tools on Bedrock.
		for _, t := range providerTools {
			tc.Tools = append(tc.Tools, toolDefinition{
				ToolSpec: &toolSpec{
					Name:        t.Name,
					InputSchema: toolInputSchema{JSON: jsonOrEmptyObject(t.InputSchema)},
				},
			})
		}
	} else {
		for _, t := range providerTools {
			res.warnings = append(res.warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: fmt.Sprintf("tool %s", t.ID),
				Details: "Bedrock does not support this provider tool for the configured model.",
			})
		}
	}

	// Function tools: filter to a single tool when toolChoice targets one,
	// matching upstream's behavior of pruning the tool list for a forced
	// choice.
	filteredFunctionTools := functionTools
	if toolChoice != nil && toolChoice.Type == provider.ToolChoiceTool {
		filteredFunctionTools = filteredFunctionTools[:0]
		for _, t := range functionTools {
			if t.Name == toolChoice.ToolName {
				filteredFunctionTools = append(filteredFunctionTools, t)
			}
		}
	}
	for _, t := range filteredFunctionTools {
		spec := &toolSpec{
			Name:        t.Name,
			InputSchema: toolInputSchema{JSON: jsonOrEmptyObject(t.InputSchema)},
		}
		if desc := strings.TrimSpace(t.Description); desc != "" {
			spec.Description = desc
		}
		if rejectsNewerSchemaFields(modelID) {
			if t.Strict != nil {
				res.warnings = append(res.warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "strict",
					Details: fmt.Sprintf("Tool '%s' has strict: %t, but strict mode is not supported by this model on Amazon Bedrock. The strict property will be ignored.", t.Name, *t.Strict),
				})
			}
		} else {
			spec.Strict = t.Strict
		}
		tc.Tools = append(tc.Tools, toolDefinition{ToolSpec: spec})
	}

	// Tool choice translation. For Anthropic-on-Bedrock provider tools the
	// choice rides on additionalModelRequestFields; otherwise it goes into
	// toolConfig.toolChoice.
	if toolChoice != nil {
		if isAnthropic && len(providerTools) > 0 {
			res.additionalTools = map[string]any{}
			switch toolChoice.Type {
			case provider.ToolChoiceAuto:
				res.additionalTools["tool_choice"] = map[string]any{"type": "auto"}
			case provider.ToolChoiceRequired:
				res.additionalTools["tool_choice"] = map[string]any{"type": "any"}
			case provider.ToolChoiceNone:
				// "none" maps to no tool choice in Anthropic's model.
				// Drop tools entirely to match upstream behavior.
				tc.Tools = nil
			case provider.ToolChoiceTool:
				res.additionalTools["tool_choice"] = map[string]any{"type": "tool", "name": toolChoice.ToolName}
			}
		} else if len(tc.Tools) > 0 {
			switch toolChoice.Type {
			case provider.ToolChoiceAuto:
				tc.ToolChoice = &toolChoiceUnion{Auto: &struct{}{}}
			case provider.ToolChoiceRequired:
				tc.ToolChoice = &toolChoiceUnion{Any: &struct{}{}}
			case provider.ToolChoiceNone:
				// Drop tools; Bedrock has no explicit "none" choice marker.
				tc.Tools = nil
				tc.ToolChoice = nil
			case provider.ToolChoiceTool:
				tc.ToolChoice = &toolChoiceUnion{Tool: &toolChoiceSpecificTool{Name: toolChoice.ToolName}}
			}
		}
	}

	if isAnthropic && len(providerTools) == 0 && disableParallelToolUse && len(tc.Tools) > 0 && (toolChoice == nil || toolChoice.Type != provider.ToolChoiceNone) {
		choice := map[string]any{"type": "auto", "disable_parallel_tool_use": true}
		if toolChoice != nil {
			switch toolChoice.Type {
			case provider.ToolChoiceRequired:
				choice["type"] = "any"
			case provider.ToolChoiceTool:
				choice["type"] = "tool"
				choice["name"] = toolChoice.ToolName
			}
		}
		if res.additionalTools == nil {
			res.additionalTools = map[string]any{}
		}
		res.additionalTools["tool_choice"] = choice
		tc.ToolChoice = nil
	}

	if len(tc.Tools) > 0 || tc.ToolChoice != nil {
		res.toolConfig = tc
	}
	return res
}

func jsonOrEmptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return raw
}

// injectJSONResponseTool appends the synthetic `json` tool to the configured
// tool set and forces `toolChoice = required` (any). Returns the updated
// preparedTools so callers can chain it with [prepareTools] when a JSON
// response format is requested and the model doesn't support native
// structured output.
func injectJSONResponseTool(pt preparedTools, schema json.RawMessage) preparedTools {
	if pt.toolConfig == nil {
		pt.toolConfig = &toolConfig{}
	}
	pt.toolConfig.Tools = append(pt.toolConfig.Tools, toolDefinition{
		ToolSpec: &toolSpec{
			Name:        jsonResponseToolName,
			Description: "Respond with a JSON object.",
			InputSchema: toolInputSchema{JSON: jsonOrEmptyObject(schema)},
		},
	})
	pt.toolConfig.ToolChoice = &toolChoiceUnion{Any: &struct{}{}}
	return pt
}
