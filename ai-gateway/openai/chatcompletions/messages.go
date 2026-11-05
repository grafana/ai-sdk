package chatcompletions

import (
	"encoding/json"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"

	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func textContent(raw json.RawMessage, assistant bool) ([]provider.ContentPart, error) {
	var parts []struct {
		Breakpoint *compatible.ChatCacheBreakpoint `json:"prompt_cache_breakpoint"`
		Type       kind                            `json:"type"`
		Text       *string                         `json:"text"`
		Refusal    *string                         `json:"refusal"`
	}
	if decodeStrict(raw, &parts) != nil {
		return nil, errRequest
	}
	result := make([]provider.ContentPart, 0, len(parts))
	for _, p := range parts {
		if p.Breakpoint.Validate() != nil || (p.Type != kindText && p.Breakpoint != nil) {
			return nil, errRequest
		}
		switch {
		case p.Type == kindText && p.Text != nil && p.Refusal == nil:
			result = append(result, provider.TextPart(*p.Text))
		case p.Type == kindRefusal && assistant && p.Refusal != nil && p.Text == nil:
			result = append(result, provider.TextPart(*p.Refusal))
		default:
			return nil, errRequest
		}
	}
	return result, nil
}

func translateMessages(r mappedRequest, target execution.Target, options provider.CallOptions) provider.CallOptions {
	if target.Type != execution.TargetOpenAICompatible {
		return options
	}
	options.Prompt = slices.Clone(options.Prompt)
	for i, m := range r.Messages {
		extra := map[string]any{}
		if m.Name != nil {
			extra["name"] = *m.Name
		}
		if m.Role == roleDeveloper {
			extra["role"] = "developer"
		}
		if m.Role == roleAssistant {
			if m.Audio != nil {
				extra["audio"] = m.Audio
			}
			content, refusal := compatibleAssistantContent(m, options.Prompt[i].Content)
			if refusal != nil {
				extra["refusal"] = *refusal
				options.Prompt[i].Content = content
			}
		}
		applyCacheRequest(m, &options.Prompt[i], extra)
		if len(extra) == 0 {
			continue
		}
		raw, _ := json.Marshal(extra)
		metadata := provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: raw}}
		options.Prompt[i].ProviderOptions = metadata
		if m.Role == roleUser && len(options.Prompt[i].Content) == 1 && options.Prompt[i].Content[0].Type == provider.ContentPartTypeText && !r.hasCacheBreakpoints() {
			options.Prompt[i].Content = slices.Clone(options.Prompt[i].Content)
			options.Prompt[i].Content[0].ProviderOptions = metadata
		}
	}
	return options
}

func compatibleAssistantContent(m message, original []provider.ContentPart) ([]provider.ContentPart, *string) {
	var text string
	var content []provider.ContentPart
	refusal := m.Refusal
	if json.Unmarshal(m.Content, &text) == nil && present(m.Content) {
		content = append(content, provider.TextPart(text))
	} else if present(m.Content) {
		var parts []struct {
			Type    kind   `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		}
		_ = json.Unmarshal(m.Content, &parts)
		for _, part := range parts {
			if part.Type == kindText {
				content = append(content, provider.TextPart(part.Text))
			} else {
				if refusal == nil {
					refusal = ptr("")
				}
				refusal = ptr(*refusal + part.Refusal)
			}
		}
	}
	for _, part := range original {
		if part.Type == provider.ContentPartTypeToolCall {
			content = append(content, part)
		}
	}
	return content, refusal
}
