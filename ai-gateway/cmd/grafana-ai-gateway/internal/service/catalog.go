package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
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
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, factory ModelFactory) (catalog.Catalog, error) {
	return buildCatalog(file, providers, client, anthropicprovider.New, factory)
}

func buildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, construct modelConstructor, factory ModelFactory) (catalog.Catalog, error) {
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
		providerConfig, ok := providers[configured.Primary.Provider]
		if !ok {
			return nil, fmt.Errorf("gateway service: model %q references unresolved provider", id)
		}
		if providerConfig.APIKey == "" {
			return nil, fmt.Errorf("gateway service: provider %q is invalid", configured.Primary.Provider)
		}
		var lower provider.LanguageModel
		switch providerConfig.Type {
		case "anthropic":
			requestOptions := []option.RequestOption{
				option.WithHTTPClient(client),
				option.WithMaxRetries(0),
			}
			if providerConfig.BaseURL != "" {
				requestOptions = append(requestOptions, option.WithBaseURL(providerConfig.BaseURL))
			}
			lower = construct(providerConfig.APIKey, configured.Primary.Model, anthropicprovider.WithRequestOptions(requestOptions...))
		case "openai-compatible":
			if providerConfig.BaseURL == "" {
				return nil, fmt.Errorf("gateway service: provider %q is invalid", configured.Primary.Provider)
			}
			lower = openaicompatible.New(configured.Primary.Model,
				openaicompatible.WithAPIKey(providerConfig.APIKey),
				openaicompatible.WithBaseURL(providerConfig.BaseURL),
				openaicompatible.WithHTTPClient(client),
				openaicompatible.WithProviderName(providerConfig.ProviderName),
				// ProviderWire finish parts carry usage, so streams must request it.
				openaicompatible.WithIncludeUsage(true),
			)
		default:
			return nil, fmt.Errorf("gateway service: provider %q is invalid", configured.Primary.Provider)
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
