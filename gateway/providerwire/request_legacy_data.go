package providerwire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type legacyDataContentType string

const (
	legacyDataContentData      legacyDataContentType = "data"
	legacyDataContentURL       legacyDataContentType = "url"
	legacyDataContentReference legacyDataContentType = "reference"
	legacyDataContentText      legacyDataContentType = "text"
)

type legacyDataContent struct {
	dataType  legacyDataContentType
	bytes     []byte
	base64    string
	url       string
	reference json.RawMessage
	text      string
}

func legacyDataContentTypeFromProvider(dataType provider.DataContentType) (legacyDataContentType, error) {
	switch dataType {
	case provider.DataContentTypeData:
		return legacyDataContentData, nil
	case provider.DataContentTypeURL:
		return legacyDataContentURL, nil
	case provider.DataContentTypeReference:
		return legacyDataContentReference, nil
	case provider.DataContentTypeText:
		return legacyDataContentText, nil
	default:
		return "", fmt.Errorf("unsupported selected file-data type %q", dataType)
	}
}

func resolveLegacyDataContentType(precedence legacyDataContentType, selected provider.DataContentType, selectedOK bool) (legacyDataContentType, error) {
	if selected == "" {
		return precedence, nil
	}
	mapped, err := legacyDataContentTypeFromProvider(selected)
	if err != nil {
		return "", err
	}
	if selectedOK && precedence == "" {
		return mapped, nil
	}
	if precedence == mapped {
		return precedence, nil
	}
	return "", fmt.Errorf("selected file-data type %q conflicts with legacy field precedence %q", mapped, precedence)
}

func legacyDataContentFromProvider(data provider.DataContent) (legacyDataContent, error) {
	legacy := legacyDataContent{
		bytes: append([]byte(nil), data.Bytes...), base64: data.Base64, url: data.URL,
		reference: append(json.RawMessage(nil), data.Reference...), text: data.Text,
	}
	switch {
	case data.Bytes != nil || data.Base64 != "":
		legacy.dataType = legacyDataContentData
	case data.URL != "":
		legacy.dataType = legacyDataContentURL
	case len(data.Reference) > 0:
		if !json.Valid(data.Reference) {
			return legacyDataContent{}, errors.New("invalid file-data reference JSON")
		}
		legacy.dataType = legacyDataContentReference
	case data.Text != "":
		legacy.dataType = legacyDataContentText
	}

	selected, selectedOK := data.DataType()
	resolved, err := resolveLegacyDataContentType(legacy.dataType, selected, selectedOK)
	if err != nil {
		return legacyDataContent{}, err
	}
	legacy.dataType = resolved
	return legacy, nil
}

func (data legacyDataContent) toProvider() (provider.DataContent, error) {
	switch data.dataType {
	case legacyDataContentData:
		if data.bytes != nil {
			return provider.BytesDataContent(data.bytes), nil
		}
		return provider.Base64DataContent(data.base64), nil
	case legacyDataContentURL:
		return provider.URLDataContent(data.url), nil
	case legacyDataContentReference:
		return provider.ReferenceDataContent(data.reference), nil
	case legacyDataContentText:
		return provider.TextDataContent(data.text), nil
	case "":
		return provider.DataContent{}, nil
	default:
		return provider.DataContent{}, fmt.Errorf("unsupported file-data type %q", data.dataType)
	}
}

func (data legacyDataContent) MarshalJSON() ([]byte, error) {
	switch data.dataType {
	case legacyDataContentData:
		value := data.base64
		if data.bytes != nil {
			value = base64.StdEncoding.EncodeToString(data.bytes)
		}
		return json.Marshal(struct {
			Type legacyDataContentType `json:"type"`
			Data string                `json:"data"`
		}{Type: data.dataType, Data: value})
	case legacyDataContentURL:
		return json.Marshal(struct {
			Type legacyDataContentType `json:"type"`
			URL  string                `json:"url"`
		}{Type: data.dataType, URL: data.url})
	case legacyDataContentReference:
		if !json.Valid(data.reference) {
			return nil, errors.New("invalid file-data reference JSON")
		}
		return json.Marshal(struct {
			Type      legacyDataContentType `json:"type"`
			Reference json.RawMessage       `json:"reference"`
		}{Type: data.dataType, Reference: data.reference})
	case legacyDataContentText:
		return json.Marshal(struct {
			Type legacyDataContentType `json:"type"`
			Text string                `json:"text"`
		}{Type: data.dataType, Text: data.text})
	case "":
		return []byte(`{}`), nil
	default:
		return nil, fmt.Errorf("unsupported file-data type %q", data.dataType)
	}
}

func (data *legacyDataContent) UnmarshalJSON(encoded []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return err
	}
	if rawType, ok := fields["type"]; ok {
		var dataType legacyDataContentType
		if err := json.Unmarshal(rawType, &dataType); err != nil {
			return err
		}
		*data = legacyDataContent{dataType: dataType}
		switch dataType {
		case legacyDataContentData:
			raw, ok := fields["data"]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return errors.New("file-data type data is required")
			}
			if err := json.Unmarshal(raw, &data.base64); err != nil {
				return err
			}
			if data.base64 == "" {
				data.bytes = []byte{}
			}
		case legacyDataContentURL:
			raw, ok := fields["url"]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return errors.New("file-data type url is required")
			}
			return json.Unmarshal(raw, &data.url)
		case legacyDataContentReference:
			raw, ok := fields["reference"]
			if !ok || !json.Valid(raw) {
				return errors.New("file-data type reference is required")
			}
			data.reference = append(json.RawMessage(nil), raw...)
		case legacyDataContentText:
			raw, ok := fields["text"]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return errors.New("file-data type text is required")
			}
			return json.Unmarshal(raw, &data.text)
		default:
			return fmt.Errorf("unsupported file-data type %q", dataType)
		}
		return nil
	}

	var legacy struct {
		Bytes     []byte          `json:"bytes"`
		Base64    string          `json:"base64"`
		URL       string          `json:"url"`
		Reference json.RawMessage `json:"reference"`
		Text      string          `json:"text"`
	}
	if err := json.Unmarshal(encoded, &legacy); err != nil {
		return err
	}
	mapped, err := legacyDataContentFromProvider(provider.DataContent{
		Bytes: legacy.Bytes, Base64: legacy.Base64, URL: legacy.URL, Reference: legacy.Reference, Text: legacy.Text,
	})
	if err != nil {
		return err
	}
	*data = mapped
	return nil
}
