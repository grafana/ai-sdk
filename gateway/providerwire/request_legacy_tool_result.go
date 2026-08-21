package providerwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type legacyToolResultOutput struct {
	Type            provider.ToolResultOutputType
	Text            string
	JSON            json.RawMessage
	Content         []legacyToolResultContentValue
	Reason          *string
	ProviderOptions map[string]json.RawMessage
}

type legacyToolResultContentValue struct {
	Type            provider.ToolResultContentType
	Text            string
	Data            *legacyDataContent
	MediaType       string
	Filename        *string
	ProviderOptions map[string]json.RawMessage
}

func legacyToolResultOutputFromProvider(output provider.ToolResultOutput) (legacyToolResultOutput, error) {
	options, err := legacyProviderOptionsFromProvider(output.ProviderOptions)
	if err != nil {
		return legacyToolResultOutput{}, err
	}
	legacy := legacyToolResultOutput{
		Type: output.Type, Text: output.Text, JSON: append(json.RawMessage(nil), output.JSON...),
		Reason: clonePointer(output.Reason), ProviderOptions: options,
	}
	if output.Content != nil {
		legacy.Content = make([]legacyToolResultContentValue, len(output.Content))
		for index, content := range output.Content {
			mapped, err := legacyToolResultContentFromProvider(content)
			if err != nil {
				return legacyToolResultOutput{}, fmt.Errorf("content %d: %w", index, err)
			}
			legacy.Content[index] = mapped
		}
	}
	return legacy, nil
}

func (output legacyToolResultOutput) toProvider() (provider.ToolResultOutput, error) {
	mapped := provider.ToolResultOutput{
		Type: output.Type, Text: output.Text, JSON: append(json.RawMessage(nil), output.JSON...),
		Reason: clonePointer(output.Reason), ProviderOptions: legacyProviderOptionsToProvider(output.ProviderOptions),
	}
	if output.Content != nil {
		mapped.Content = make([]provider.ToolResultContentValue, len(output.Content))
		for index, content := range output.Content {
			value, err := content.toProvider()
			if err != nil {
				return provider.ToolResultOutput{}, fmt.Errorf("content %d: %w", index, err)
			}
			mapped.Content[index] = value
		}
	}
	return mapped, nil
}

func (output legacyToolResultOutput) MarshalJSON() ([]byte, error) {
	fields := map[string]json.RawMessage{}
	typeJSON, err := json.Marshal(output.Type)
	if err != nil {
		return nil, err
	}
	fields["type"] = typeJSON
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		fields["value"], err = json.Marshal(output.Text)
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if len(output.JSON) == 0 {
			fields["value"] = json.RawMessage("null")
		} else if json.Valid(output.JSON) {
			fields["value"] = output.JSON
		} else {
			err = errors.New("invalid tool-result JSON value")
		}
	case provider.ToolOutputContent:
		fields["value"], err = json.Marshal(output.Content)
	case provider.ToolOutputExecutionDenied:
		if output.Reason != nil {
			fields["reason"], err = json.Marshal(*output.Reason)
		}
	default:
		return nil, fmt.Errorf("unsupported tool-result output type %q", output.Type)
	}
	if err != nil {
		return nil, err
	}
	if len(output.ProviderOptions) > 0 {
		fields["providerOptions"], err = json.Marshal(output.ProviderOptions)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

func (output *legacyToolResultOutput) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var outputType provider.ToolResultOutputType
	if err := json.Unmarshal(fields["type"], &outputType); err != nil {
		return err
	}
	*output = legacyToolResultOutput{Type: outputType}
	if raw := fields["providerOptions"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &output.ProviderOptions); err != nil {
			return err
		}
	}
	value, hasValue := fields["value"]
	if hasValue {
		if outputType == provider.ToolOutputExecutionDenied {
			return errors.New("execution-denied tool result must not contain value")
		}
		switch outputType {
		case provider.ToolOutputText, provider.ToolOutputErrorText:
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("tool-result text value is required")
			}
			return json.Unmarshal(value, &output.Text)
		case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
			output.JSON = append(json.RawMessage(nil), value...)
			return nil
		case provider.ToolOutputContent:
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("tool-result content value is required")
			}
			return json.Unmarshal(value, &output.Content)
		default:
			return fmt.Errorf("unsupported tool-result output type %q", outputType)
		}
	}
	switch outputType {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		raw, ok := fields["text"]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("legacy tool-result text is required")
		}
		return json.Unmarshal(raw, &output.Text)
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		raw, ok := fields["json"]
		if !ok {
			return errors.New("legacy tool-result JSON is required")
		}
		output.JSON = append(json.RawMessage(nil), raw...)
	case provider.ToolOutputContent:
		raw, ok := fields["content"]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("legacy tool-result content is required")
		}
		return json.Unmarshal(raw, &output.Content)
	case provider.ToolOutputExecutionDenied:
		if raw, ok := fields["reason"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			var reason string
			if err := json.Unmarshal(raw, &reason); err != nil {
				return err
			}
			output.Reason = &reason
		}
	default:
		return fmt.Errorf("unsupported tool-result output type %q", outputType)
	}
	return nil
}

