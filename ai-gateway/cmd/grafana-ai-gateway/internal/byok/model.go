package byok

import (
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

func New(request Request, client *http.Client) (provider.LanguageModel, error) {
	if len(request.credentials) == 0 {
		return nil, ErrInvalidRequest
	}
	if client == nil {
		return nil, fmt.Errorf("gateway byok: HTTP client is required")
	}
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	candidates := make([]provider.LanguageModel, 0, len(request.credentials))
	for _, credential := range request.credentials {
		var model provider.LanguageModel
		switch request.provider {
		case anthropic:
			model = nativemodel.NewAnthropic(credential.apiKey, request.model, "", &ownedClient)
		case openai:
			model = nativemodel.NewOpenAI(credential.apiKey, request.model, "", &ownedClient)
		}
		candidates = append(candidates, model)
	}
	model, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	if request.provider == anthropic {
		return nativeoptions.Model{LanguageModel: model, Validate: nativeoptions.Anthropic}, nil
	}
	return model, nil
}
