package providerwirev4

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type toolDTO struct {
	Type            string                      `json:"type"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description,omitempty"`
	InputSchema     json.RawMessage             `json:"inputSchema,omitempty"`
	InputExamples   []inputExampleDTO           `json:"inputExamples,omitempty"`
	Strict          *bool                       `json:"strict,omitempty"`
	ID              string                      `json:"id,omitempty"`
	Args            *map[string]json.RawMessage `json:"args,omitempty"`
	ProviderOptions providerOptionsDTO          `json:"providerOptions,omitempty"`
}

func (dto *toolDTO) UnmarshalJSON(data []byte) error {
	type toolAlias toolDTO
	object, err := decodeObject(data, "tool")
	if err != nil {
		return err
	}
	variant, err := decodeRequiredString(object, "type", "tool")
	if err != nil {
		return err
	}
	if err := rejectUnknownFields(object, "tool", "type", "name", "description", "inputSchema", "inputExamples", "strict", "id", "args", "providerOptions"); err != nil {
		return err
	}
	fields := []string{"type", "name"}
	switch provider.ToolType(variant) {
	case provider.ToolTypeFunction:
		fields = append(fields, "description", "inputSchema", "inputExamples", "strict", "providerOptions")
	case provider.ToolTypeProvider:
		if raw, exists := object["providerOptions"]; exists {
			if options, objectErr := decodeObject(raw, "inactive provider tool options"); objectErr == nil {
				if _, reserved := options["gateway"]; reserved {
					return errors.New("providerwirev4: provider tool must not contain reserved provider option \"gateway\"")
				}
			}
		}
		fields = append(fields, "id", "args")
	default:
		return fmt.Errorf("providerwirev4: unsupported tool type %q", variant)
	}
	if err := rejectNullFields(object, "tool", fields...); err != nil {
		return err
	}
	var decoded toolAlias
	if err := decodeSelectedObject(object, &decoded, fields...); err != nil {
		return err
	}
	*dto = toolDTO(decoded)
	return nil
}

type inputExampleDTO struct {
	Input json.RawMessage `json:"input"`
}

func (dto *inputExampleDTO) UnmarshalJSON(data []byte) error {
	type inputExampleAlias inputExampleDTO
	object, err := decodeObject(data, "tool input example")
	if err != nil {
		return err
	}
	if err := rejectUnknownFields(object, "tool input example", "input"); err != nil {
		return err
	}
	return decodeSelectedObject(object, (*inputExampleAlias)(dto), "input")
}

func encodeTool(tool provider.Tool) (toolDTO, error) {
	dto := toolDTO{Type: string(tool.Type), Name: tool.Name}
	switch tool.Type {
	case provider.ToolTypeFunction:
		providerOptions, err := encodeNestedProviderOptions(tool.ProviderOptions, "tool")
		if err != nil {
			return toolDTO{}, err
		}
		dto.Description, dto.Strict, dto.ProviderOptions = tool.Description, tool.Strict, providerOptions
		if tool.Name == "" {
			return toolDTO{}, errors.New("providerwirev4: function tool name is required")
		}
		if err := validateJSONObject(tool.InputSchema, "function tool input schema"); err != nil {
			return toolDTO{}, err
		}
		dto.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
		dto.InputExamples = make([]inputExampleDTO, len(tool.InputExamples))
		for i, example := range tool.InputExamples {
			if _, err := decodeObject(example.Input, "tool input example"); err != nil {
				return toolDTO{}, err
			}
			dto.InputExamples[i] = inputExampleDTO{Input: append(json.RawMessage(nil), example.Input...)}
		}
	case provider.ToolTypeProvider:
		if len(tool.ProviderOptions) > 0 {
			return toolDTO{}, errors.New("providerwirev4: provider tool providerOptions are not in LanguageModelV4")
		}
		if tool.Name == "" {
			return toolDTO{}, errors.New("providerwirev4: provider tool name is required")
		}
		if err := validateQualifiedIdentifier(tool.ID, "provider tool ID"); err != nil {
			return toolDTO{}, err
		}
		if tool.Args == nil {
			return toolDTO{}, errors.New("providerwirev4: provider tool args object is required")
		}
		dto.ID = tool.ID
		args := make(map[string]json.RawMessage, len(tool.Args))
		for key, value := range tool.Args {
			if err := validateJSON(value, fmt.Sprintf("provider tool argument %q", key)); err != nil {
				return toolDTO{}, err
			}
			args[key] = append(json.RawMessage(nil), value...)
		}
		dto.Args = &args
	default:
		return toolDTO{}, fmt.Errorf("providerwirev4: unsupported tool type %q", tool.Type)
	}
	return dto, nil
}

func decodeTool(dto toolDTO) (provider.Tool, error) {
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool")
	if err != nil {
		return provider.Tool{}, err
	}
	tool := provider.Tool{Type: provider.ToolType(dto.Type), Name: dto.Name}
	switch tool.Type {
	case provider.ToolTypeFunction:
		tool.Description, tool.Strict, tool.ProviderOptions = dto.Description, dto.Strict, providerOptions
		if tool.Name == "" {
			return provider.Tool{}, errors.New("providerwirev4: function tool name is required")
		}
		if err := validateJSONObject(dto.InputSchema, "function tool input schema"); err != nil {
			return provider.Tool{}, err
		}
		tool.InputSchema = append(json.RawMessage(nil), dto.InputSchema...)
		tool.InputExamples = make([]provider.InputExample, len(dto.InputExamples))
		for i, example := range dto.InputExamples {
			if _, err := decodeObject(example.Input, "tool input example"); err != nil {
				return provider.Tool{}, err
			}
			tool.InputExamples[i] = provider.InputExample{Input: append(json.RawMessage(nil), example.Input...)}
		}
	case provider.ToolTypeProvider:
		tool.ID = dto.ID
		if tool.Name == "" {
			return provider.Tool{}, errors.New("providerwirev4: provider tool name is required")
		}
		if err := validateQualifiedIdentifier(tool.ID, "provider tool ID"); err != nil {
			return provider.Tool{}, err
		}
		if dto.Args == nil {
			return provider.Tool{}, errors.New("providerwirev4: provider tool args object is required")
		}
		tool.Args = make(map[string]json.RawMessage, len(*dto.Args))
		for key, value := range *dto.Args {
			if err := validateJSON(value, fmt.Sprintf("provider tool argument %q", key)); err != nil {
				return provider.Tool{}, err
			}
			tool.Args[key] = append(json.RawMessage(nil), value...)
		}
	default:
		return provider.Tool{}, fmt.Errorf("providerwirev4: unsupported tool type %q", dto.Type)
	}
	return tool, nil
}
