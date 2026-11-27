package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"
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

func compactionReplayMetadata(opts provider.ProviderOptions) (signature *string, ok bool) {
	raw := extractRawJSON(opts)
	if raw == nil {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, false
	}
	var kind string
	if json.Unmarshal(fields["type"], &kind) != nil || kind != "compaction" {
		return nil, false
	}
	if rawSig := fields["signature"]; rawSig != nil {
		if json.Unmarshal(rawSig, &signature) != nil {
			return nil, true
		}
	}
	return signature, true
}

type compactionBlock struct {
	Content   string
	Signature *string
}

func decodeCompactionBlock(raw string) (compactionBlock, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return compactionBlock{}, fmt.Errorf("decoding compaction block: %w", err)
	}
	content, err := decodeCompactionContent(fields["content"])
	if err != nil {
		return compactionBlock{}, err
	}
	block := compactionBlock{Content: content}
	if signature := fields["signature"]; signature != nil {
		if err := json.Unmarshal(signature, &block.Signature); err != nil {
			return compactionBlock{}, fmt.Errorf("decoding compaction signature: %w", err)
		}
	}
	return block, nil
}

func decodeCompactionContent(raw json.RawMessage) (string, error) {
	var content string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &content); err != nil {
			return "", fmt.Errorf("decoding compaction content: %w", err)
		}
	}
	return content, nil
}

func (block compactionBlock) metadata() (provider.ProviderMetadata, error) {
	metadata := map[string]string{"type": "compaction"}
	if block.Signature != nil {
		metadata["signature"] = *block.Signature
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshaling compaction metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": raw}, nil
}
