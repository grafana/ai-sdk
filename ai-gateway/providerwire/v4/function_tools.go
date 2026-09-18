package v4

import (
	"bytes"
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

type wireFunctionTool struct {
	Type            provider.ToolType          `json:"type"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description"`
	InputSchema     json.RawMessage            `json:"inputSchema"`
	InputExamples   []wireInputExample         `json:"inputExamples"`
	Strict          *bool                      `json:"strict"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wireInputExample struct {
	Input json.RawMessage `json:"input"`
}
type wireToolChoice struct {
	Type     provider.ToolChoiceType `json:"type"`
	ToolName string                  `json:"toolName"`
}
type wireToolOutput struct {
	Type            provider.ToolResultOutputType `json:"type"`
	Value           json.RawMessage               `json:"value"`
	ProviderOptions map[string]json.RawMessage    `json:"providerOptions"`
}

func mapFunctionTools(rawTools []json.RawMessage, rawChoice json.RawMessage) ([]provider.Tool, *provider.ToolChoice, *requestFailure) {
	tools := make([]provider.Tool, 0, len(rawTools))
	for _, raw := range rawTools {
		var wire wireFunctionTool
		if json.Unmarshal(raw, &wire) != nil {
			return nil, nil, invalidMappingFailure()
		}
		if wire.Type != provider.ToolTypeFunction {
			return nil, nil, unsupportedMappingFailure(capabilityTools)
		}
		if trimmed := bytes.TrimSpace(wire.InputSchema); len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, nil, unsupportedMappingFailure(capabilityTools)
		}
		tool := provider.Tool{Type: wire.Type, Name: wire.Name, Description: wire.Description, InputSchema: wire.InputSchema, Strict: wire.Strict}
		if wire.InputExamples != nil {
			tool.InputExamples = make([]provider.InputExample, 0, len(wire.InputExamples))
			for _, example := range wire.InputExamples {
				tool.InputExamples = append(tool.InputExamples, provider.InputExample{Input: example.Input})
			}
		}
		if wire.ProviderOptions != nil {
			tool.ProviderOptions = make(provider.ProviderOptions, len(wire.ProviderOptions))
			for namespace, raw := range wire.ProviderOptions {
				if namespace == "gateway" || namespace == "grafana" || namespace == "grafana-ai-sdk" {
					return nil, nil, unsupportedMappingFailure(capabilityProviderOptions)
				}
				tool.ProviderOptions[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
			}
		}
		tools = append(tools, tool)
	}
	var choice *provider.ToolChoice
	if len(rawChoice) != 0 {
		var wire wireToolChoice
		if json.Unmarshal(rawChoice, &wire) != nil {
			return nil, nil, invalidMappingFailure()
		}
		switch wire.Type {
		case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired, provider.ToolChoiceTool:
		default:
			return nil, nil, invalidMappingFailure()
		}
		choice = &provider.ToolChoice{Type: wire.Type, ToolName: wire.ToolName}
	}
	return tools, choice, nil
}

func mapToolOutput(wire *wireToolOutput) (*provider.ToolResultOutput, *requestFailure) {
	if wire == nil {
		return nil, invalidMappingFailure()
	}
	if !providerOptionsEmpty(wire.ProviderOptions) {
		return nil, unsupportedMappingFailure(capabilityProviderOptions)
	}
	output := &provider.ToolResultOutput{Type: wire.Type}
	switch wire.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		if json.Unmarshal(wire.Value, &output.Text) != nil {
			return nil, invalidMappingFailure()
		}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		output.JSON = wire.Value
	case provider.ToolOutputContent:
		var parts []wirePart
		if json.Unmarshal(wire.Value, &parts) != nil {
			return nil, invalidMappingFailure()
		}
		output.Content = make([]provider.ToolResultContentValue, 0, len(parts))
		for _, part := range parts {
			if part.Type != provider.ContentPartTypeText {
				return nil, unsupportedMappingFailure(capabilityTools)
			}
			if !providerOptionsEmpty(part.ProviderOptions) {
				return nil, unsupportedMappingFailure(capabilityProviderOptions)
			}
			output.Content = append(output.Content, provider.ToolResultContentValue{Type: provider.ToolContentText, Text: part.Text})
		}
	default:
		return nil, unsupportedMappingFailure(capabilityTools)
	}
	return output, nil
}
