package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	v4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"

	"go.yaml.in/yaml/v4"
)

const (
	maxModelRows   = 1024
	maxCandidates  = 16
	maxAliases     = 128
	maxStringBytes = 2048
)

var publicIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

// File contains strict named provider and model configuration.
type File struct {
	Auth      *AuthConfig         `yaml:"auth,omitempty"`
	Server    *ServerConfig       `yaml:"server,omitempty"`
	Providers map[string]Provider `yaml:"providers"`
	Models    map[string]Model    `yaml:"models"`
}

// Provider configures one named provider instance.
type Provider struct {
	Type      string `yaml:"type"`
	APIKeyEnv string `yaml:"apiKeyEnv"`
	BaseURL   string `yaml:"baseURL,omitempty"`
	// ProviderName overrides the openai-compatible provider identifier.
	ProviderName string `yaml:"providerName,omitempty"`
}

// Model configures one canonical public model and its aliases.
type Model struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Primary     Primary   `yaml:"primary"`
	Fallback    []Primary `yaml:"fallback,omitempty"`
	Aliases     []string  `yaml:"aliases,omitempty"`
}

// Primary maps one public model to a named provider and backend model ID.
type Primary struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// ResolvedProvider contains one startup-resolved provider secret.
type ResolvedProvider struct {
	Type         string
	APIKey       string
	BaseURL      string
	ProviderName string
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
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return File{}, fmt.Errorf("config: invalid YAML document")
	}
	if err := validateAuthPresence(&document); err != nil {
		return File{}, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	var result File
	if err := decoder.Decode(&result); err != nil {
		return File{}, fmt.Errorf("config: invalid YAML fields or values")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return File{}, fmt.Errorf("config: invalid trailing YAML document")
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
	if file.Auth != nil {
		if err := file.Auth.validate(); err != nil {
			return err
		}
	}
	if len(file.Providers) == 0 {
		return fmt.Errorf("config: at least one provider is required")
	}
	for name, provider := range file.Providers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("config: provider name must not be empty")
		}
		switch provider.Type {
		case "anthropic", "openai":
			if provider.ProviderName != "" {
				return fmt.Errorf("config: providers.%s.providerName is only supported for openai-compatible providers", name)
			}
		case "openai-compatible":
			if provider.ProviderName != "" && strings.TrimSpace(provider.ProviderName) == "" {
				return fmt.Errorf("config: providers.%s.providerName must not be blank", name)
			}
			if strings.TrimSpace(provider.BaseURL) == "" {
				return fmt.Errorf("config: providers.%s.baseURL is required for openai-compatible providers", name)
			}
			// The provider name doubles as the provider-option namespace callers
			// use for this backend, and the runtime reserves one namespace for
			// the host. Naming a provider after it would leave that backend with
			// a namespace every caller is refused.
			// openai-compatible reads options under the name before the first dot,
			// trimmed, so "grafana.chat" and " grafana" collide too.
			namespace, _, _ := strings.Cut(provider.ProviderName, ".")
			if strings.TrimSpace(namespace) == v4.ReservedProviderOptionNamespace {
				return fmt.Errorf("config: providers.%s.providerName %q is reserved by the runtime as the host provider-option namespace; rename the provider", name, v4.ReservedProviderOptionNamespace)
			}
		default:
			return fmt.Errorf("config: providers.%s.type %q is unsupported (want anthropic, openai or openai-compatible)", name, provider.Type)
		}
		if strings.TrimSpace(provider.APIKeyEnv) == "" {
			return fmt.Errorf("config: providers.%s.apiKeyEnv is required", name)
		}
	}
	if len(file.Models) == 0 {
		return fmt.Errorf("config: at least one model is required")
	}
	rowCount := 0
	for _, model := range file.Models {
		if len(model.Aliases) > maxAliases || len(model.Fallback) >= maxCandidates || len(model.Aliases)+1 > maxModelRows-rowCount {
			return fmt.Errorf("config: configured discovery cardinality exceeds limit")
		}
		rowCount += len(model.Aliases) + 1
		if !validDiscoveryText(model.Name) || !validDiscoveryText(model.Description) || !validDiscoveryText(model.Primary.Provider) || !validDiscoveryText(model.Primary.Model) || !validDiscoveryText(file.Providers[model.Primary.Provider].ProviderName) {
			return fmt.Errorf("config: configured discovery text is invalid or exceeds limit")
		}
		for _, candidate := range model.Fallback {
			if !validDiscoveryText(candidate.Provider) || !validDiscoveryText(candidate.Model) || !validDiscoveryText(file.Providers[candidate.Provider].ProviderName) {
				return fmt.Errorf("config: configured candidate text is invalid or exceeds limit")
			}
		}
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
		seenCandidates := map[Primary]struct{}{model.Primary: {}}
		for index, candidate := range model.Fallback {
			if strings.TrimSpace(candidate.Provider) == "" || strings.TrimSpace(candidate.Model) == "" {
				return fmt.Errorf("config: models.%s.fallback[%d] requires provider and model", id, index)
			}
			if _, ok := file.Providers[candidate.Provider]; !ok {
				return fmt.Errorf("config: models.%s.fallback[%d] references unknown provider", id, index)
			}
			if _, exists := seenCandidates[candidate]; exists {
				return fmt.Errorf("config: models.%s.fallback[%d] repeats a candidate", id, index)
			}
			seenCandidates[candidate] = struct{}{}
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
		resolved[name] = ResolvedProvider{Type: provider.Type, APIKey: value, BaseURL: provider.BaseURL, ProviderName: provider.ProviderName}
	}
	return resolved, nil
}

