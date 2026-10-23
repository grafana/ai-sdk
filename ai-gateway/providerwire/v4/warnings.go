package v4

import (
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type responseWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature *string              `json:"feature,omitempty"`
	Setting *string              `json:"setting,omitempty"`
	Message *string              `json:"message,omitempty"`
	Details string               `json:"details,omitempty"`
}

func mapWarnings(warnings []provider.Warning) ([]responseWarning, error) {
	mapped := make([]responseWarning, 0, len(warnings))
	for _, warning := range warnings {
		value := responseWarning{Type: warning.Type}
		var first, second string
		switch warning.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility:
			value.Feature, value.Details = &warning.Feature, warning.Details
			first, second = warning.Feature, warning.Details
		case provider.WarnDeprecated:
			value.Setting, value.Message = &warning.Setting, &warning.Message
			first, second = warning.Setting, warning.Message
		case provider.WarnOther:
			value.Message = &warning.Message
			first = warning.Message
		default:
			return nil, errInvalidUnarySuccess
		}
		if !utf8.ValidString(first) || !utf8.ValidString(second) {
			return nil, errInvalidUnarySuccess
		}
		mapped = append(mapped, value)
	}
	return mapped, nil
}
