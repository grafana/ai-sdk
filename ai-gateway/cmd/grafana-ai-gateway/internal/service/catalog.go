package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
)

type modelConstructor func(apiKey, modelID string, options ...anthropicprovider.Option) provider.LanguageModel

// BuildCatalog constructs every configured Anthropic model exactly once.
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client) (catalog.Catalog, error) {
	if client == nil {
		return nil, fmt.Errorf("gateway service: anthropic client is nil")
	}
	return buildCatalog(file, providers, client, anthropicprovider.New)
}

func buildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, construct modelConstructor) (catalog.Catalog, error) {
	if construct == nil {
		return nil, fmt.Errorf("gateway service: model constructor is nil")
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
		if providerConfig.Type != "anthropic" || providerConfig.APIKey == "" {
			return nil, fmt.Errorf("gateway service: provider %q is invalid", configured.Primary.Provider)
		}
		requestOptions := []option.RequestOption{
			option.WithHTTPClient(client),
			option.WithMaxRetries(0),
		}
		if providerConfig.BaseURL != "" {
			requestOptions = append(requestOptions, option.WithBaseURL(providerConfig.BaseURL))
		}
		model := construct(providerConfig.APIKey, configured.Primary.Model, anthropicprovider.WithRequestOptions(requestOptions...))
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
