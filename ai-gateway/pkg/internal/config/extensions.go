package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v4"
)

const (
	extensionEnabledKey = "enabled"
	extensionConfigKey  = "config"
	extensionSecretKey  = "Env"
)

// ExtensionEntry is one operator extension's entry in the extensions setting.
type ExtensionEntry struct {
	// Enabled is nil when the entry does not set it.
	Enabled *bool
	// Config is the entry's configuration as a JSON object, or nil when absent.
	Config json.RawMessage
	// SecretEnv maps each secret name to the environment variable named by its
	// <name>Env key.
	SecretEnv map[string]string
}

// ParseExtensions reads the extensions setting: one strict YAML mapping from
// extension name to its entry. A blank document has no entries. It checks only
// the document's shape; names, secret keys and configurations are checked
// against the registered extensions. Failures never include values.
func ParseExtensions(document string) (map[string]ExtensionEntry, error) {
	setting := SettingName(ExtensionsSetting)
	if len(document) > MaxExtensionsBytes {
		return nil, fmt.Errorf("config: %s exceeds %d bytes", setting, MaxExtensionsBytes)
	}
	if strings.TrimSpace(document) == "" {
		return nil, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(document))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, fmt.Errorf("config: %s is not valid YAML", setting)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config: %s must be one YAML document", setting)
	}
	if rejectYAMLIndirection(&node) != nil {
		return nil, fmt.Errorf("config: %s: YAML merge keys and aliases are not supported", setting)
	}
	root := documentRoot(&node)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config: %s must be a mapping of extension names", setting)
	}
	entries := make(map[string]ExtensionEntry, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, fmt.Errorf("config: %s keys must be extension names", setting)
		}
		name := key.Value
		if _, duplicate := entries[name]; duplicate {
			return nil, fmt.Errorf("config: %s.%s is repeated", setting, name)
		}
		entry, err := parseExtensionEntry(setting+"."+name, value)
		if err != nil {
			return nil, err
		}
		entries[name] = entry
	}
	return entries, nil
}

func documentRoot(node *yaml.Node) *yaml.Node {
	if node.Kind != yaml.DocumentNode || len(node.Content) != 1 {
		return nil
	}
	return node.Content[0]
}

func parseExtensionEntry(path string, node *yaml.Node) (ExtensionEntry, error) {
	if node.Kind != yaml.MappingNode {
		return ExtensionEntry{}, fmt.Errorf("config: %s must be a mapping", path)
	}
	var entry ExtensionEntry
	seen := make(map[string]struct{}, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return ExtensionEntry{}, fmt.Errorf("config: %s keys must be strings", path)
		}
		if _, duplicate := seen[key.Value]; duplicate {
			return ExtensionEntry{}, fmt.Errorf("config: %s.%s is repeated", path, key.Value)
		}
		seen[key.Value] = struct{}{}
		field := path + "." + key.Value
		switch {
		case key.Value == extensionEnabledKey:
			var enabled bool
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" || value.Decode(&enabled) != nil {
				return ExtensionEntry{}, fmt.Errorf("config: %s must be a boolean", field)
			}
			entry.Enabled = &enabled
		case key.Value == extensionConfigKey:
			config, err := yamlObjectJSON(value)
			if err != nil {
				return ExtensionEntry{}, fmt.Errorf("config: %s must be a mapping of JSON values with string keys", field)
			}
			entry.Config = config
		case strings.HasSuffix(key.Value, extensionSecretKey) && len(key.Value) > len(extensionSecretKey):
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || strings.TrimSpace(value.Value) == "" {
				return ExtensionEntry{}, fmt.Errorf("config: %s must name an environment variable", field)
			}
			if entry.SecretEnv == nil {
				entry.SecretEnv = map[string]string{}
			}
			entry.SecretEnv[strings.TrimSuffix(key.Value, extensionSecretKey)] = value.Value
		default:
			return ExtensionEntry{}, fmt.Errorf("config: %s is not a supported field", field)
		}
	}
	return entry, nil
}

// yamlObjectJSON converts a YAML mapping with string keys at every level to a
// JSON object.
func yamlObjectJSON(node *yaml.Node) (json.RawMessage, error) {
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("not a mapping")
	}
	value, err := yamlValue(node)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func yamlValue(node *yaml.Node) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		object := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, errors.New("non-string key")
			}
			if _, duplicate := object[key.Value]; duplicate {
				return nil, errors.New("repeated key")
			}
			value, err := yamlValue(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			object[key.Value] = value
		}
		return object, nil
	case yaml.SequenceNode:
		array := make([]any, len(node.Content))
		for i, item := range node.Content {
			value, err := yamlValue(item)
			if err != nil {
				return nil, err
			}
			array[i] = value
		}
		return array, nil
	case yaml.ScalarNode:
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, errors.New("unsupported node")
	}
}
