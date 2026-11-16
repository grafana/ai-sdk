package execution

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// anthropicFallbackKind is the custom content kind of an Anthropic server-side
// fallback marker, whose to.model names the model that served the output.
const anthropicFallbackKind = "anthropic.fallback"

// servedModel returns the to.model of a valid Anthropic server-side fallback
// marker. Metadata that is not valid UTF-8, or a missing or empty model, is
// not a valid marker.
func servedModel(kind string, metadata provider.ProviderMetadata) (string, bool) {
	raw := metadata["anthropic"]
	if kind != anthropicFallbackKind || !utf8.Valid(raw) {
		return "", false
	}
	var marker struct {
		To struct {
			Model string `json:"model"`
		} `json:"to"`
	}
	if json.Unmarshal(raw, &marker) != nil || marker.To.Model == "" {
		return "", false
	}
	return marker.To.Model, true
}

func (a *attemptCapture) serveContent(content []provider.GenerateContentPart) {
	for _, part := range content {
		if part.Type == provider.ContentCustom {
			a.servePart(part.Kind, part.ProviderMetadata)
		}
	}
}

func (a *attemptCapture) servePart(kind string, metadata provider.ProviderMetadata) {
	if model, ok := servedModel(kind, metadata); ok {
		a.serve(model)
	}
}
