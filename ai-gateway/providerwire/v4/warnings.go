package v4

import (
	"errors"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxWarningStringBytes           = 4096
	minimumMappedStreamWarningBytes = int64(len(`{"type":"other","message":""}`))
)

var errInvalidStreamWarning = errors.New("providerwire v4: invalid warning")

type streamWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature *string              `json:"feature,omitempty"`
	Setting *string              `json:"setting,omitempty"`
	Message *string              `json:"message,omitempty"`
	Details *string              `json:"details,omitempty"`
}

func warningBytesFit(warnings []provider.Warning, remaining *int64) bool {
	if int64(len(warnings)) > *remaining/minimumMappedStreamWarningBytes {
		return false
	}
	for _, warning := range warnings {
		var fields []string
		switch warning.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			fields = []string{warning.Feature, warning.Details}
		case provider.WarnDeprecated:
			fields = []string{warning.Setting, warning.Message}
		case provider.WarnOther:
			fields = []string{warning.Message}
		default:
			return false
		}
		for _, field := range fields {
			if len(field) > maxWarningStringBytes || int64(len(field)) > *remaining {
				return false
			}
			*remaining -= int64(len(field))
		}
	}
	return true
}

func streamWarningCountFits(count int, limit int64) bool {
	available := limit - int64(len(canonicalEmptyStartFrame))
	return available >= 0 && (count == 0 || int64(count) <= available/(minimumMappedStreamWarningBytes+1))
}

func mapStreamWarnings(warnings []provider.Warning, limit int64) ([]streamWarning, error) {
	remaining := limit
	if !warningBytesFit(warnings, &remaining) {
		return nil, errInvalidStreamWarning
	}
	mapped := make([]streamWarning, 0, len(warnings))
	for _, warning := range warnings {
		value := streamWarning{Type: warning.Type}
		switch warning.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			value.Feature = &warning.Feature
			if warning.Details != "" {
				value.Details = &warning.Details
			}
		case provider.WarnDeprecated:
			value.Setting, value.Message = &warning.Setting, &warning.Message
		case provider.WarnOther:
			value.Message = &warning.Message
		}
		for _, field := range []*string{value.Feature, value.Setting, value.Message, value.Details} {
			if field != nil && !utf8.ValidString(*field) {
				return nil, errInvalidStreamWarning
			}
		}
		mapped = append(mapped, value)
	}
	return mapped, nil
}