func validDiscoveryText(value string) bool {
	return len(value) <= maxStringBytes && utf8.ValidString(value)
}

func validatePublicID(value string) error {
	if len(value) < 1 || len(value) > 128 || !publicIDPattern.MatchString(value) {
		return fmt.Errorf("public ID must be 1-128 ASCII bytes matching %s", publicIDPattern.String())
	}
	return nil
}

func validateAuthPresence(document *yaml.Node) error {
	if err := rejectYAMLIndirection(document); err != nil {
		return err
	}

	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config: YAML root must be a mapping")
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Kind != yaml.ScalarNode || root.Content[i].Tag != "!!str" {
			return fmt.Errorf("config: YAML root mapping keys must be strings")
		}
		if root.Content[i].Value == "server" {
			if err := validateServerTypes(root.Content[i+1], 0); err != nil {
				return err
			}
		}
		if root.Content[i].Value != "auth" {
			continue
		}
		auth := root.Content[i+1]
		if auth.Kind != yaml.MappingNode || len(auth.Content) == 0 {
			return fmt.Errorf("config: auth must be a nonempty mapping")
		}
		if err := validateAuthScalarTypes(auth); err != nil {
			return err
		}
		for j := 0; j < len(auth.Content); j += 2 {
			if auth.Content[j].Value == "jwt" || auth.Content[j].Value == "staticKey" {
				if auth.Content[j+1].Kind != yaml.MappingNode {
					return fmt.Errorf("config: auth provider configuration must be a mapping")
				}
			}
		}
	}
	return nil
}

func rejectYAMLIndirection(node *yaml.Node) error {
	if node.Tag == "!!merge" || node.Kind == yaml.AliasNode {
		return fmt.Errorf("config: YAML merge keys and aliases are not supported")
	}
	for _, child := range node.Content {
		if err := rejectYAMLIndirection(child); err != nil {
			return err
		}
	}
	return nil
}

func validateAuthScalarTypes(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag != "!!str" {
		return fmt.Errorf("config: auth values and mapping keys must be strings")
	}
	for _, child := range node.Content {
		if err := validateAuthScalarTypes(child); err != nil {
			return err
		}
	}
	return nil
}

func validateServerTypes(node *yaml.Node, depth int) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("config: server configuration must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return fmt.Errorf("config: server mapping keys must be strings")
		}
		if depth == 0 && key.Value == "cloud" {
			if err := validateServerTypes(value, 1); err != nil {
				return err
			}
		}
		if depth == 1 && key.Value == "enabled" && (value.Kind != yaml.ScalarNode || value.Tag != "!!bool") {
			return fmt.Errorf("config: server.cloud.enabled must be a boolean")
		}
	}
	return nil
}
