package byok

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
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

// Request is a decoded BYOK request: the requested destination followed by one
// destination per fallback model, in order, and the provider start timeouts.
type Request struct {
	Destinations []Destination
	Timeouts     map[Provider]time.Duration
}

type requestAccount struct {
	nativemodel.Config
	ModelMappings []json.RawMessage `json:"modelMappings"`
}

// DecodeRequest validates the complete request-account map in controls and
// returns the destinations of selector and of each fallback model in
// controls.Models. An invalid selector, requested or fallback, wraps
// [ErrInvalidSelector]; invalid accounts, or a destination without accounts,
// wrap [ErrInvalidRequest]; a provider timeout for an unsupported provider is
// [execution.ErrProviderTimeouts].
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
	for name, accounts := range accountsByProvider {
		if err := validateAccounts(name, accounts, approvedBaseURLs); err != nil {
			return Request{}, err
		}
	}
	requested, err := destination(selected, model, accountsByProvider)
	if err != nil {
		return Request{}, err
	}
	timeouts := make(map[Provider]time.Duration, len(controls.ProviderTimeouts))
	for name, milliseconds := range controls.ProviderTimeouts {
		if !supportedProvider(Provider(name)) {
			return Request{}, execution.ErrProviderTimeouts
		}
		timeouts[Provider(name)] = time.Duration(milliseconds) * time.Millisecond
	}
	request := Request{Destinations: []Destination{requested}, Timeouts: timeouts}
	for _, fallback := range controls.Models {
		name, model, ok := parseSelector(fallback)
		if !ok {
			return Request{}, ErrInvalidSelector
		}
		next, err := destination(name, model, accountsByProvider)
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

func destination(name Provider, model string, accountsByProvider map[Provider][]requestAccount) (Destination, error) {
	supplied := accountsByProvider[name]
	if len(supplied) == 0 {
		return Destination{}, fmt.Errorf("%w: gateway.byok requires accounts for selected provider", ErrInvalidRequest)
	}
	if len(supplied) > config.MaxProviderAttempts {
		return Destination{}, fmt.Errorf("%w: gateway.byok allows at most %d accounts for the selected provider", ErrInvalidRequest, config.MaxProviderAttempts)
	}
	accounts := make([]nativemodel.Config, len(supplied))
	for i, account := range supplied {
		accounts[i] = account.Config
	}
	return Destination{Provider: name, Model: model, Accounts: accounts}, nil
}
