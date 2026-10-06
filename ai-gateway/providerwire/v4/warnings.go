package v4

import (
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const minimumWarningBytes = int64(len(`{"type":"other","message":""}`))

type responseWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature *string              `json:"feature,omitempty"`
	Setting *string              `json:"setting,omitempty"`
	Message *string              `json:"message,omitempty"`
	Details string               `json:"details,omitempty"`
}

func warningValues(warning provider.Warning) ([2]string, bool) {
	switch warning.Type {
	case provider.WarnUnsupported, provider.WarnCompatibility:
		return [2]string{warning.Feature, warning.Details}, true
	case provider.WarnDeprecated:
		return [2]string{warning.Setting, warning.Message}, true
	case provider.WarnOther:
		return [2]string{warning.Message}, true
	default:
		return [2]string{}, false
	}
}

func warningsPreflight(warnings []provider.Warning, remaining *int64) bool {
	if *remaining < 0 || int64(len(warnings)) > *remaining/minimumWarningBytes {
		return false
	}
	*remaining -= int64(len(warnings)) * minimumWarningBytes
	for _, warning := range warnings {
		values, ok := warningValues(warning)
		if !ok || !responseStringsFit(remaining, values[:]...) {
			return false
		}
	}
	return true
}

func mapWarnings(warnings []provider.Warning) ([]responseWarning, error) {
	mapped := make([]responseWarning, 0, len(warnings))
	for _, warning := range warnings {
		values, ok := warningValues(warning)
		if !ok || !utf8.ValidString(values[0]) || !utf8.ValidString(values[1]) {
			return nil, errInvalidUnarySuccess
		}
		value := responseWarning{Type: warning.Type}
		switch warning.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			value.Feature, value.Details = &warning.Feature, warning.Details
		case provider.WarnDeprecated:
			value.Setting, value.Message = &warning.Setting, &warning.Message
		case provider.WarnOther:
			value.Message = &warning.Message
		}
		mapped = append(mapped, value)
	}
	return mapped, nil
}
