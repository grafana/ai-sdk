package byok

import (
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

func New(name Provider, modelID string, accounts []nativemodel.Config, client *http.Client) (provider.LanguageModel, error) {
	var construct func(nativemodel.Config, string, *http.Client) provider.LanguageModel
	var target execution.TargetType
	switch name {
	case Anthropic:
		construct, target = nativemodel.NewAnthropic, execution.TargetAnthropic
	case OpenAI:
		construct, target = nativemodel.NewOpenAI, execution.TargetOpenAI
	default:
		return nil, ErrInvalidSelector
	}
	if modelID == "" || len(accounts) == 0 || len(accounts) > config.MaxProviderAttempts {
		return nil, ErrInvalidRequest
	}
	if client == nil {
		return nil, fmt.Errorf("gateway byok: HTTP client is required")
	}
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	candidates := make([]provider.LanguageModel, 0, len(accounts))
	for _, account := range accounts {
		candidate := construct(account, modelID, &ownedClient)
		if name == Anthropic {
			candidate = nativeoptions.Model{LanguageModel: candidate, Validate: nativeoptions.Anthropic}
		}
		candidates = append(candidates, execution.Translate(candidate, execution.Target{Type: target, ModelID: modelID}))
	}
	model, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	chain := model.WithDecider(execution.FallbackDecider)
	if name == Anthropic {
		return nativeoptions.Model{LanguageModel: chain, Validate: nativeoptions.Anthropic}, nil
	}
	return chain, nil
}
