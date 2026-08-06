package providerwirev4

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type providerOptionsDTO map[string]json.RawMessage
type providerMetadataDTO map[string]json.RawMessage

type dataDTO struct {
	Type      string          `json:"type"`
	Data      *string         `json:"data,omitempty"`
	URL       *string         `json:"url,omitempty"`
	Reference json.RawMessage `json:"reference,omitempty"`
	Text      *string         `json:"text,omitempty"`
}

type warningDTO struct {
	Type    string  `json:"type"`
	Feature *string `json:"feature,omitempty"`
	Setting *string `json:"setting,omitempty"`
	Message *string `json:"message,omitempty"`
	Details *string `json:"details,omitempty"`
}

func (dto *warningDTO) UnmarshalJSON(data []byte) error {
	type warningAlias warningDTO
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if err := rejectNullFields(object, "warning", "type", "feature", "setting", "message", "details"); err != nil {
		return err
	}
	return json.Unmarshal(data, (*warningAlias)(dto))
}

type finishReasonDTO struct {
	Unified string `json:"unified"`
	Raw     string `json:"raw,omitempty"`
}

func (dto *finishReasonDTO) UnmarshalJSON(data []byte) error {
	type finishReasonAlias finishReasonDTO
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if err := rejectNullFields(object, "finish reason", "unified", "raw"); err != nil {
		return err
	}
	return json.Unmarshal(data, (*finishReasonAlias)(dto))
}

type inputUsageDTO struct {
	Total      *int `json:"total,omitempty"`
	NoCache    *int `json:"noCache,omitempty"`
	CacheRead  *int `json:"cacheRead,omitempty"`
	CacheWrite *int `json:"cacheWrite,omitempty"`
}

func (dto *inputUsageDTO) UnmarshalJSON(data []byte) error {
	type inputUsageAlias inputUsageDTO
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if err := rejectNullFields(object, "input token usage", "total", "noCache", "cacheRead", "cacheWrite"); err != nil {
		return err
	}
	return json.Unmarshal(data, (*inputUsageAlias)(dto))
}

type outputUsageDTO struct {
	Total     *int `json:"total,omitempty"`
	Text      *int `json:"text,omitempty"`
	Reasoning *int `json:"reasoning,omitempty"`
}

func (dto *outputUsageDTO) UnmarshalJSON(data []byte) error {
	type outputUsageAlias outputUsageDTO
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if err := rejectNullFields(object, "output token usage", "total", "text", "reasoning"); err != nil {
		return err
	}
	return json.Unmarshal(data, (*outputUsageAlias)(dto))
}

type usageDTO struct {
	InputTokens  *inputUsageDTO  `json:"inputTokens"`
	OutputTokens *outputUsageDTO `json:"outputTokens"`
	Raw          json.RawMessage `json:"raw,omitempty"`
}

func decodeObject(data []byte, context string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("providerwirev4: decoding %s: %w", context, err)
	}
	if object == nil {
		return nil, fmt.Errorf("providerwirev4: %s must be an object", context)
	}
	return object, nil
}

func requireField(object map[string]json.RawMessage, name, context string) (json.RawMessage, error) {
	value, err := requireJSONField(object, name, context)
	if err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, fmt.Errorf("providerwirev4: %s field %q is required", context, name)
	}
	return value, nil
}

func requireJSONField(object map[string]json.RawMessage, name, context string) (json.RawMessage, error) {
	value, ok := object[name]
	if !ok || len(bytes.TrimSpace(value)) == 0 || !json.Valid(value) {
		return nil, fmt.Errorf("providerwirev4: %s field %q is required", context, name)
	}
	return value, nil
}

func decodeRequiredString(object map[string]json.RawMessage, name, context string) (string, error) {
	raw, err := requireField(object, name, context)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("providerwirev4: decoding %s field %q: %w", context, name, err)
	}
	return value, nil
}

func validateJSON(value json.RawMessage, context string) error {
	if len(bytes.TrimSpace(value)) == 0 || !json.Valid(value) {
		return fmt.Errorf("providerwirev4: %s must contain one valid JSON value", context)
	}
	return nil
}

func validateJSONObject(value json.RawMessage, context string) error {
	_, err := decodeObject(value, context)
	return err
}

