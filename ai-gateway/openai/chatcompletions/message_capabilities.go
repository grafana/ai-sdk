package chatcompletions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func (r mappedRequest) validateMessageCapabilities(target execution.Target) error {
	for _, m := range r.Messages {
		if target.Type != execution.TargetOpenAICompatible {
			if m.Name != nil && *m.Name != "" {
				return provider.ErrUnsupportedFunctionality
			}
			if hasSubstantiveRefusal(m) {
				return provider.ErrUnsupportedFunctionality
			}
		}
	}
	return nil
}

func hasSubstantiveRefusal(m message) bool {
	if m.Refusal != nil && *m.Refusal != "" {
		return true
	}
	var parts []struct {
		Type    kind   `json:"type"`
		Refusal string `json:"refusal"`
	}
	_ = json.Unmarshal(m.Content, &parts)
	for _, p := range parts {
		if p.Type == kindRefusal && p.Refusal != "" {
			return true
		}
	}
	return false
}
