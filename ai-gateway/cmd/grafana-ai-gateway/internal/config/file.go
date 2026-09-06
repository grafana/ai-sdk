package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"
)

var publicIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

// File contains strict named provider and model configuration.
type File struct {
	Providers map[string]Provider `yaml:"providers"`
	Models    map[string]Model    `yaml:"models"`
}

// Provider configures one named provider instance.
type Provider struct {
	Type      string `yaml:"type"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
	BaseURL   string `yaml:"baseURL,omitempty"`
}

// Model configures one canonical public model and its aliases.
type Model struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Primary     Primary  `yaml:"primary"`
	Aliases     []string `yaml:"aliases,omitempty"`
}

// Primary maps one public model to a named provider and backend model ID.
type Primary struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// ResolvedProvider contains one startup-resolved provider secret.
type ResolvedProvider struct {
	Type    string
	APIKey  string
	BaseURL string
}

// LoadFile reads and validates exactly one bounded strict YAML document.
func LoadFile(path string, maxBytes int64) (File, error) {
	if maxBytes <= 0 || maxBytes == int64(^uint64(0)>>1) {
		return File{}, fmt.Errorf("config: maximum file bytes are unsafe")
	}
	info, err := os.Stat(path)
	if err != nil {
		return File{}, fmt.Errorf("config: inspecting file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("config: path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("config: opening file: %w", err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return File{}, fmt.Errorf("config: reading file: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return File{}, fmt.Errorf("config: file exceeds %d bytes", maxBytes)
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	var result File
	if err := decoder.Decode(&result); err != nil {
		return File{}, fmt.Errorf("config: decoding YAML: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return File{}, fmt.Errorf("config: decoding trailing YAML: %w", err)
		}
		return File{}, fmt.Errorf("config: trailing YAML document is not allowed")
	}
	if err := result.Validate(); err != nil {
		return File{}, err
	}
	return result, nil
}

// Validate validates provider references, model routes, and public IDs.
func (file File) Validate() error {
	if len(file.Providers) == 0 {
		return fmt.Errorf("config: at least one provider is required")
	}
	for name, provider := range file.Providers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("config: provider name must not be empty")
		}
		if provider.Type != "anthropic" {
			return fmt.Errorf("config: provider %q has unsupported type", name)
		}
		if strings.TrimSpace(provider.APIKeyEnv) == "" {
			return fmt.Errorf("config: providers.%s.apiKeyEnv is required", name)
		}
	}
	if len(file.Models) == 0 {
		return fmt.Errorf("config: at least one model is required")
	}
	publicIDs := make(map[string]string, len(file.Models))
	for id, model := range file.Models {
		if err := validatePublicID(id); err != nil {
			return fmt.Errorf("config: model %q: %w", id, err)
		}
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("config: models.%s.name is required", id)
		}
		if strings.TrimSpace(model.Primary.Provider) == "" {
			return fmt.Errorf("config: models.%s.primary.provider is required", id)
		}
		if _, ok := file.Providers[model.Primary.Provider]; !ok {
			return fmt.Errorf("config: models.%s.primary.provider references unknown provider %q", id, model.Primary.Provider)
		}
		if strings.TrimSpace(model.Primary.Model) == "" {
			return fmt.Errorf("config: models.%s.primary.model is required", id)
		}
		if existing, ok := publicIDs[id]; ok {
			return fmt.Errorf("config: public model ID %q collides with %s", id, existing)
		}
		publicIDs[id] = "canonical model"
	}
	for id, model := range file.Models {
		seenAliases := make(map[string]struct{}, len(model.Aliases))
		for _, alias := range model.Aliases {
			if err := validatePublicID(alias); err != nil {
				return fmt.Errorf("config: model %q alias %q: %w", id, alias, err)
			}
			if _, exists := seenAliases[alias]; exists {
				return fmt.Errorf("config: model %q repeats alias %q", id, alias)
			}
			seenAliases[alias] = struct{}{}
			if existing, exists := publicIDs[alias]; exists {
				return fmt.Errorf("config: alias %q collides with %s", alias, existing)
			}
			publicIDs[alias] = "alias"
		}
	}
	return nil
}

// ResolveProviderSecrets resolves each unique referenced environment variable once.
func (file File) ResolveProviderSecrets(lookupEnv LookupEnv) (map[string]ResolvedProvider, error) {
	if lookupEnv == nil {
		return nil, fmt.Errorf("config: environment lookup is nil")
	}
	values := make(map[string]string)
	resolved := make(map[string]ResolvedProvider, len(file.Providers))
	for name, provider := range file.Providers {
		value, exists := values[provider.APIKeyEnv]
		if !exists {
			var ok bool
			value, ok = lookupEnv(provider.APIKeyEnv)
			if !ok || value == "" {
				return nil, fmt.Errorf("config: providers.%s.apiKeyEnv %q is unset or empty", name, provider.APIKeyEnv)
			}
			values[provider.APIKeyEnv] = value
		}
		resolved[name] = ResolvedProvider{Type: provider.Type, APIKey: value, BaseURL: provider.BaseURL}
	}
	return resolved, nil
}

func validatePublicID(value string) error {
	if len(value) < 1 || len(value) > 128 || !publicIDPattern.MatchString(value) {
		return fmt.Errorf("public ID must be 1-128 ASCII bytes matching %s", publicIDPattern.String())
	}
	return nil
}