func rejectNullFields(object map[string]json.RawMessage, context string, fields ...string) error {
	for _, field := range fields {
		value, exists := object[field]
		if exists && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s field %q must not be null", context, field)
		}
	}
	return nil
}

func validateProviderReference(value json.RawMessage, context string) error {
	object, err := decodeObject(value, context)
	if err != nil {
		return err
	}
	if _, exists := object["type"]; exists {
		return fmt.Errorf("providerwirev4: %s must not contain field %q", context, "type")
	}
	for providerID, reference := range object {
		if bytes.Equal(bytes.TrimSpace(reference), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, providerID)
		}
		var id string
		if err := json.Unmarshal(reference, &id); err != nil {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, providerID)
		}
	}
	return nil
}

func validateQualifiedIdentifier(value, context string) error {
	prefix, suffix, found := strings.Cut(value, ".")
	if !found || strings.TrimSpace(prefix) == "" || strings.TrimSpace(suffix) == "" {
		return fmt.Errorf("providerwirev4: %s must use a non-empty provider-qualified identifier", context)
	}
	return nil
}

func validateStringMap(value json.RawMessage, context string) error {
	object, err := decodeObject(value, context)
	if err != nil {
		return err
	}
	for key, raw := range object {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, key)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("providerwirev4: %s field %q must be a string", context, key)
		}
	}
	return nil
}

func validateStringArray(value json.RawMessage, context string) error {
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil || items == nil {
		return fmt.Errorf("providerwirev4: %s must be an array of strings", context)
	}
	for i, raw := range items {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("providerwirev4: %s element %d must be a string", context, i)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("providerwirev4: %s element %d must be a string", context, i)
		}
	}
	return nil
}

func rejectContradictoryFields(object map[string]json.RawMessage, context string, known []string, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for _, field := range known {
		if _, present := object[field]; !present {
			continue
		}
		if _, ok := allowedSet[field]; !ok {
			return fmt.Errorf("providerwirev4: %s contains contradictory field %q", context, field)
		}
	}
	return nil
}

func encodeProviderOptions(options provider.ProviderOptions) (providerOptionsDTO, error) {
	if len(options) == 0 {
		return nil, nil
	}
	encoded := make(providerOptionsDTO, len(options))
	for key, option := range options {
		var value json.RawMessage
		if raw, ok := option.(provider.RawProviderOption); ok {
			value = append(json.RawMessage(nil), raw.Raw...)
		} else {
			data, err := json.Marshal(option)
			if err != nil {
				return nil, fmt.Errorf("providerwirev4: encoding provider option %q: %w", key, err)
			}
			value = data
		}
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		encoded[key] = value
	}
	return encoded, nil
}

func encodeNestedProviderOptions(options provider.ProviderOptions, context string) (providerOptionsDTO, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return encodeProviderOptions(options)
}

func decodeProviderOptions(options providerOptionsDTO) (provider.ProviderOptions, error) {
	if len(options) == 0 {
		return nil, nil
	}
	decoded := make(provider.ProviderOptions, len(options))
	for key, value := range options {
		if _, err := decodeObject(value, fmt.Sprintf("provider option %q", key)); err != nil {
			return nil, err
		}
		decoded[key] = provider.RawProviderOption{Key: key, Raw: append(json.RawMessage(nil), value...)}
	}
	return decoded, nil
}

func decodeNestedProviderOptions(options providerOptionsDTO, context string) (provider.ProviderOptions, error) {
	if _, exists := options["gateway"]; exists {
		return nil, fmt.Errorf("providerwirev4: %s must not contain reserved provider option %q", context, "gateway")
	}
	return decodeProviderOptions(options)
}

func encodeProviderMetadata(metadata provider.ProviderMetadata) (providerMetadataDTO, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	encoded := make(providerMetadataDTO, len(metadata))
	for key, value := range metadata {
		if _, err := decodeObject(value, fmt.Sprintf("provider metadata %q", key)); err != nil {
			return nil, err
		}
		encoded[key] = append(json.RawMessage(nil), value...)
	}
	return encoded, nil
}

