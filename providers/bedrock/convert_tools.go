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
func prepareTools(tools []provider.Tool, toolChoice *provider.ToolChoice, modelID string, isAnthropic bool, disableParallelToolUse *bool) preparedTools {
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
		} else if t.Strict != nil && *t.Strict && !strictToolSchemaCompatible(spec.InputSchema.JSON) {
			res.warnings = append(res.warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "strict",
				Details: fmt.Sprintf("Tool '%s' has strict: true, but Amazon Bedrock requires every object in a strict tool schema to set additionalProperties: false. The strict property will be ignored.", t.Name),
			})
		} else {
			spec.Strict = t.Strict
		}
		tc.Tools = append(tc.Tools, toolDefinition{ToolSpec: spec})
	}

	choiceType := provider.ToolChoiceAuto
	if toolChoice != nil {
		choiceType = toolChoice.Type
	}
	usingAnthropicTools := isAnthropic && len(providerTools) > 0
	useAnthropicChoice := usingAnthropicTools ||
		(isAnthropic && disableParallelToolUse != nil && *disableParallelToolUse && len(tc.Tools) > 0 && choiceType != provider.ToolChoiceNone)
	if useAnthropicChoice {
		if choiceType != provider.ToolChoiceNone && (toolChoice != nil || (disableParallelToolUse != nil && *disableParallelToolUse)) {
			choice := map[string]any{"type": "auto"}
			switch choiceType {
			case provider.ToolChoiceRequired:
				choice["type"] = "any"
			case provider.ToolChoiceTool:
				choice["type"] = "tool"
				choice["name"] = toolChoice.ToolName
			}
			if disableParallelToolUse != nil {
				choice["disable_parallel_tool_use"] = *disableParallelToolUse
			}
			res.additionalTools = map[string]any{"tool_choice": choice}
		}
	} else if toolChoice != nil && len(tc.Tools) > 0 {
		switch choiceType {
		case provider.ToolChoiceAuto:
			tc.ToolChoice = &toolChoiceUnion{Auto: &struct{}{}}
		case provider.ToolChoiceRequired:
			tc.ToolChoice = &toolChoiceUnion{Any: &struct{}{}}
		case provider.ToolChoiceNone:
			tc.Tools = nil
		case provider.ToolChoiceTool:
			tc.ToolChoice = &toolChoiceUnion{Tool: &toolChoiceSpecificTool{Name: toolChoice.ToolName}}
		}
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

func withJSONResponseTool(tools []provider.Tool, schema json.RawMessage) []provider.Tool {
	return append(append([]provider.Tool{}, tools...), provider.Tool{
		Type:        provider.ToolTypeFunction,
		Name:        jsonResponseToolName,
		Description: "Respond with a JSON object.",
		InputSchema: schema,
	})
}
