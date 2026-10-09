package byok

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
)

const maxCredentials = 8

var (
	ErrInvalidRequest  = errors.New("gateway byok: invalid request")
	ErrInvalidSelector = fmt.Errorf("%w: invalid provider/model selector", ErrInvalidRequest)
)

type Provider string

const (
	Anthropic Provider = "anthropic"
	OpenAI    Provider = "openai"
)

type Request struct {
	Provider Provider
	Model    string
	Accounts []nativemodel.Config
}

type requestAccount struct {
	nativemodel.Config
	ModelMappings []json.RawMessage `json:"modelMappings"`
}

func DecodeRequest(selector string, gateway json.RawMessage, approvedBaseURLs map[Provider][]string) (Request, error) {
	name, model, ok := strings.Cut(selector, "/")
	selected := Provider(name)
	if !ok || model == "" || !utf8.ValidString(selector) || (selected != Anthropic && selected != OpenAI) {
		return Request{}, ErrInvalidSelector
	}
	var controls map[string]map[Provider][]requestAccount
	decoder := json.NewDecoder(bytes.NewReader(gateway))
	if decoder.Decode(&controls) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Request{}, fmt.Errorf("%w: gateway.byok has unsupported or invalid fields", ErrInvalidRequest)
	}
	accountsByProvider := controls["byok"]
	if len(controls) != 1 || accountsByProvider == nil {
		return Request{}, fmt.Errorf("%w: gateway.byok is the only supported control", ErrInvalidRequest)
	}
	for name, accounts := range accountsByProvider {
		var defaultURL string
		switch name {
		case Anthropic:
			defaultURL = nativemodel.AnthropicBaseURL
		case OpenAI:
			defaultURL = nativemodel.OpenAIBaseURL
		default:
			return Request{}, fmt.Errorf("%w: unsupported gateway.byok provider", ErrInvalidRequest)
		}
		if len(accounts) == 0 || len(accounts) > maxCredentials {
			return Request{}, fmt.Errorf("%w: gateway.byok requires 1 to %d accounts per provider", ErrInvalidRequest, maxCredentials)
		}
		for _, account := range accounts {
			if len(account.ModelMappings) != 0 {
				return Request{}, fmt.Errorf("%w: modelMappings is not supported", ErrInvalidRequest)
			}
			if account.APIKey == "" {
				return Request{}, fmt.Errorf("%w: apiKey is required", ErrInvalidRequest)
			}
			if name == Anthropic && (account.Organization != "" || account.Project != "") {
				return Request{}, fmt.Errorf("%w: organization and project require OpenAI", ErrInvalidRequest)
			}
			if account.BaseURL != "" && account.BaseURL != defaultURL && !slices.Contains(approvedBaseURLs[name], account.BaseURL) {
				return Request{}, fmt.Errorf("%w: baseURL is not service-approved for this provider", ErrInvalidRequest)
			}
		}
	}
	if len(accountsByProvider[selected]) == 0 {
		return Request{}, fmt.Errorf("%w: gateway.byok requires accounts for selected provider", ErrInvalidRequest)
	}
	accounts := make([]nativemodel.Config, len(accountsByProvider[selected]))
	for i, account := range accountsByProvider[selected] {
		accounts[i] = account.Config
	}
	return Request{Provider: selected, Model: model, Accounts: accounts}, nil
}