func decodeProviderMetadata(metadata providerMetadataDTO) (provider.ProviderMetadata, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	decoded := make(provider.ProviderMetadata, len(metadata))
	for key, value := range metadata {
		if _, err := decodeObject(value, fmt.Sprintf("provider metadata %q", key)); err != nil {
			return nil, err
		}
		decoded[key] = append(json.RawMessage(nil), value...)
	}
	return decoded, nil
}

func encodeData(data *provider.DataContent, allowReferenceText bool) (*dataDTO, error) {
	if data == nil {
		return nil, errors.New("providerwirev4: file data is required")
	}
	if err := data.Validate(); err != nil {
		return nil, fmt.Errorf("providerwirev4: validating file data: %w", err)
	}
	switch {
	case data.Bytes != nil || data.Base64 != "":
		value := data.Base64
		if data.Bytes != nil {
			value = base64.StdEncoding.EncodeToString(data.Bytes)
		}
		return &dataDTO{Type: "data", Data: &value}, nil
	case data.URL != "":
		value := data.URL
		return &dataDTO{Type: "url", URL: &value}, nil
	case len(data.Reference) > 0 && allowReferenceText:
		if err := validateProviderReference(data.Reference, "file reference"); err != nil {
			return nil, err
		}
		return &dataDTO{Type: "reference", Reference: append(json.RawMessage(nil), data.Reference...)}, nil
	case data.Text != "" && allowReferenceText:
		value := data.Text
		return &dataDTO{Type: "text", Text: &value}, nil
	default:
		return nil, errors.New("providerwirev4: file data variant is not representable")
	}
}

func decodeData(raw json.RawMessage, allowReferenceText bool) (*provider.DataContent, error) {
	object, err := decodeObject(raw, "file data")
	if err != nil {
		return nil, err
	}
	variant, err := decodeRequiredString(object, "type", "file data")
	if err != nil {
		return nil, err
	}
	knownFields := []string{"data", "url", "reference", "text"}
	switch variant {
	case "data":
		if err := rejectContradictoryFields(object, "file data", knownFields, "data"); err != nil {
			return nil, err
		}
		value, err := decodeRequiredString(object, "data", "file data")
		if err != nil {
			return nil, err
		}
		return dataContent(value), nil
	case "url":
		if err := rejectContradictoryFields(object, "file data", knownFields, "url"); err != nil {
			return nil, err
		}
		value, err := decodeRequiredString(object, "url", "file data")
		if err != nil || value == "" {
			if err == nil {
				err = errors.New("providerwirev4: file data URL must not be empty")
			}
			return nil, err
		}
		return &provider.DataContent{URL: value}, nil
	case "reference":
		if err := rejectContradictoryFields(object, "file data", knownFields, "reference"); err != nil {
			return nil, err
		}
		if !allowReferenceText {
			return nil, errors.New("providerwirev4: reference file data is not supported here")
		}
		value, err := requireField(object, "reference", "file data")
		if err != nil {
			return nil, err
		}
		if err := validateProviderReference(value, "file reference"); err != nil {
			return nil, err
		}
		return &provider.DataContent{Reference: append(json.RawMessage(nil), value...)}, nil
	case "text":
		if err := rejectContradictoryFields(object, "file data", knownFields, "text"); err != nil {
			return nil, err
		}
		if !allowReferenceText {
			return nil, errors.New("providerwirev4: text file data is not supported here")
		}
		value, err := decodeRequiredString(object, "text", "file data")
		if err != nil {
			return nil, err
		}
		return &provider.DataContent{Text: value}, nil
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported file data type %q", variant)
	}
}

func dataContent(value string) *provider.DataContent {
	if value == "" {
		return &provider.DataContent{Bytes: []byte{}}
	}
	return &provider.DataContent{Base64: value}
}

