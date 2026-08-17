package providerwirev4

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type dataDTO struct {
	Type      string          `json:"type"`
	Data      *string         `json:"data,omitempty"`
	URL       *string         `json:"url,omitempty"`
	Reference json.RawMessage `json:"reference,omitempty"`
	Text      *string         `json:"text,omitempty"`
}

func encodeData(data *provider.DataContent, allowReferenceText bool) (*dataDTO, error) {
	if data == nil {
		return nil, errors.New("providerwirev4: file data is required")
	}
	if err := data.Validate(); err != nil {
		if !allowReferenceText || data.Bytes != nil || data.Base64 != "" || data.URL != "" || len(data.Reference) != 0 || data.Text != "" {
			return nil, fmt.Errorf("providerwirev4: validating file data: %w", err)
		}
	}
	switch {
	case data.Bytes != nil || data.Base64 != "":
		value := data.Base64
		if data.Bytes != nil {
			value = base64.StdEncoding.EncodeToString(data.Bytes)
		}
		return &dataDTO{Type: "data", Data: &value}, nil
	case data.IsURL():
		if data.URL == "" {
			return nil, errors.New("providerwirev4: file data URL must not be empty")
		}
		value := data.URL
		return &dataDTO{Type: "url", URL: &value}, nil
	case len(data.Reference) > 0 && allowReferenceText:
		if err := validateProviderReference(data.Reference, "file reference"); err != nil {
			return nil, err
		}
		return &dataDTO{Type: "reference", Reference: append(json.RawMessage(nil), data.Reference...)}, nil
	case allowReferenceText:
		value := data.Text
		return &dataDTO{Type: "text", Text: &value}, nil
	default:
		return nil, errors.New("providerwirev4: file data variant is not representable")
	}
}

func decodeRequestData(raw json.RawMessage, allowReferenceText bool) (*provider.DataContent, error) {
	object, err := decodeObject(raw, "file data")
	if err != nil {
		return nil, err
	}
	variant, err := decodeRequiredString(object, "type", "file data")
	if err != nil {
		return nil, err
	}
	if err := rejectUnknownFields(object, "file data", "type", "data", "url", "reference", "text"); err != nil {
		return nil, err
	}
	switch variant {
	case "data":
		value, err := decodeRequiredString(object, "data", "file data")
		if err != nil {
			return nil, err
		}
		data := provider.Base64DataContent(value)
		return &data, nil
	case "url":
		value, err := decodeRequiredString(object, "url", "file data")
		if err != nil || value == "" {
			if err == nil {
				err = errors.New("providerwirev4: file data URL must not be empty")
			}
			return nil, err
		}
		return &provider.DataContent{URL: value}, nil
	case "reference":
		if !allowReferenceText {
			return nil, errors.New("providerwirev4: reference file data is not supported here")
		}
		value, err := requireField(object, "reference", "file data")
		if err != nil {
			return nil, err
		}
		if err := validateProviderReference(value, "file reference"); err != nil {
			return nil, err
		}
		return &provider.DataContent{Reference: append(json.RawMessage(nil), value...)}, nil
	case "text":
		if !allowReferenceText {
			return nil, errors.New("providerwirev4: text file data is not supported here")
		}
		value, err := decodeRequiredString(object, "text", "file data")
		if err != nil {
			return nil, err
		}
		return &provider.DataContent{Text: value}, nil
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported file data type %q", variant)
	}
}