func legacyToolResultContentFromProvider(content provider.ToolResultContentValue) (legacyToolResultContentValue, error) {
	options, err := legacyProviderOptionsFromProvider(content.ProviderOptions)
	if err != nil {
		return legacyToolResultContentValue{}, err
	}
	legacy := legacyToolResultContentValue{
		Type: content.Type, Text: content.Text, MediaType: content.MediaType,
		Filename: clonePointer(content.Filename), ProviderOptions: options,
	}
	if content.Data != nil {
		data, err := legacyDataContentFromProvider(*content.Data)
		if err != nil {
			return legacyToolResultContentValue{}, err
		}
		legacy.Data = &data
	}
	return legacy, nil
}

func (content legacyToolResultContentValue) toProvider() (provider.ToolResultContentValue, error) {
	mapped := provider.ToolResultContentValue{
		Type: content.Type, Text: content.Text, MediaType: content.MediaType,
		Filename: clonePointer(content.Filename), ProviderOptions: legacyProviderOptionsToProvider(content.ProviderOptions),
	}
	if content.Data != nil {
		data, err := content.Data.toProvider()
		if err != nil {
			return provider.ToolResultContentValue{}, err
		}
		mapped.Data = &data
	}
	return mapped, nil
}

func (content legacyToolResultContentValue) MarshalJSON() ([]byte, error) {
	switch content.Type {
	case provider.ToolContentText:
		return json.Marshal(struct {
			Type            provider.ToolResultContentType `json:"type"`
			Text            string                         `json:"text"`
			ProviderOptions map[string]json.RawMessage     `json:"providerOptions,omitempty"`
		}{Type: provider.ToolContentText, Text: content.Text, ProviderOptions: content.ProviderOptions})
	case provider.ToolContentFile, provider.ToolContentFileData, provider.ToolContentFileURL, provider.ToolContentFileReference:
		if content.Data == nil {
			return nil, errors.New("tool-result file content data is required")
		}
		return json.Marshal(struct {
			Type            provider.ToolResultContentType `json:"type"`
			Data            *legacyDataContent             `json:"data"`
			MediaType       string                         `json:"mediaType"`
			Filename        *string                        `json:"filename,omitempty"`
			ProviderOptions map[string]json.RawMessage     `json:"providerOptions,omitempty"`
		}{
			Type: provider.ToolContentFile, Data: content.Data, MediaType: content.MediaType,
			Filename: content.Filename, ProviderOptions: content.ProviderOptions,
		})
	case provider.ToolContentCustom:
		return json.Marshal(struct {
			Type            provider.ToolResultContentType `json:"type"`
			ProviderOptions map[string]json.RawMessage     `json:"providerOptions,omitempty"`
		}{Type: provider.ToolContentCustom, ProviderOptions: content.ProviderOptions})
	default:
		return nil, fmt.Errorf("unsupported tool-result content type %q", content.Type)
	}
}

func (content *legacyToolResultContentValue) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var contentType provider.ToolResultContentType
	if err := json.Unmarshal(fields["type"], &contentType); err != nil {
		return err
	}
	content.Type = contentType
	if raw := fields["providerOptions"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &content.ProviderOptions); err != nil {
			return err
		}
	}
	if raw := fields["filename"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var filename string
		if err := json.Unmarshal(raw, &filename); err != nil {
			return err
		}
		content.Filename = &filename
	}
	switch contentType {
	case provider.ToolContentText:
		raw, ok := fields["text"]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("tool-result text content text is required")
		}
		return json.Unmarshal(raw, &content.Text)
	case provider.ToolContentFile:
		if err := json.Unmarshal(fields["mediaType"], &content.MediaType); err != nil {
			return errors.New("tool-result file mediaType is required")
		}
		raw, ok := fields["data"]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("tool-result file data is required")
		}
		var mapped legacyDataContent
		if err := json.Unmarshal(raw, &mapped); err != nil {
			return err
		}
		content.Data = &mapped
	case provider.ToolContentFileData:
		if err := json.Unmarshal(fields["mediaType"], &content.MediaType); err != nil {
			return errors.New("legacy tool-result file mediaType is required")
		}
		var encoded string
		if err := json.Unmarshal(fields["data"], &encoded); err != nil {
			return errors.New("legacy tool-result file data is required")
		}
		mapped, _ := legacyDataContentFromProvider(provider.Base64DataContent(encoded))
		content.Type = provider.ToolContentFile
		content.Data = &mapped
	case provider.ToolContentFileURL:
		var url string
		if err := json.Unmarshal(fields["url"], &url); err != nil {
			return errors.New("legacy tool-result file URL is required")
		}
		mapped, _ := legacyDataContentFromProvider(provider.URLDataContent(url))
		content.Type = provider.ToolContentFile
		content.Data = &mapped
	case provider.ToolContentFileReference:
		raw, ok := fields["providerReference"]
		if !ok || !json.Valid(raw) {
			return errors.New("legacy tool-result providerReference is required")
		}
		mapped, err := legacyDataContentFromProvider(provider.ReferenceDataContent(raw))
		if err != nil {
			return err
		}
		content.Type = provider.ToolContentFile
		content.Data = &mapped
	case provider.ToolContentCustom:
	default:
		return fmt.Errorf("unsupported tool-result content type %q", contentType)
	}
	return nil
}
