package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

type modelConstructor func(apiKey, modelID string, options ...anthropicprovider.Option) provider.LanguageModel

// ModelFactory composes one canonical logical model around its unchanged lower
// model. WP8 observers use this seam; WP9 may replace the lower model with a
// fallback without changing the logical wrapper.
type ModelFactory func(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error)

// BuildCatalog constructs every configured model exactly once.
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, factory ModelFactory, physical ...*PhysicalAttemptSink) (catalog.Catalog, error) {
	return buildCatalog(file, providers, client, anthropicprovider.New, factory, physical...)
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
		for _, descriptor := range descriptors {
			providerConfig, ok := providers[descriptor.Provider]
			if !ok {
				return nil, fmt.Errorf("gateway service: model %q references unresolved provider", id)
			}
			if providerConfig.APIKey == "" {
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			var candidate provider.LanguageModel
			switch providerConfig.Type {
			case "anthropic":
				requestOptions := []option.RequestOption{option.WithHTTPClient(client), option.WithMaxRetries(0)}
				if providerConfig.BaseURL != "" {
					requestOptions = append(requestOptions, option.WithBaseURL(providerConfig.BaseURL))
				}
				candidate = construct(providerConfig.APIKey, descriptor.Model, anthropicprovider.WithRequestOptions(requestOptions...))
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
			default:
				return nil, fmt.Errorf("gateway service: provider %q is invalid", descriptor.Provider)
			}
			if candidate == nil {
				return nil, fmt.Errorf("gateway service: constructing model %q returned nil", id)
			}
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
			lower = fallbackTextModel{LanguageModel: ordered}
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
			},
			Model: model,
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
