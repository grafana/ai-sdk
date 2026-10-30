package config

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"

	"go.yaml.in/yaml/v4"
)

const (
	maxModelRows   = 1024
	maxAliases     = 128
	maxStringBytes = 2048
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

// ParseModelCatalog validates exactly one bounded strict YAML model catalog
// document. Failures never include the document.
func ParseModelCatalog(document string) (File, error) {
	if len(document) > MaxModelCatalogBytes {
		return File{}, fmt.Errorf("config: %s exceeds %d bytes", SettingName(ModelCatalogSetting), MaxModelCatalogBytes)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(document), &node); err != nil {
		return File{}, fmt.Errorf("config: invalid YAML document")
	}
	if err := validateDocumentShape(&node); err != nil {
		return File{}, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(document))
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
			// use for this backend, and the host owns the call-level control
			// namespace. Naming a provider after it would leave that backend's
			// call options read as host controls.
			// openai-compatible reads options under the name before the first dot,
			// trimmed, so "gateway.chat" and " gateway" collide too.
			namespace, _, _ := strings.Cut(provider.ProviderName, ".")
			if strings.TrimSpace(namespace) == execution.ControlNamespace {
				return fmt.Errorf("config: providers.%s.providerName %q is reserved by the runtime as the host provider-option namespace; rename the provider", name, execution.ControlNamespace)
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
		if len(model.Aliases) > maxAliases || len(model.Fallback)+1 > MaxProviderAttempts || len(model.Aliases)+1 > maxModelRows-rowCount {
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

func validateDocumentShape(document *yaml.Node) error {
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
