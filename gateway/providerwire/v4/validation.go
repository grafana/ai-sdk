package providerwirev4

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
	object, err := decodeObject(value, context)
	if err != nil {
		return err
	}
	if _, exists := object["type"]; exists {
		return fmt.Errorf("providerwirev4: %s contains reserved field %q", context, "type")
	}
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
