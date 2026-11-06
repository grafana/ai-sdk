package chatcompletions

import (
	"encoding/json"
	"slices"

	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

// projectLogprobs normalizes Responses' per-output groups and limits alternatives
// for both native protocols without mutating provider-owned metadata.
func (r mappedRequest) projectLogprobs(metadata provider.ProviderMetadata, native *compatible.ChatCompletionLogprobs) (*compatible.ChatCompletionLogprobs, error) {
	if !boolValue(r.Logprobs) {
		return nil, nil
	}
	if native == nil && len(metadata["openai"]) > 0 {
		var values struct {
			Logprobs [][]compatible.ChatCompletionTokenLogprob `json:"logprobs"`
		}
		if json.Unmarshal(metadata["openai"], &values) != nil {
			return nil, errOutput
		}
		if values.Logprobs != nil {
			native = &compatible.ChatCompletionLogprobs{}
			for _, group := range values.Logprobs {
				native.Content = append(native.Content, group...)
			}
		}
	}
	if native == nil {
		return nil, nil
	}
	if (compatible.ChatCompletionMetadata{Logprobs: native}).Validate() != nil {
		return nil, errOutput
	}
	count := 0
	if r.TopLogprobs != nil {
		count = *r.TopLogprobs
	}
	out := *native
	out.Content = trimLogprobAlternatives(native.Content, count)
	out.Refusal = trimLogprobAlternatives(native.Refusal, count)
	return &out, nil
}

func trimLogprobAlternatives(values []compatible.ChatCompletionTokenLogprob, count int) []compatible.ChatCompletionTokenLogprob {
	out := slices.Clone(values)
	for i := range out {
		// Empty arrays distinguish zero requested alternatives from unavailable data.
		out[i].TopLogprobs = append([]compatible.ChatCompletionTokenProbability{}, values[i].TopLogprobs[:min(count, len(values[i].TopLogprobs))]...)
	}
	return out
}
