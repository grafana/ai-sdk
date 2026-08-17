package providerwirev4

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type providerOptionsDTO map[string]json.RawMessage

type dataDTO struct {
	Type      string          `json:"type"`
	Data      *string         `json:"data,omitempty"`
	URL       *string         `json:"url,omitempty"`
	Reference json.RawMessage `json:"reference,omitempty"`
	Text      *string         `json:"text,omitempty"`
}

func decodeObject(data []byte, context string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("providerwirev4: decoding %s: %w", context, err)
	}
	if object == nil {
		return nil, fmt.Errorf("providerwirev4: %s must be an object", context)
	}
	return object, nil
}

func requireField(object map[string]json.RawMessage, name, context string) (json.RawMessage, error) {
	value, err := requireJSONField(object, name, context)
	if err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, fmt.Errorf("providerwirev4: %s field %q is required", context, name)
	}
	return value, nil
}

func requireJSONField(object map[string]json.RawMessage, name, context string) (json.RawMessage, error) {
	value, ok := object[name]
	if !ok || len(bytes.TrimSpace(value)) == 0 || !json.Valid(value) {
		return nil, fmt.Errorf("providerwirev4: %s field %q is required", context, name)
	}
	return value, nil
}

func decodeRequiredString(object map[string]json.RawMessage, name, context string) (string, error) {
	raw, err := requireField(object, name, context)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("providerwirev4: decoding %s field %q: %w", context, name, err)
	}
	return value, nil
}

func validateJSON(value json.RawMessage, context string) error {
	if len(bytes.TrimSpace(value)) == 0 || !json.Valid(value) {
		return fmt.Errorf("providerwirev4: %s must contain one valid JSON value", context)
	}
	return nil
}

func validateJSONObject(value json.RawMessage, context string) error {
	_, err := decodeObject(value, context)
	return err
}

func rejectNullFields(object map[string]json.RawMessage, context string, fields ...string) error {
	for _, field := range fields {
		value, exists := object[field]
		if exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s field %q must not be null", context, field)
		}
	}
	return nil
}

func rejectUnknownFields(object map[string]json.RawMessage, context string, fields ...string) error {
	known := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		known[field] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range object {
		if _, exists := known[field]; !exists {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("providerwirev4: %s contains unsupported field %q", context, unknown[0])
}

func decodeSelectedObject(object map[string]json.RawMessage, destination any, fields ...string) error {
	selected := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		if value, exists := object[field]; exists {
			selected[field] = value
		}
	}
	data, err := json.Marshal(selected)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, destination)
}

func validateProviderReference(value json.RawMessage, context string) error {
	return validateStringMap(value, context)
}

func validateQualifiedIdentifier(value, context string) error {
	prefix, suffix, found := strings.Cut(value, ".")
	if !found || strings.TrimSpace(prefix) == "" || strings.TrimSpace(suffix) == "" {
		return fmt.Errorf("providerwirev4: %s must use a non-empty provider-qualified identifier", context)
	}
	return nil
}

func validateStringMap(value json.RawMessage, context string) error {
	object, err := decodeObject(value, context)
	if err != nil {
		return err
	}
	for key, raw := range object {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, key)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, key)
		}
	}
	return nil
}

func validateStringArray(value json.RawMessage, context string) error {
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil || items == nil {
		return fmt.Errorf("providerwirev4: %s must be an array of strings", context)
	}
	for i, raw := range items {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s element %d must be a string", context, i)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("providerwirev4: %s element %d must be a string", context, i)
		}
	}
	return nil
}

func encodeProviderOptions(options provider.ProviderOptions) (providerOptionsDTO, error) {
	if len(options) == 0 {
		return nil, nil
	}
	encoded := make(providerOptionsDTO, len(options))
	for key, option := range options {
		var value json.RawMessage
		if raw, ok := option.(provider.RawProviderOption); ok {
			value = append(json.RawMessage(nil), raw.Raw...)
		} else {
			data, err := json.Marshal(option)
			if err != nil {
				return nil, fmt.Errorf("providerwirev4: encoding provider option %q: %w", key, err)
			}
			value = data
		}
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		encoded[key] = value
	}
	return encoded, nil
}

func encodeNestedProviderOptions(options provider.ProviderOptions, context string) (providerOptionsDTO, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return encodeProviderOptions(options)
}

func decodeProviderOptions(options providerOptionsDTO) (provider.ProviderOptions, error) {
	if len(options) == 0 {
		return nil, nil
	}
	decoded := make(provider.ProviderOptions, len(options))
	for key, value := range options {
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		decoded[key] = provider.RawProviderOption{Key: key, Raw: append(json.RawMessage(nil), value...)}
	}
	return decoded, nil
}

func decodeNestedProviderOptions(options providerOptionsDTO, context string) (provider.ProviderOptions, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return decodeProviderOptions(options)
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
		return dataContent(value), nil
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

func dataContent(value string) *provider.DataContent {
	data := provider.Base64DataContent(value)
	return &data
}
