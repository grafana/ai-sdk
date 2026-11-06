package chatcompletions

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func chatMetadata(metadata provider.ProviderMetadata) (compatible.ChatCompletionMetadata, error) {
	var result compatible.ChatCompletionMetadata
	found := false
	for _, raw := range metadata {
		var namespace map[string]json.RawMessage
		if err := json.Unmarshal(raw, &namespace); err != nil {
			return result, errOutput
		}
		value, ok := namespace[compatible.ChatCompletionMetadataKey]
		if !ok {
			continue
		}
		if found {
			return result, errOutput
		}
		found = true
		if err := json.Unmarshal(value, &result); err != nil {
			return result, errOutput
		}
	}
	if result.Validate() != nil {
		return result, errOutput
	}
	return result, nil
}

func nativeUsage(u *usage, raw json.RawMessage) error {
	if u == nil || len(raw) == 0 {
		return nil
	}
	var native struct {
		Prompt            *int               `json:"prompt_tokens"`
		Completion        *int               `json:"completion_tokens"`
		Total             *int               `json:"total_tokens"`
		PromptDetails     *promptDetails     `json:"prompt_tokens_details"`
		CompletionDetails *completionDetails `json:"completion_tokens_details"`
	}
	if err := json.Unmarshal(raw, &native); err != nil {
		return errOutput
	}
	if native.Prompt != nil && *native.Prompt != u.Prompt || native.Completion != nil && *native.Completion != u.Completion || native.Total != nil && *native.Total != u.Total {
		return errOutput
	}
	if d := native.PromptDetails; d != nil {
		for _, count := range []*int{d.Cached, d.Audio, d.CacheWrite} {
			if count != nil && (*count < 0 || *count > u.Prompt) {
				return errOutput
			}
		}
		u.PromptDetails = d
	}
	if d := native.CompletionDetails; d != nil {
		for _, count := range []*int{d.Reasoning, d.Audio, d.AcceptedPrediction, d.RejectedPrediction} {
			if count != nil && (*count < 0 || *count > u.Completion) {
				return errOutput
			}
		}
		u.CompletionDetails = d
	}
	return nil
}
