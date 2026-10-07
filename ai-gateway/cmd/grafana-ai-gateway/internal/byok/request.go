package byok

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/schema"
)

const (
	MaxSelectorBytes   = 2048
	maxCredentialBytes = 4096
	maxCredentials     = 8
	maxBYOKBytes       = 65536
	maxBaseURLBytes    = 2048
	maxAccountIDBytes  = 256
)

var (
	ErrInvalidRequest     = errors.New("gateway byok: invalid request")
	ErrInvalidSelector    = fmt.Errorf("%w: invalid provider/model selector", ErrInvalidRequest)
	ErrUnsupportedControl = fmt.Errorf("%w: unsupported gateway control", ErrInvalidRequest)
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

const accountSchema = `{
	"type":"object","minProperties":1,"additionalProperties":false,
	"properties":{
		"anthropic":{"type":"array","minItems":1,"maxItems":8,"items":{
			"type":"object","required":["apiKey"],"additionalProperties":false,
			"properties":{"apiKey":{"type":"string","minLength":1},"baseURL":{"type":"string"}}
		}},
		"openai":{"type":"array","minItems":1,"maxItems":8,"items":{
			"type":"object","required":["apiKey"],"additionalProperties":false,
			"properties":{
				"apiKey":{"type":"string","minLength":1},"baseURL":{"type":"string"},
				"organization":{"type":"string"},"project":{"type":"string"}
			}
		}}
	}
}`

var compileAccountSchema = sync.OnceValues(func() (*schema.CompiledSchema, error) {
	return schema.CompileSchema(json.RawMessage(accountSchema))
})

func DecodeRequest(selector string, gateway json.RawMessage, approvedBaseURLs map[Provider][]string) (Request, error) {
	name, model, ok := strings.Cut(selector, "/")
	selected := Provider(name)
	if !ok || model == "" || len(selector) > MaxSelectorBytes || !validHeaderValue(selector) ||
		(selected != Anthropic && selected != OpenAI) {
		return Request{}, ErrInvalidSelector
	}
	var controls map[string]json.RawMessage
	if json.Unmarshal(gateway, &controls) != nil || len(controls) == 0 {
		return Request{}, fmt.Errorf("%w: gateway.byok is required", ErrInvalidRequest)
	}
	if len(controls) != 1 || controls["byok"] == nil {
		return Request{}, ErrUnsupportedControl
	}
	raw := controls["byok"]
	if len(raw) > maxBYOKBytes {
		return Request{}, fmt.Errorf("%w: gateway.byok exceeds byte limit", ErrInvalidRequest)
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return Request{}, ErrInvalidRequest
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return Request{}, ErrInvalidRequest
	}
	validator, err := compileAccountSchema()
	if err != nil {
		return Request{}, fmt.Errorf("gateway byok: compiling account schema: %w", err)
	}
	if validator.Validate(normalized) != nil {
		return Request{}, fmt.Errorf("%w: gateway.byok has an unsupported provider or invalid account fields", ErrInvalidRequest)
	}
	var accounts map[Provider][]nativemodel.Config
	if json.Unmarshal(normalized, &accounts) != nil {
		return Request{}, ErrInvalidRequest
	}
	for name, entries := range accounts {
		for _, account := range entries {
			if err := validateAccount(name, account, approvedBaseURLs[name]); err != nil {
				return Request{}, err
			}
		}
	}
	if len(accounts[selected]) == 0 {
		return Request{}, fmt.Errorf("%w: gateway.byok requires accounts for selected provider", ErrInvalidRequest)
	}
	return Request{Provider: selected, Model: model, Accounts: accounts[selected]}, nil
}

func validateAccount(name Provider, account nativemodel.Config, approvedBaseURLs []string) error {
	if len(account.APIKey) > maxCredentialBytes || !validHeaderValue(account.APIKey) {
		return fmt.Errorf("%w: apiKey must be a bounded nonempty header value", ErrInvalidRequest)
	}
	for _, value := range []string{account.Organization, account.Project} {
		if len(value) > maxAccountIDBytes || (value != "" && !validHeaderValue(value)) {
			return fmt.Errorf("%w: organization and project must be bounded header values", ErrInvalidRequest)
		}
	}
	if account.BaseURL == "" {
		return nil
	}
	endpoint, err := url.Parse(account.BaseURL)
	if err != nil || len(account.BaseURL) > maxBaseURLBytes || !validHeaderValue(account.BaseURL) ||
		endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.Opaque != "" ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery ||
		strings.Contains(account.BaseURL, "#") {
		return fmt.Errorf("%w: baseURL must be an absolute HTTPS URL without credentials, query or fragment", ErrInvalidRequest)
	}
	defaultURL := nativemodel.AnthropicBaseURL
	if name == OpenAI {
		defaultURL = nativemodel.OpenAIBaseURL
	}
	if account.BaseURL != defaultURL && !slices.Contains(approvedBaseURLs, account.BaseURL) {
		return fmt.Errorf("%w: baseURL is not service-approved for this provider", ErrInvalidRequest)
	}
	return nil
}

func validHeaderValue(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}
