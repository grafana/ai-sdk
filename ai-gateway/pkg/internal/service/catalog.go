package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativeoptions"
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

// Catalog is the configured model catalog. It also keeps each route's
// physical candidates, the same instances its prebuilt model uses, so a
// request's routing controls can compose them.
type Catalog struct {
	catalog.Catalog
	routes   map[string]physicalRoute
	factory  ModelFactory
	physical *PhysicalAttemptSink
}

// physicalRoute is one configured route's candidates in configured order.
type physicalRoute struct {
	descriptors []config.Primary
	candidates  []provider.LanguageModel
}

// BuildCatalog constructs every configured model exactly once.
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, factory ModelFactory, physical ...*PhysicalAttemptSink) (*Catalog, error) {
	return buildCatalog(file, providers, client, nativemodel.NewAnthropic, factory, physical...)
}

func buildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, construct modelConstructor, factory ModelFactory, physical ...*PhysicalAttemptSink) (*Catalog, error) {
	if factory == nil {
		return nil, fmt.Errorf("gateway service: model factory is required")
	}
	var sink *PhysicalAttemptSink
	if len(physical) != 0 {
		sink = physical[0]
	}
	routes := make(map[string]physicalRoute, len(file.Models))
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
		for _, descriptor := range descriptors {
			providerConfig, ok := providers[descriptor.Provider]
			if !ok {
				return nil, fmt.Errorf("gateway service: model %q references unresolved provider", id)
			}
			if providerConfig.APIKey == "" {
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			var candidate provider.LanguageModel
			var validateOptions func(provider.CallOptions) error
			target := execution.Target{ModelID: descriptor.Model}
			switch providerConfig.Type {
			case "anthropic":
				candidate = construct(nativemodel.Config{APIKey: providerConfig.APIKey, BaseURL: providerConfig.BaseURL}, descriptor.Model, client)
				validateOptions = nativeoptions.Anthropic
				target.Type = execution.TargetAnthropic
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
				target.Type, target.ProviderName = execution.TargetOpenAICompatible, providerName
			case "openai":
				candidate = nativemodel.NewOpenAI(nativemodel.Config{APIKey: providerConfig.APIKey, BaseURL: providerConfig.BaseURL}, descriptor.Model, client)
				target.Type = execution.TargetOpenAI
			default:
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			if candidate == nil {
				return nil, fmt.Errorf("gateway service: constructing model %q returned nil", id)
			}
			if validateOptions != nil {
				candidate = nativeoptions.Model{LanguageModel: candidate, Validate: validateOptions}
			}
			candidate = execution.Translate(candidate, target)
			providerName := providerConfig.Type
			if providerConfig.Type == "openai-compatible" && providerConfig.ProviderName != "" {
				providerName = providerConfig.ProviderName
			}
			configuredCandidates = append(configuredCandidates, catalog.ConfiguredCandidate{
				ProviderInstance: descriptor.Provider, Provider: providerName, ModelID: descriptor.Model,
			})
			candidates = append(candidates, candidate)
		}
		lower, err := composeCandidates(candidates, descriptors, sink)
		if err != nil {
			return nil, err
		}
		routes[id] = physicalRoute{descriptors: descriptors, candidates: candidates}
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
			Model: model,
		})
	}
	static, err := catalog.NewStatic(entries)
	if err != nil {
		return nil, err
	}
	return &Catalog{Catalog: static, routes: routes, factory: factory, physical: sink}, nil
}

// composeCandidates returns the only candidate, or an ordered fallback chain
// over candidates whose physical attempts are recorded against descriptors.
func composeCandidates(candidates []provider.LanguageModel, descriptors []config.Primary, sink *PhysicalAttemptSink) (provider.LanguageModel, error) {
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	ordered, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	return ordered.WithDecider(execution.FallbackDecider).WithAttemptObserver(physicalAttemptObserver(descriptors, sink)), nil
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
