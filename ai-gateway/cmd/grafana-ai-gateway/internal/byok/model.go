package byok

import (
	"fmt"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativeoptions"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// Groups returns one routing group per destination, keyed by its selector,
// with one candidate per account. Accounts are never treated as repeated
// destinations.
func (r Request) Groups() []routing.Group {
	groups := make([]routing.Group, len(r.Destinations))
	for i, destination := range r.Destinations {
		groups[i] = routing.Group{Key: string(destination.Provider) + "/" + destination.Model, Candidates: make([]routing.Candidate, len(destination.Accounts))}
		for j := range destination.Accounts {
			groups[i].Candidates[j] = routing.Candidate{Provider: string(destination.Provider)}
		}
	}
	return groups
}

// New returns the model that tries request's accounts in the order of
// placements, each against its own destination. An all-Anthropic chain also
// checks native options before any attempt.
func New(request Request, placements []routing.Placement, client *http.Client) (provider.LanguageModel, error) {
	if len(placements) == 0 || len(placements) > config.MaxProviderAttempts {
		return nil, ErrInvalidRequest
	}
	if client == nil {
		return nil, fmt.Errorf("gateway byok: HTTP client is required")
	}
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	candidates := make([]provider.LanguageModel, 0, len(placements))
	allAnthropic := true
	for _, placement := range placements {
		if placement.Group < 0 || placement.Group >= len(request.Destinations) {
			return nil, ErrInvalidRequest
		}
		destination := request.Destinations[placement.Group]
		if destination.Model == "" || placement.Candidate < 0 || placement.Candidate >= len(destination.Accounts) {
			return nil, ErrInvalidRequest
		}
		candidate, err := newCandidate(destination, destination.Accounts[placement.Candidate], &ownedClient)
		if err != nil {
			return nil, err
		}
		allAnthropic = allAnthropic && destination.Provider == Anthropic
		candidates = append(candidates, candidate)
	}
	model, err := fallback.New(candidates...)
	if err != nil {
		return nil, err
	}
	chain := model.WithDecider(execution.FallbackDecider)
	if allAnthropic {
		return nativeoptions.Model{LanguageModel: chain, Validate: nativeoptions.Anthropic}, nil
	}
	return chain, nil
}

func newCandidate(destination Destination, account nativemodel.Config, client *http.Client) (provider.LanguageModel, error) {
	var candidate provider.LanguageModel
	var target execution.TargetType
	switch destination.Provider {
	case Anthropic:
		candidate = nativeoptions.Model{LanguageModel: nativemodel.NewAnthropic(account, destination.Model, client), Validate: nativeoptions.Anthropic}
		target = execution.TargetAnthropic
	case OpenAI:
		candidate, target = nativemodel.NewOpenAI(account, destination.Model, client), execution.TargetOpenAI
	default:
		return nil, ErrInvalidSelector
	}
	return execution.Translate(candidate, execution.Target{Type: target, ModelID: destination.Model}), nil
}
