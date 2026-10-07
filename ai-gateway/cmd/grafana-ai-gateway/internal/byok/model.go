package byok

import (
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

func New(name Provider, modelID string, accounts []nativemodel.Config, client *http.Client) (provider.LanguageModel, error) {
	var construct func(nativemodel.Config, string, *http.Client) provider.LanguageModel
	switch name {
	case Anthropic:
		construct = nativemodel.NewAnthropic
	case OpenAI:
		construct = nativemodel.NewOpenAI
	default:
		return nil, ErrInvalidSelector
	}
	if modelID == "" || len(accounts) == 0 || len(accounts) > maxCredentials {
		return nil, ErrInvalidRequest
	}
	if client == nil {
		return nil, fmt.Errorf("gateway byok: HTTP client is required")
	}
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	candidates := make([]provider.LanguageModel, 0, len(accounts))
	for _, account := range accounts {
		candidates = append(candidates, construct(account, modelID, &ownedClient))
	}
	model, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	if name == Anthropic {
		return nativeoptions.Model{LanguageModel: model, Validate: nativeoptions.Anthropic}, nil
	}
	return model, nil
}
