package chatcompletions

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func (r mappedRequest) preserveNativeFiles(target execution.Target, options provider.CallOptions) provider.CallOptions {
	if target.Type != execution.TargetOpenAICompatible {
		return options
	}
	options.Prompt = slices.Clone(options.Prompt)
	for i, m := range r.Messages {
		if m.Role != roleUser {
			continue
		}
		var parts []userPart
		if json.Unmarshal(m.Content, &parts) != nil {
			continue
		}
		options.Prompt[i].Content = slices.Clone(options.Prompt[i].Content)
		for j, p := range parts {
			if p.File != nil {
				part := &options.Prompt[i].Content[j]
				part.ProviderOptions = maps.Clone(part.ProviderOptions)
				if part.ProviderOptions == nil {
					part.ProviderOptions = provider.ProviderOptions{}
				}
				part.ProviderOptions["openaiCompatible"] = compatible.ChatCompletionFileOptions{PreserveNativeFile: true}
			}
		}
	}
	return options
}
