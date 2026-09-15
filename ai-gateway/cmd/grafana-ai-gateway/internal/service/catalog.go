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
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type modelConstructor func(apiKey, modelID string, options ...anthropicprovider.Option) provider.LanguageModel

// BuildCatalog constructs every configured model exactly once.
func BuildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client) (catalog.Catalog, error) {
	return buildCatalog(file, providers, client, anthropicprovider.New)
}

func buildCatalog(file config.File, providers map[string]config.ResolvedProvider, client *http.Client, construct modelConstructor) (catalog.Catalog, error) {
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
		var model provider.LanguageModel
		switch providerConfig.Type {
		case "anthropic":
			requestOptions := []option.RequestOption{
				option.WithHTTPClient(client),
				option.WithMaxRetries(0),
			}
			if providerConfig.BaseURL != "" {
				requestOptions = append(requestOptions, option.WithBaseURL(providerConfig.BaseURL))
			}
			model = construct(providerConfig.APIKey, configured.Primary.Model, anthropicprovider.WithRequestOptions(requestOptions...))
		case "openai":
			baseURL := providerConfig.BaseURL
			if baseURL == "" {
				baseURL = defaultOpenAIBaseURL
			}
			// Assembling the client directly keeps ambient OPENAI_* environment defaults out of the request.
			responsesService := responses.NewResponseService(
				openaioption.WithAPIKey(providerConfig.APIKey),
				openaioption.WithBaseURL(baseURL),
				openaioption.WithHTTPClient(client),
				openaioption.WithMaxRetries(0),
			)
			model = openaiprovider.NewResponsesWithClient(openaisdk.Client{Responses: responsesService}, configured.Primary.Model)
		default:
			return nil, fmt.Errorf("gateway service: provider %q is invalid", configured.Primary.Provider)
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
