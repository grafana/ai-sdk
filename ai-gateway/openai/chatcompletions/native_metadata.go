package chatcompletions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func chatMetadata(metadata provider.ProviderMetadata) (compatible.ChatCompletionMetadata, error) {
	var ns struct {
		ChatCompletion compatible.ChatCompletionMetadata `json:"chatCompletion"`
	}
	if raw := metadata["openaiCompatible"]; len(raw) > 0 {
		if json.Unmarshal(raw, &ns) != nil {
			return ns.ChatCompletion, errOutput
		}
	}
	if ns.ChatCompletion.ServiceTier == nil && len(metadata["openai"]) > 0 {
		var responses struct {
			ServiceTier *string `json:"serviceTier"`
		}
		if json.Unmarshal(metadata["openai"], &responses) != nil {
			return ns.ChatCompletion, errOutput
		}
		ns.ChatCompletion.ServiceTier = responses.ServiceTier
	}
	if ns.ChatCompletion.Validate() != nil {
		return ns.ChatCompletion, errOutput
	}
	return ns.ChatCompletion, nil
}
func nativeUsage(u *usage, native *compatible.ChatCompletionUsage) error {
	if u == nil || native == nil {
		return nil
	}
	if native.PromptTokens != nil && *native.PromptTokens != u.Prompt || native.CompletionTokens != nil && *native.CompletionTokens != u.Completion || native.TotalTokens != nil && *native.TotalTokens != u.Total {
		return errOutput
	}
	if d := native.PromptTokensDetails; d != nil {
		if u.PromptDetails == nil {
			u.PromptDetails = &promptDetails{}
		}
		for _, count := range []*int{d.CachedTokens, d.AudioTokens, d.CacheWriteTokens, d.ImageTokens, d.TextTokens} {
			if count != nil && (*count < 0 || *count > u.Prompt) {
				return errOutput
			}
		}
		u.PromptDetails.Cached = d.CachedTokens
		u.PromptDetails.Audio = d.AudioTokens
		u.PromptDetails.CacheWrite = d.CacheWriteTokens
		u.PromptDetails.Image = d.ImageTokens
		u.PromptDetails.Text = d.TextTokens
	}
	if d := native.CompletionTokensDetails; d != nil {
		if u.CompletionDetails == nil {
			u.CompletionDetails = &completionDetails{}
		}
		for _, count := range []*int{d.ReasoningTokens, d.AudioTokens, d.AcceptedPredictionTokens, d.RejectedPredictionTokens, d.TextTokens} {
			if count != nil && (*count < 0 || *count > u.Completion) {
				return errOutput
			}
		}
		u.CompletionDetails.Reasoning = d.ReasoningTokens
		u.CompletionDetails.Audio = d.AudioTokens
		u.CompletionDetails.AcceptedPrediction = d.AcceptedPredictionTokens
		u.CompletionDetails.RejectedPrediction = d.RejectedPredictionTokens
		u.CompletionDetails.Text = d.TextTokens
	}
	return nil
}

func nativeAnnotations(values []compatible.ChatCompletionAnnotation) []annotation {
	if values == nil {
		return nil
	}
	result := make([]annotation, len(values))
	for i, a := range values {
		c := a.URLCitation
		result[i] = annotation{Type: a.Type, URLCitation: urlCitation{StartIndex: *c.StartIndex, EndIndex: *c.EndIndex, Title: c.Title, URL: c.URL}}
	}
	return result
}
