package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/grafana/ai-sdk/provider"
)

const anthropicFallbackKind = "anthropic.fallback"
const fallbackBlockType constant.Fallback = "fallback"

func marshalFallbackMetadata(block anthropic.BetaFallbackBlock) (provider.ProviderMetadata, error) {
	data, err := json.Marshal(anthropic.BetaFallbackBlockParam{
		From: anthropic.BetaFallbackInfoParam{Model: block.From.Model},
		To:   anthropic.BetaFallbackInfoParam{Model: block.To.Model},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling fallback metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": data}, nil
}

func convertFallbackContent(opts provider.ProviderOptions) (*anthropic.BetaFallbackBlockParam, bool) {
	type modelInfo struct {
		Model *string `json:"model"`
	}
	var metadata struct {
		Type *constant.Fallback `json:"type"`
		From *modelInfo         `json:"from"`
		To   *modelInfo         `json:"to"`
	}
	if err := json.Unmarshal(extractRawJSON(opts), &metadata); err != nil ||
		metadata.Type == nil || *metadata.Type != fallbackBlockType ||
		metadata.From == nil || metadata.From.Model == nil ||
		metadata.To == nil || metadata.To.Model == nil {
		return nil, false
	}
	return &anthropic.BetaFallbackBlockParam{
		From: anthropic.BetaFallbackInfoParam{Model: *metadata.From.Model},
		To:   anthropic.BetaFallbackInfoParam{Model: *metadata.To.Model},
	}, true
}
