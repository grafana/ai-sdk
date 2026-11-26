package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

const compactionBeta = anthropic.AnthropicBetaCompact2026_09_04

func resolveCompactionRequest(config *CompactionConfig, raw json.RawMessage) (anthropic.BetaCompactionConfigUnionParam, error) {
	var params anthropic.BetaCompactionConfigUnionParam
	if !isJSONNullOrAbsent(raw) {
		var fields struct {
			Compaction        json.RawMessage `json:"compaction"`
			ContextManagement json.RawMessage `json:"contextManagement"`
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return params, err
		}
		if len(fields.Compaction) > 0 && isJSONNullOrAbsent(fields.Compaction) {
			return params, errors.New("compaction must be an object")
		}
		if config != nil {
			if !isJSONNullOrAbsent(fields.ContextManagement) {
				return params, errors.New("`compaction` and `contextManagement` cannot be used together")
			}
			var compaction struct {
				Instructions json.RawMessage `json:"instructions"`
			}
			if err := json.Unmarshal(fields.Compaction, &compaction); err != nil {
				return params, err
			}
			if len(compaction.Instructions) > 0 && isJSONNullOrAbsent(compaction.Instructions) {
				return params, errors.New("compaction.instructions must be a string")
			}
		}
	}
	if config == nil {
		return params, nil
	}
	if config.Type != CompactionSummarize {
		return params, errors.New(`compaction.type must be "summarize"`)
	}
	params.OfSummarize = &anthropic.BetaSummarizeCompactionParam{}
	if config.Instructions != nil {
		params.OfSummarize.Instructions = anthropic.String(*config.Instructions)
	}
	return params, nil
}

func compactionMetadata(block anthropic.BetaCompactionBlock) (provider.ProviderMetadata, error) {
	metadata := map[string]string{"type": "compaction"}
	if block.JSON.Signature.Valid() {
		metadata["signature"] = block.Signature
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshaling compaction metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": raw}, nil
}
