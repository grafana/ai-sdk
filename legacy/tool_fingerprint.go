package aisdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type canonicalUndefined struct{}

type toolDescriptionFingerprint struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

// ToolDrift describes differences between two tool fingerprint maps.
type ToolDrift struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// FingerprintTools returns a stable digest for each tool's security-relevant
// definition fields: string description, input JSON schema, and title.
func FingerprintTools(tools ToolSet) (map[string]string, error) {
	fingerprints := make(map[string]string, len(tools))
	for name, tool := range tools {
		digest, err := fingerprintTool(tool)
		if err != nil {
			return nil, fmt.Errorf("aisdk: fingerprinting tool %q: %w", name, err)
		}
		fingerprints[name] = digest
	}
	return fingerprints, nil
}

// DetectToolDrift compares current tool fingerprints with a trusted baseline.
func DetectToolDrift(current, baseline map[string]string) ToolDrift {
	drift := ToolDrift{
		Added:   []string{},
		Removed: []string{},
		Changed: []string{},
	}

	currentNames := make([]string, 0, len(current))
	for name := range current {
		currentNames = append(currentNames, name)
	}
	sort.Strings(currentNames)

	for _, name := range currentNames {
		baselineDigest, ok := baseline[name]
		if !ok {
			drift.Added = append(drift.Added, name)
			continue
		}
		if current[name] != baselineDigest {
			drift.Changed = append(drift.Changed, name)
		}
	}

	baselineNames := make([]string, 0, len(baseline))
	for name := range baseline {
		baselineNames = append(baselineNames, name)
	}
	sort.Strings(baselineNames)

	for _, name := range baselineNames {
		if _, ok := current[name]; !ok {
			drift.Removed = append(drift.Removed, name)
		}
	}

	return drift
}

func fingerprintTool(tool Tool) (string, error) {
	inputSchema, err := toolFingerprintSchema(tool)
	if err != nil {
		return "", err
	}

	title := any(canonicalUndefined{})
	if tool.Title != "" {
		title = tool.Title
	}

	payload := map[string]any{
		"description": tagToolDescription(tool.Description),
		"inputSchema": inputSchema,
		"title":       title,
	}
	return hashCanonical(payload)
}

func tagToolDescription(description string) toolDescriptionFingerprint {
	if description == "" {
		return toolDescriptionFingerprint{Type: "none"}
	}
	return toolDescriptionFingerprint{Type: "string", Value: description}
}

func toolFingerprintSchema(tool Tool) (any, error) {
	raw := tool.InputSchema.JSON()
	if len(raw) == 0 {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}, nil
	}
	return decodeCanonicalJSON(raw)
}

func canonicalJSONString(value any) (string, error) {
	decoded, err := normalizeCanonicalValue(value)
	if err != nil {
		return "", err
	}
	return writeCanonicalJSON(decoded)
}

func normalizeCanonicalValue(value any) (any, error) {
	switch v := value.(type) {
	case canonicalUndefined:
		return v, nil
	case json.RawMessage:
		return decodeCanonicalJSON(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			normalized, err := normalizeCanonicalValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			normalized, err := normalizeCanonicalValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return decodeCanonicalJSON(data)
	}
}

func decodeCanonicalJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func writeCanonicalJSON(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case canonicalUndefined:
		return "undefined", nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case string:
		return strconv.Quote(v), nil
	case json.Number:
		return v.String(), nil
	case float64:
		data, err := json.Marshal(v)
		return string(data), err
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			part, err := writeCanonicalJSON(item)
			if err != nil {
				return "", err
			}
			parts[i] = part
		}
		return "[" + strings.Join(parts, ",") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]string, len(keys))
		for i, key := range keys {
			part, err := writeCanonicalJSON(v[key])
			if err != nil {
				return "", err
			}
			entries[i] = strconv.Quote(key) + ":" + part
		}
		return "{" + strings.Join(entries, ",") + "}", nil
	default:
		return "", fmt.Errorf("unsupported canonical JSON value %T", value)
	}
}
