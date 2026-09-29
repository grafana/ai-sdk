package v4

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

var errInvalidProviderMetadata = errors.New("providerwire v4: invalid provider metadata")

type projectedOpenAI struct {
	ItemID string `json:"itemId"`
}

type projectedCallerType string

const projectedCallerDirect projectedCallerType = "direct"

type projectedCaller struct {
	Type projectedCallerType `json:"type"`
}

type projectedAnthropic struct {
	Caller projectedCaller `json:"caller"`
}

type projectedMetadata struct {
	OpenAI    *projectedOpenAI    `json:"openai,omitempty"`
	Anthropic *projectedAnthropic `json:"anthropic,omitempty"`
}

func validItemID(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func readMetadataValue(decoder *json.Decoder, depth int) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= 2 {
		return errInvalidProviderMetadata
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errInvalidProviderMetadata
			}
			if _, exists := seen[name]; exists {
				return errInvalidProviderMetadata
			}
			seen[name] = struct{}{}
			if err := readMetadataValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := readMetadataValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errInvalidProviderMetadata
	}
	_, err = decoder.Token()
	return err
}

func validMetadataEscapes(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || i+1 >= len(raw) {
				return false
			}
			if raw[i+1] != 'u' {
				i++
				continue
			}
			if i+5 >= len(raw) {
				return false
			}
			value, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
			if err != nil || value >= 0xdc00 && value <= 0xdfff {
				return false
			}
			if value >= 0xd800 && value <= 0xdbff {
				if i+11 >= len(raw) || raw[i+6] != '\\' || raw[i+7] != 'u' {
					return false
				}
				low, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
				if err != nil || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			}
			i += 5
		}
	}
	return true
}

func validMetadataNamespace(raw json.RawMessage, limit int64) bool {
	if int64(len(raw)) > limit || !utf8.Valid(raw) || !validMetadataEscapes(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return false
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := key.(string)
		if !ok {
			return false
		}
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
		if readMetadataValue(decoder, 1) != nil {
			return false
		}
	}
	_, err = decoder.Token()
	if err != nil {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}

func projectProviderMetadata(input provider.ProviderMetadata, allowAnthropic bool, textID string, limit int64) (*projectedMetadata, error) {
	var mapped projectedMetadata
	remaining := limit
	if raw, ok := input["openai"]; ok {
		if !validMetadataNamespace(raw, remaining) {
			return nil, errInvalidProviderMetadata
		}
		remaining -= int64(len(raw))
		var value map[string]json.RawMessage
		if json.Unmarshal(raw, &value) != nil {
			return nil, errInvalidProviderMetadata
		}
		if item, ok := value["itemId"]; ok {
			var id string
			if json.Unmarshal(item, &id) != nil || !validItemID(id) || (textID != "" && id != textID) {
				return nil, errInvalidProviderMetadata
			}
			mapped.OpenAI = &projectedOpenAI{ItemID: id}
		}
	}
	if allowAnthropic {
		if raw, ok := input["anthropic"]; ok {
			if !validMetadataNamespace(raw, remaining) {
				return nil, errInvalidProviderMetadata
			}
			var value map[string]json.RawMessage
			if json.Unmarshal(raw, &value) != nil {
				return nil, errInvalidProviderMetadata
			}
			if caller, ok := value["caller"]; ok {
				var fields map[string]json.RawMessage
				if json.Unmarshal(caller, &fields) != nil || len(fields) != 1 {
					return nil, errInvalidProviderMetadata
				}
				var kind string
				if json.Unmarshal(fields["type"], &kind) != nil || kind != string(projectedCallerDirect) {
					return nil, errInvalidProviderMetadata
				}
				mapped.Anthropic = &projectedAnthropic{Caller: projectedCaller{Type: projectedCallerDirect}}
			}
		}
	}
	if mapped.OpenAI == nil && mapped.Anthropic == nil {
		return nil, nil
	}
	return &mapped, nil
}
