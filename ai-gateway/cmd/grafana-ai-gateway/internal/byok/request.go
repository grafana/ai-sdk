package byok

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxSelectorBytes   = 2048
	maxCredentialBytes = 4096
	maxCredentials     = 8
	maxBYOKBytes       = 65536
)

var (
	ErrInvalidRequest     = errors.New("gateway byok: invalid request")
	ErrInvalidSelector    = fmt.Errorf("%w: invalid provider/model selector", ErrInvalidRequest)
	ErrUnsupportedControl = fmt.Errorf("%w: unsupported gateway control", ErrInvalidRequest)
)

type providerName string

const (
	anthropic providerName = "anthropic"
	openai    providerName = "openai"
)

type Request struct {
	provider    providerName
	model       string
	credentials []credential
}

type credential struct {
	apiKey string
}

func (c *credential) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	var key string
	if json.Unmarshal(data, &fields) != nil || len(fields) != 1 ||
		json.Unmarshal(fields["apiKey"], &key) != nil ||
		len(key) > maxCredentialBytes || !validHeaderValue(key) {
		return fmt.Errorf("%w: credential requires only a bounded nonempty apiKey", ErrInvalidRequest)
	}
	c.apiKey = key
	return nil
}

func DecodeRequest(selector string, gateway json.RawMessage) (Request, error) {
	name, model, ok := strings.Cut(selector, "/")
	selected := providerName(name)
	if !ok || model == "" || len(selector) > MaxSelectorBytes || !validHeaderValue(selector) ||
		(selected != anthropic && selected != openai) {
		return Request{}, ErrInvalidSelector
	}
	var controls map[string]json.RawMessage
	if json.Unmarshal(gateway, &controls) != nil || len(controls) == 0 {
		return Request{}, fmt.Errorf("%w: gateway.byok is required", ErrInvalidRequest)
	}
	if len(controls) != 1 || controls["byok"] == nil {
		return Request{}, ErrUnsupportedControl
	}
	credentials, err := decodeCredentials(controls["byok"])
	if err != nil {
		return Request{}, err
	}
	if len(credentials[selected]) == 0 {
		return Request{}, fmt.Errorf("%w: gateway.byok requires credentials for selected provider", ErrInvalidRequest)
	}
	return Request{provider: selected, model: model, credentials: credentials[selected]}, nil
}

func decodeCredentials(raw json.RawMessage) (map[providerName][]credential, error) {
	if len(raw) > maxBYOKBytes {
		return nil, fmt.Errorf("%w: gateway.byok exceeds byte limit", ErrInvalidRequest)
	}
	var providers map[providerName]json.RawMessage
	if json.Unmarshal(raw, &providers) != nil || len(providers) == 0 {
		return nil, fmt.Errorf("%w: gateway.byok must be a nonempty provider map", ErrInvalidRequest)
	}
	credentials := make(map[providerName][]credential, len(providers))
	for name, raw := range providers {
		if name != anthropic && name != openai {
			return nil, fmt.Errorf("%w: gateway.byok provider is unsupported", ErrInvalidRequest)
		}
		var entries []credential
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("%w: gateway.byok.%s contains invalid credentials", ErrInvalidRequest, name)
		}
		if len(entries) == 0 || len(entries) > maxCredentials {
			return nil, fmt.Errorf("%w: gateway.byok.%s must contain 1 to %d credentials", ErrInvalidRequest, name, maxCredentials)
		}
		credentials[name] = entries
	}
	return credentials, nil
}

func validHeaderValue(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}
