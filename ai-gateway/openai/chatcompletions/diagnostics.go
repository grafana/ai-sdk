package chatcompletions

import (
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type warning struct {
	Type    provider.WarningType `json:"type"`
	Feature string               `json:"feature,omitempty"`
	Setting string               `json:"setting,omitempty"`
	Message string               `json:"message,omitempty"`
	Details string               `json:"details,omitempty"`
}

type nativeResponse struct {
	ID      string `json:"id,omitempty"`
	Model   string `json:"model,omitempty"`
	Created *int64 `json:"created,omitempty"`
}

type diagnostics struct {
	Warnings       []warning       `json:"warnings,omitempty"`
	NativeResponse *nativeResponse `json:"native_response,omitempty"`
}

func mapWarnings(values []provider.Warning, max int64) ([]warning, error) {
	if len(values) > 1024 {
		return nil, errOutput
	}
	result := make([]warning, 0, len(values))
	for _, value := range values {
		switch value.Type {
		case provider.WarnUnsupported, provider.WarnCompatibility, provider.WarnDeprecated, provider.WarnOther:
		default:
			return nil, errOutput
		}
		for _, text := range []string{value.Feature, value.Setting, value.Message, value.Details} {
			if !utf8.ValidString(text) || int64(len(text)) > max {
				return nil, errOutput
			}
			max -= int64(len(text))
		}
		result = append(result, warning{value.Type, value.Feature, value.Setting, value.Message, value.Details})
	}
	return result, nil
}

func mapNativeResponse(id, model string, timestamp time.Time, max int64) (*nativeResponse, error) {
	if !utf8.ValidString(id) || !utf8.ValidString(model) || int64(len(id))+int64(len(model)) > max {
		return nil, errOutput
	}
	result := &nativeResponse{ID: id, Model: model}
	if !timestamp.IsZero() {
		if timestamp.Year() < 1 || timestamp.Year() > 9999 {
			return nil, errOutput
		}
		result.Created = ptr(timestamp.Unix())
	}
	return result, nil
}

func (n *nativeResponse) apply(id, model *string, created *int64) {
	if n.ID != "" {
		*id = n.ID
	}
	if n.Model != "" {
		*model = n.Model
	}
	if n.Created != nil {
		*created = *n.Created
	}
}
