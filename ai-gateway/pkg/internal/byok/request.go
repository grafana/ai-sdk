package byok

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/routing"
)

var (
	ErrInvalidRequest  = errors.New("gateway byok: invalid request")
	ErrInvalidSelector = fmt.Errorf("%w: invalid provider/model selector", ErrInvalidRequest)
)

type Provider string

const (
	Anthropic Provider = "anthropic"
	OpenAI    Provider = "openai"
)

// Destination is one native provider/model of a request and its ordered
// accounts.
type Destination struct {
	Provider Provider
	Model    string
	Accounts []nativemodel.Config
}

// Selector returns the destination's provider/model selector.
func (d Destination) Selector() string { return string(d.Provider) + "/" + d.Model }

// Request is a decoded BYOK request: the requested destination followed by one
// destination per fallback model, in order.
type Request struct {
	Destinations []Destination
}

type requestAccount struct {
	nativemodel.Config
	ModelMappings []json.RawMessage `json:"modelMappings"`
}

// DecodeRequest validates the complete request-account map in controls and
// returns the destinations of selector and of each fallback model in
// controls.Models. An invalid selector, requested or fallback, wraps
// [ErrInvalidSelector]; invalid accounts, a destination without accounts, or
// more requested-provider accounts than the attempt bound wrap
// [ErrInvalidRequest]. Fallback destinations are bounded by routing
// composition instead. A repeated selector is decoded once, and destinations
// of the same provider share one read-only account slice.
func DecodeRequest(selector string, controls routing.Controls, approvedBaseURLs map[Provider][]string) (Request, error) {
	selected, model, ok := parseSelector(selector)
	if !ok {
		return Request{}, ErrInvalidSelector
	}
	var accountsByProvider map[Provider][]requestAccount
	if controls.BYOK != nil && json.Unmarshal(controls.BYOK, &accountsByProvider) != nil {
		return Request{}, fmt.Errorf("%w: gateway.byok has unsupported or invalid fields", ErrInvalidRequest)
	}
	if accountsByProvider == nil {
		return Request{}, fmt.Errorf("%w: gateway.byok requires accounts", ErrInvalidRequest)
	}
	configs := make(map[Provider][]nativemodel.Config, len(accountsByProvider))
	for name, accounts := range accountsByProvider {
		if err := validateAccounts(name, accounts, approvedBaseURLs); err != nil {
			return Request{}, err
		}
		configs[name] = make([]nativemodel.Config, len(accounts))
		for i, account := range accounts {
			configs[name][i] = account.Config
		}
	}
	requested, err := destination(selected, model, configs)
	if err != nil {
		return Request{}, err
	}
	if len(requested.Accounts) > config.MaxProviderAttempts {
		return Request{}, fmt.Errorf("%w: gateway.byok allows at most %d accounts for the selected provider", ErrInvalidRequest, config.MaxProviderAttempts)
	}
	request := Request{Destinations: []Destination{requested}}
	seen := map[string]bool{selector: true}
	for _, fallback := range controls.Models {
		name, model, ok := parseSelector(fallback)
		if !ok {
			return Request{}, ErrInvalidSelector
		}
		if seen[fallback] {
			continue
		}
		seen[fallback] = true
		next, err := destination(name, model, configs)
		if err != nil {
			return Request{}, err
		}
		request.Destinations = append(request.Destinations, next)
	}
	return request, nil
}

func parseSelector(selector string) (Provider, string, bool) {
	name, model, ok := strings.Cut(selector, "/")
	provider := Provider(name)
	return provider, model, ok && model != "" && utf8.ValidString(selector) && supportedProvider(provider)
}

func supportedProvider(name Provider) bool {
	return name == Anthropic || name == OpenAI
}

func validateAccounts(name Provider, accounts []requestAccount, approvedBaseURLs map[Provider][]string) error {
	var defaultURL string
	switch name {
	case Anthropic:
		defaultURL = nativemodel.AnthropicBaseURL
	case OpenAI:
		defaultURL = nativemodel.OpenAIBaseURL
	default:
		return fmt.Errorf("%w: unsupported gateway.byok provider", ErrInvalidRequest)
	}
	if len(accounts) == 0 {
		return fmt.Errorf("%w: gateway.byok requires at least one account per provider", ErrInvalidRequest)
	}
	for _, account := range accounts {
		if len(account.ModelMappings) != 0 {
			return fmt.Errorf("%w: modelMappings is not supported", ErrInvalidRequest)
		}
		if account.APIKey == "" {
			return fmt.Errorf("%w: apiKey is required", ErrInvalidRequest)
		}
		if name == Anthropic && (account.Organization != "" || account.Project != "") {
			return fmt.Errorf("%w: organization and project require OpenAI", ErrInvalidRequest)
		}
		if account.BaseURL != "" && account.BaseURL != defaultURL && !slices.Contains(approvedBaseURLs[name], account.BaseURL) {
			return fmt.Errorf("%w: baseURL is not service-approved for this provider", ErrInvalidRequest)
		}
	}
	return nil
}

func destination(name Provider, model string, configs map[Provider][]nativemodel.Config) (Destination, error) {
	accounts := configs[name]
	if len(accounts) == 0 {
		return Destination{}, fmt.Errorf("%w: gateway.byok requires accounts for each named provider", ErrInvalidRequest)
	}
	return Destination{Provider: name, Model: model, Accounts: accounts}, nil
}
