package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

type modelConstructor func(config nativemodel.Config, modelID string, client *http.Client) provider.LanguageModel

// ModelFactory composes one canonical logical model around its unchanged lower
// model. WP8 observers use this seam; WP9 may replace the lower model with a
// fallback without changing the logical wrapper.
type ModelFactory func(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error)

// BuildCatalog constructs every configured model exactly once.
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, factory ModelFactory, physical ...*PhysicalAttemptSink) (catalog.Catalog, error) {
	return buildCatalog(file, providers, client, nativemodel.NewAnthropic, factory, physical...)
}

func buildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, construct modelConstructor, factory ModelFactory, physical ...*PhysicalAttemptSink) (catalog.Catalog, error) {
	if factory == nil {
		return nil, fmt.Errorf("gateway service: model factory is required")
	}
	ids := make([]string, 0, len(file.Models))
	for id := range file.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	entries := make([]catalog.StaticEntry, 0, len(ids))
	for _, id := range ids {
		configured := file.Models[id]
		descriptors := append([]config.Primary{configured.Primary}, configured.Fallback...)
		candidates := make([]provider.LanguageModel, 0, len(descriptors))
		configuredCandidates := make([]catalog.ConfiguredCandidate, 0, len(descriptors))
		protectedSources := make([]string, 0, len(descriptors))
		for _, descriptor := range descriptors {
			providerConfig, ok := providers[descriptor.Provider]
			if !ok {
				return nil, fmt.Errorf("gateway service: model %q references unresolved provider", id)
			}
			if providerConfig.APIKey == "" {
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			protectedSources = append(protectedSources, providerConfig.APIKey)
			var candidate provider.LanguageModel
			var validateOptions func(provider.CallOptions) error
			switch providerConfig.Type {
			case "anthropic":
				candidate = construct(nativemodel.Config{APIKey: providerConfig.APIKey, BaseURL: providerConfig.BaseURL}, descriptor.Model, client)
				validateOptions = nativeoptions.Anthropic
			case "openai-compatible":
				if providerConfig.BaseURL == "" {
					return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
				}
				candidate = openaicompatible.New(descriptor.Model,
					openaicompatible.WithAPIKey(providerConfig.APIKey),
					openaicompatible.WithBaseURL(providerConfig.BaseURL),
					openaicompatible.WithHTTPClient(client),
					openaicompatible.WithProviderName(providerConfig.ProviderName),
					// ProviderWire finish parts carry usage, so streams must request it.
					openaicompatible.WithIncludeUsage(true),
				)
				providerName := candidate.Provider()
				validateOptions = func(options provider.CallOptions) error {
					return nativeoptions.Compatible(options, providerName)
				}
			case "openai":
				candidate = nativemodel.NewOpenAI(nativemodel.Config{APIKey: providerConfig.APIKey, BaseURL: providerConfig.BaseURL}, descriptor.Model, client)
			default:
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			if candidate == nil {
				return nil, fmt.Errorf("gateway service: constructing model %q returned nil", id)
			}
			if validateOptions != nil {
				candidate = nativeoptions.Model{LanguageModel: candidate, Validate: validateOptions}
			}
			providerName := providerConfig.Type
			if providerConfig.Type == "openai-compatible" && providerConfig.ProviderName != "" {
				providerName = providerConfig.ProviderName
			}
			configuredCandidates = append(configuredCandidates, catalog.ConfiguredCandidate{
				ProviderInstance: descriptor.Provider, Provider: providerName, ModelID: descriptor.Model,
			})
			candidates = append(candidates, candidate)
		}
		lower := candidates[0]
		if len(candidates) > 1 {
			ordered, err := fallback.New(candidates...)
			if err != nil {
				return nil, err
			}
			var sink *PhysicalAttemptSink
			if len(physical) != 0 {
				sink = physical[0]
			}
			ordered.WithAttemptObserver(physicalAttemptObserver(descriptors, sink))
			lower = ordered
		}
		model, err := factory(id, lower)
		if err != nil {
			return nil, fmt.Errorf("gateway service: composing model %q: %w", id, err)
		}
		if model == nil {
			return nil, fmt.Errorf("gateway service: composing model %q returned nil", id)
		}
		entries = append(entries, catalog.StaticEntry{
			Info: catalog.ModelInfo{
				ID:          id,
				Name:        configured.Name,
				Description: configured.Description,
				Aliases:     append([]string(nil), configured.Aliases...),
				Candidates:  configuredCandidates,
			},
			Model:            model,
			ProtectedSources: protectedSources,
		})
	}
	return catalog.NewStatic(entries)
}

func identityModelFactory(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
	if lower == nil {
		return nil, fmt.Errorf("lower model is nil")
	}
	return middleware.Wrap(middleware.WrapOptions{
		Model:      lower,
		ProviderID: "grafana",
		ModelID:    canonicalID,
	}), nil
}
