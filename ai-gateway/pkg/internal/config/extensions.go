package config

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v4"
)

// ExtensionEntry is one operator extension's entry in the extensions setting.
type ExtensionEntry struct {
	// Enabled is nil when the entry does not set it.
	Enabled *bool `yaml:"enabled"`
	// Config is the entry's configuration, or nil when absent.
	Config map[string]any `yaml:"config"`
	// Secrets maps each secret name to the environment variable holding it.
	Secrets map[string]string `yaml:"secrets"`
}

// ParseExtensions reads the extensions setting: one strict YAML mapping from
// extension name to its entry. A blank document has no entries. It checks only
// the document's shape; names, secrets and configurations are checked against
// the registered extensions. Failures never include values.
func ParseExtensions(document string) (map[string]ExtensionEntry, error) {
	setting := SettingName(ExtensionsSetting)
	if len(document) > MaxExtensionsBytes {
		return nil, fmt.Errorf("config: %s exceeds %d bytes", setting, MaxExtensionsBytes)
	}
	if strings.TrimSpace(document) == "" {
		return nil, nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(document), &node); err != nil {
		return nil, fmt.Errorf("config: %s is not valid YAML", setting)
	}
	if err := validateDocumentShape(&node); err != nil {
		return nil, fmt.Errorf("%w in %s", err, setting)
	}
	decoder := yaml.NewDecoder(strings.NewReader(document))
	decoder.KnownFields(true)
	var entries map[string]ExtensionEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("config: %s has invalid fields or values", setting)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config: %s must be one YAML document", setting)
	}
	return entries, nil
}