func encodeWarnings(warnings []provider.Warning) ([]warningDTO, error) {
	if warnings == nil {
		return nil, nil
	}
	encoded := make([]warningDTO, len(warnings))
	for i, warning := range warnings {
		dto := warningDTO{Type: string(warning.Type)}
		switch warning.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			if warning.Setting != "" || warning.Message != "" {
				return nil, fmt.Errorf("providerwirev4: warning %q contains contradictory fields", warning.Type)
			}
			dto.Feature = &warning.Feature
			if warning.Details != "" {
				dto.Details = &warning.Details
			}
		case provider.WarnDeprecated:
			if warning.Feature != "" || warning.Details != "" {
				return nil, errors.New("providerwirev4: deprecated warning contains contradictory fields")
			}
			dto.Setting, dto.Message = &warning.Setting, &warning.Message
		case provider.WarnOther:
			if warning.Feature != "" || warning.Setting != "" || warning.Details != "" {
				return nil, errors.New("providerwirev4: other warning contains contradictory fields")
			}
			dto.Message = &warning.Message
		default:
			return nil, fmt.Errorf("providerwirev4: unsupported warning type %q", warning.Type)
		}
		encoded[i] = dto
	}
	return encoded, nil
}

func decodeWarnings(warnings []warningDTO) ([]provider.Warning, error) {
	if warnings == nil {
		return nil, nil
	}
	decoded := make([]provider.Warning, len(warnings))
	for i, warning := range warnings {
		typeValue := provider.WarningType(warning.Type)
		value := provider.Warning{Type: typeValue}
		if warning.Details != nil {
			value.Details = *warning.Details
		}
		switch typeValue {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			if warning.Feature == nil || warning.Setting != nil || warning.Message != nil {
				return nil, fmt.Errorf("providerwirev4: warning %q fields do not match its variant", warning.Type)
			}
			value.Feature = *warning.Feature
		case provider.WarnDeprecated:
			if warning.Setting == nil || warning.Message == nil || warning.Feature != nil || warning.Details != nil {
				return nil, errors.New("providerwirev4: deprecated warning setting and message are required")
			}
			value.Setting, value.Message = *warning.Setting, *warning.Message
		case provider.WarnOther:
			if warning.Message == nil || warning.Feature != nil || warning.Setting != nil || warning.Details != nil {
				return nil, errors.New("providerwirev4: other warning message is required")
			}
			value.Message = *warning.Message
		default:
			return nil, fmt.Errorf("providerwirev4: unsupported warning type %q", warning.Type)
		}
		decoded[i] = value
	}
	return decoded, nil
}

func encodeUsage(usage provider.Usage) (usageDTO, error) {
	if len(usage.Raw) > 0 {
		if _, err := decodeObject(usage.Raw, "raw usage"); err != nil {
			return usageDTO{}, err
		}
	}
	return usageDTO{
		InputTokens:  &inputUsageDTO{Total: usage.InputTokens.Total, NoCache: usage.InputTokens.NoCache, CacheRead: usage.InputTokens.CacheRead, CacheWrite: usage.InputTokens.CacheWrite},
		OutputTokens: &outputUsageDTO{Total: usage.OutputTokens.Total, Text: usage.OutputTokens.Text, Reasoning: usage.OutputTokens.Reasoning},
		Raw:          append(json.RawMessage(nil), usage.Raw...),
	}, nil
}

func decodeUsage(usage usageDTO) (provider.Usage, error) {
	if usage.InputTokens == nil || usage.OutputTokens == nil {
		return provider.Usage{}, errors.New("providerwirev4: usage inputTokens and outputTokens objects are required")
	}
	if len(usage.Raw) > 0 {
		if _, err := decodeObject(usage.Raw, "raw usage"); err != nil {
			return provider.Usage{}, err
		}
	}
	return provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: usage.InputTokens.Total, NoCache: usage.InputTokens.NoCache, CacheRead: usage.InputTokens.CacheRead, CacheWrite: usage.InputTokens.CacheWrite},
		OutputTokens: provider.OutputTokenUsage{Total: usage.OutputTokens.Total, Text: usage.OutputTokens.Text, Reasoning: usage.OutputTokens.Reasoning},
		Raw:          append(json.RawMessage(nil), usage.Raw...),
	}, nil
}

func validateFinishReason(reason finishReasonDTO) error {
	switch provider.UnifiedFinishReason(reason.Unified) {
	case provider.FinishReasonStop, provider.FinishReasonLength, provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls, provider.FinishReasonError, provider.FinishReasonOther:
		return nil
	default:
		return fmt.Errorf("providerwirev4: unsupported finish reason %q", reason.Unified)
	}
}
