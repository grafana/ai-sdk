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
		From: fallbackInfo(block.From.Model),
		To:   fallbackInfo(block.To.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling fallback metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": data}, nil
}

func convertFallbackContent(opts provider.ProviderOptions) (*anthropic.BetaFallbackBlockParam, bool) {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(extractRawJSON(opts), &metadata); err != nil {
		return nil, false
	}
	var blockType constant.Fallback
	if err := json.Unmarshal(metadata["type"], &blockType); err != nil || blockType != fallbackBlockType {
		return nil, false
	}
	from, fromValid := fallbackModel(metadata["from"])
	to, toValid := fallbackModel(metadata["to"])
	if !fromValid || !toValid {
		return nil, false
	}
	return &anthropic.BetaFallbackBlockParam{
		From: fallbackInfo(from),
		To:   fallbackInfo(to),
	}, true
}

func fallbackModel(raw json.RawMessage) (string, bool) {
	var info map[string]json.RawMessage
	if err := json.Unmarshal(raw, &info); err != nil {
		return "", false
	}
	var model *string
	if err := json.Unmarshal(info["model"], &model); err != nil || model == nil {
		return "", false
	}
	return *model, true
}

func fallbackInfo(model string) anthropic.BetaFallbackInfoParam {
	info := anthropic.BetaFallbackInfoParam{Model: model}
	info.SetExtraFields(map[string]any{"model": model})
	return info
}
