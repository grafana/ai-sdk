package chatcompletions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"maps"
	"slices"
)

type cacheContentPart struct {
	Type       string                          `json:"type"`
	Text       *string                         `json:"text"`
	Breakpoint *compatible.ChatCacheBreakpoint `json:"prompt_cache_breakpoint"`
}

func cacheParts(m message) []cacheContentPart {
	var parts []cacheContentPart
	if json.Unmarshal(m.Content, &parts) != nil {
		return nil
	}
	return parts
}
func (r mappedRequest) hasCacheBreakpoints() bool {
	for _, m := range r.Messages {
		if messageHasCacheBreakpoint(m) {
			return true
		}
	}
	return false
}

func messageHasCacheBreakpoint(m message) bool {
	for _, p := range cacheParts(m) {
		if p.Breakpoint != nil {
			return true
		}
	}
	return false
}

func applyCacheRequest(m message, prompt *provider.Message, extra map[string]any) {
	parts := cacheParts(m)
	marked := false
	for _, p := range parts {
		marked = marked || p.Breakpoint != nil
	}
	if !marked {
		return
	}
	if m.Role != roleUser {
		text := make([]compatible.ChatCacheTextPart, 0, len(parts))
		for _, p := range parts {
			if p.Type == "text" {
				text = append(text, compatible.ChatCacheTextPart{Type: p.Type, Text: p.Text, Breakpoint: p.Breakpoint})
			}
		}
		extra["chatTextParts"] = text
		return
	}
	prompt.Content = slices.Clone(prompt.Content)
	for i, p := range parts {
		if p.Breakpoint == nil {
			continue
		}
		raw, _ := json.Marshal(map[string]any{"prompt_cache_breakpoint": p.Breakpoint})
		// Preserve existing part options (for example image fidelity) by merging
		// the native namespace, never replacing domain role, identity or content.
		opts := maps.Clone(prompt.Content[i].ProviderOptions)
		if opts == nil {
			opts = provider.ProviderOptions{}
		}
		var native map[string]json.RawMessage
		if current, ok := opts["openaiCompatible"]; ok {
			data, _ := json.Marshal(current)
			if rawOption, isRaw := current.(provider.RawProviderOption); isRaw {
				data = rawOption.Raw
			}
			_ = json.Unmarshal(data, &native)
		}
		if native == nil {
			native = map[string]json.RawMessage{}
		}
		var bp map[string]json.RawMessage
		_ = json.Unmarshal(raw, &bp)
		native["prompt_cache_breakpoint"] = bp["prompt_cache_breakpoint"]
		raw, _ = json.Marshal(native)
		opts["openaiCompatible"] = provider.RawProviderOption{Key: "openaiCompatible", Raw: raw}
		prompt.Content[i].ProviderOptions = opts
	}
}
