package providerwirev4

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type toolResultOutputDTO struct {
	Type            string             `json:"type"`
	Value           json.RawMessage    `json:"value,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	ProviderOptions providerOptionsDTO `json:"providerOptions,omitempty"`
}

type toolResultContentDTO struct {
	Type            string             `json:"type"`
	Text            *string            `json:"text,omitempty"`
	Data            json.RawMessage    `json:"data,omitempty"`
	MediaType       *string            `json:"mediaType,omitempty"`
	Filename        string             `json:"filename,omitempty"`
	ProviderOptions providerOptionsDTO `json:"providerOptions,omitempty"`
}

func encodeToolResultOutput(output provider.ToolResultOutput) (toolResultOutputDTO, error) {
	if output.Type == provider.ToolOutputContent && len(output.ProviderOptions) > 0 {
		return toolResultOutputDTO{}, errors.New("providerwirev4: content tool result output providerOptions are not in LanguageModelV4")
	}
	providerOptions, err := encodeNestedProviderOptions(output.ProviderOptions, "tool result output")
	if err != nil {
		return toolResultOutputDTO{}, err
	}
	dto := toolResultOutputDTO{Type: string(output.Type), ProviderOptions: providerOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		dto.Value, err = json.Marshal(output.Text)
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if err := validateJSON(output.JSON, "tool result JSON value"); err != nil {
			return toolResultOutputDTO{}, err
		}
		dto.Value = append(json.RawMessage(nil), output.JSON...)
	case provider.ToolOutputContent:
		if output.Content == nil {
			return toolResultOutputDTO{}, errors.New("providerwirev4: content tool result value is required")
		}
		content := make([]toolResultContentDTO, len(output.Content))
		for i, value := range output.Content {
			content[i], err = encodeToolResultContent(value)
			if err != nil {
				return toolResultOutputDTO{}, err
			}
		}
		dto.Value, err = json.Marshal(content)
	case provider.ToolOutputExecutionDenied:
		dto.Reason = output.Reason
	default:
		return toolResultOutputDTO{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", output.Type)
	}
	return dto, err
}

func decodeToolResultOutput(data json.RawMessage) (provider.ToolResultOutput, error) {
	object, err := decodeObject(data, "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	variant, err := decodeRequiredString(object, "type", "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	if _, legacy := object["text"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	if _, legacy := object["json"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	if _, legacy := object["content"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	if err := rejectUnknownFields(object, "tool result output", "type", "value", "reason", "providerOptions"); err != nil {
		return provider.ToolResultOutput{}, err
	}
	fields := []string{"type"}
	outputType := provider.ToolResultOutputType(variant)
	switch outputType {
	case provider.ToolOutputText, provider.ToolOutputErrorText, provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		fields = append(fields, "value", "providerOptions")
	case provider.ToolOutputContent:
		fields = append(fields, "value")
		if raw, exists := object["providerOptions"]; exists {
			if options, objectErr := decodeObject(raw, "inactive content tool result provider options"); objectErr == nil {
				if _, reserved := options["gateway"]; reserved {
					return provider.ToolResultOutput{}, errors.New("providerwirev4: tool result output must not contain reserved provider option \"gateway\"")
				}
			}
		}
	case provider.ToolOutputExecutionDenied:
		fields = append(fields, "reason", "providerOptions")
	default:
		return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", variant)
	}
	if outputType != provider.ToolOutputContent {
		nonNullFields := []string{"providerOptions"}
		if outputType == provider.ToolOutputExecutionDenied {
			nonNullFields = append(nonNullFields, "reason")
		}
		if err := rejectNullFields(object, "tool result output", nonNullFields...); err != nil {
			return provider.ToolResultOutput{}, err
		}
	}
	var dto toolResultOutputDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.ToolResultOutput{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	output := provider.ToolResultOutput{Type: outputType, Reason: dto.Reason, ProviderOptions: providerOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		value, err := requireField(object, "value", "tool result output")
		if err != nil || json.Unmarshal(value, &output.Text) != nil {
			return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: tool result %q value must be a string", variant)
		}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		value, err := requireJSONField(object, "value", "tool result output")
		if err != nil {
			return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: tool result %q value is required", variant)
		}
		output.JSON = append(json.RawMessage(nil), value...)
	case provider.ToolOutputContent:
		value, err := requireField(object, "value", "tool result output")
		if err != nil {
			return provider.ToolResultOutput{}, err
		}
		var content []json.RawMessage
		if err := json.Unmarshal(value, &content); err != nil || content == nil {
			return provider.ToolResultOutput{}, errors.New("providerwirev4: tool result content value must be an array")
		}
		output.Content = make([]provider.ToolResultContentValue, len(content))
		for i, item := range content {
			decoded, err := decodeToolResultContent(item)
			if err != nil {
				return provider.ToolResultOutput{}, err
			}
			output.Content[i] = decoded
		}
	case provider.ToolOutputExecutionDenied:
	default:
		return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", variant)
	}
	return output, nil
}

func encodeToolResultContent(content provider.ToolResultContentValue) (toolResultContentDTO, error) {
	providerOptions, err := encodeNestedProviderOptions(content.ProviderOptions, "tool result content")
	if err != nil {
		return toolResultContentDTO{}, err
	}
	dto := toolResultContentDTO{Type: string(content.Type), ProviderOptions: providerOptions}
	switch content.Type {
	case provider.ToolContentText:
		dto.Text = &content.Text
	case provider.ToolContentFile:
		data, err := encodeData(content.Data, true)
		if err != nil {
			return toolResultContentDTO{}, err
		}
		dto.Data, err = json.Marshal(data)
		if err != nil {
			return toolResultContentDTO{}, err
		}
		dto.MediaType, dto.Filename = &content.MediaType, content.Filename
	case provider.ToolContentCustom:
	default:
		return toolResultContentDTO{}, fmt.Errorf("providerwirev4: unsupported canonical tool result content type %q", content.Type)
	}
	return dto, nil
}

func decodeToolResultContent(data json.RawMessage) (provider.ToolResultContentValue, error) {
	object, err := decodeObject(data, "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	variant, err := decodeRequiredString(object, "type", "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	if err := rejectUnknownFields(object, "tool result content", "type", "text", "data", "mediaType", "filename", "providerOptions"); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	fields := []string{"type", "providerOptions"}
	switch provider.ToolResultContentType(variant) {
	case provider.ToolContentText:
		fields = append(fields, "text")
	case provider.ToolContentFile:
		fields = append(fields, "data", "mediaType", "filename")
	case provider.ToolContentCustom:
	case provider.ToolContentFileData, provider.ToolContentFileURL, provider.ToolContentFileReference:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: legacy tool result content type %q is not supported", variant)
	default:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: unsupported tool result content type %q", variant)
	}
	if err := rejectNullFields(object, "tool result content", fields...); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	var dto toolResultContentDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	content := provider.ToolResultContentValue{Type: provider.ToolResultContentType(variant), ProviderOptions: providerOptions}
	switch content.Type {
	case provider.ToolContentText:
		if dto.Text == nil {
			return provider.ToolResultContentValue{}, errors.New("providerwirev4: tool result content text is required")
		}
		content.Text = *dto.Text
	case provider.ToolContentFile:
		if len(dto.Data) == 0 || dto.MediaType == nil {
			return provider.ToolResultContentValue{}, errors.New("providerwirev4: tool result file data and mediaType are required")
		}
		content.Data, err = decodeRequestData(dto.Data, true)
		if err != nil {
			return provider.ToolResultContentValue{}, err
		}
		content.MediaType, content.Filename = *dto.MediaType, dto.Filename
	case provider.ToolContentCustom:
	case provider.ToolContentFileData, provider.ToolContentFileURL, provider.ToolContentFileReference:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: legacy tool result content type %q is not supported", variant)
	default:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: unsupported tool result content type %q", variant)
	}
	return content, nil
}
