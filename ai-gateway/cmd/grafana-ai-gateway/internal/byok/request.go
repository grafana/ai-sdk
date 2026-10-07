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

type request struct {
	provider providerName
	model    string
	keys     []string
}

func parseRequest(selector string, gateway json.RawMessage) (request, error) {
	name, model, ok := strings.Cut(selector, "/")
	if !ok || model == "" || len(selector) > MaxSelectorBytes || !validHeaderValue(selector) {
		return request{}, ErrInvalidSelector
	}
	selected := providerName(name)
	if selected != anthropic && selected != openai {
		return request{}, ErrInvalidSelector
	}
	var controls map[string]json.RawMessage
	if json.Unmarshal(gateway, &controls) != nil || len(controls) == 0 {
		return request{}, fmt.Errorf("%w: gateway.byok is required", ErrInvalidRequest)
	}
	if len(controls) != 1 || controls["byok"] == nil {
		return request{}, ErrUnsupportedControl
	}
	raw := controls["byok"]
	if len(raw) > maxBYOKBytes {
		return request{}, fmt.Errorf("%w: gateway.byok exceeds byte limit", ErrInvalidRequest)
	}
	var credentials map[string]json.RawMessage
	if json.Unmarshal(raw, &credentials) != nil || len(credentials) == 0 {
		return request{}, fmt.Errorf("%w: gateway.byok must be a nonempty provider map", ErrInvalidRequest)
	}
	for name := range credentials {
		if providerName(name) != anthropic && providerName(name) != openai {
			return request{}, fmt.Errorf("%w: gateway.byok provider is unsupported", ErrInvalidRequest)
		}
	}
	var keys []string
	for _, name := range []providerName{anthropic, openai} {
		raw, exists := credentials[string(name)]
		if !exists {
			continue
		}
		var entries []map[string]json.RawMessage
		if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 || len(entries) > maxCredentials {
			return request{}, fmt.Errorf("%w: gateway.byok.%s must contain 1 to %d credentials", ErrInvalidRequest, name, maxCredentials)
		}
		for index, entry := range entries {
			var key string
			if len(entry) != 1 || entry["apiKey"] == nil || json.Unmarshal(entry["apiKey"], &key) != nil || len(key) > maxCredentialBytes || !validHeaderValue(key) {
				return request{}, fmt.Errorf("%w: gateway.byok.%s[%d] requires only a bounded nonempty apiKey", ErrInvalidRequest, name, index)
			}
			if name == selected {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		return request{}, fmt.Errorf("%w: gateway.byok requires credentials for selected provider", ErrInvalidRequest)
	}
	return request{provider: selected, model: model, keys: keys}, nil
}

func validHeaderValue(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}
