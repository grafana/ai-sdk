package byok

import (
	"encoding/json"
	"fmt"
	"net/http"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

func New(selector string, gateway json.RawMessage, client *http.Client) (provider.LanguageModel, error) {
	selection, err := parseRequest(selector, gateway)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("gateway byok: HTTP client is required")
	}
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	candidates := make([]provider.LanguageModel, 0, len(selection.keys))
	for _, key := range selection.keys {
		var model provider.LanguageModel
		switch selection.provider {
		case anthropic:
			model = anthropicprovider.New(key, selection.model, anthropicprovider.WithRequestOptions(
				anthropicoption.WithBaseURL("https://api.anthropic.com"),
				anthropicoption.WithHTTPClient(&ownedClient),
				anthropicoption.WithMaxRetries(0),
			))
		case openai:
			service := responses.NewResponseService(
				openaioption.WithAPIKey(key),
				openaioption.WithBaseURL("https://api.openai.com/v1"),
				openaioption.WithHTTPClient(&ownedClient),
				openaioption.WithMaxRetries(0),
			)
			model = openaiprovider.NewResponsesWithClient(openaisdk.Client{Responses: service}, selection.model)
		}
		candidates = append(candidates, model)
	}
	model, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	if selection.provider == anthropic {
		return nativeoptions.Model{LanguageModel: model, Validate: nativeoptions.Anthropic}, nil
	}
	return model, nil
}
