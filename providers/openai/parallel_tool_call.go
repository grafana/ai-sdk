package openai

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

const parallelToolName = "parallel"

// OpenAIParallelToolCall preserves an expanded OpenAI parallel wrapper for continuation.
type OpenAIParallelToolCall struct {
	ItemID     string `json:"itemId"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Input      string `json:"input"`
	Index      int    `json:"index"`
	Count      int    `json:"count"`
}

type expandedParallelToolCall struct {
	toolCallID       string
	toolName         string
	input            string
	providerMetadata provider.ProviderMetadata
}

type parallelToolResultGroup struct {
	metadata      OpenAIParallelToolCall
	results       map[int]provider.ContentPart
	callEmitted   bool
	resultEmitted bool
	invalid       bool
}

func isUndeclaredParallelToolCall(name string, tools []provider.Tool) bool {
	if name != parallelToolName {
		return false
	}
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeFunction && tool.Name == parallelToolName {
			return false
		}
	}
	return true
}

func expandParallelToolCall(toolCallID, toolName, input, itemID, metadataKey string, tools []provider.Tool) ([]expandedParallelToolCall, bool) {
	if !isUndeclaredParallelToolCall(toolName, tools) {
		return nil, false
	}
	var payload struct {
		ToolUses []struct {
			RecipientName string         `json:"recipient_name"`
			Parameters    map[string]any `json:"parameters"`
		} `json:"tool_uses"`
	}
	if err := json.Unmarshal([]byte(input), &payload); err != nil || len(payload.ToolUses) == 0 {
		return nil, false
	}
	available := make(map[string]struct{})
	for _, tool := range tools {
		if tool.Type == provider.ToolTypeFunction {
			available[tool.Name] = struct{}{}
		}
	}
	calls := make([]expandedParallelToolCall, 0, len(payload.ToolUses))
	for index, use := range payload.ToolUses {
		if !strings.HasPrefix(use.RecipientName, "functions.") || use.Parameters == nil {
			return nil, false
		}
		name := strings.TrimPrefix(use.RecipientName, "functions.")
		if name == "" {
			return nil, false
		}
		if _, ok := available[name]; !ok {
			return nil, false
		}
		arguments, err := json.Marshal(use.Parameters)
		if err != nil {
			return nil, false
		}
		meta := OpenAIParallelToolCall{
			ItemID: itemID, ToolCallID: toolCallID, ToolName: toolName,
			Input: input, Index: index, Count: len(payload.ToolUses),
		}
		raw, err := json.Marshal(map[string]any{"parallelToolCall": meta})
		if err != nil {
			return nil, false
		}
		calls = append(calls, expandedParallelToolCall{
			toolCallID:       toolCallID + "_" + strconv.Itoa(index),
			toolName:         name,
			input:            string(arguments),
			providerMetadata: provider.ProviderMetadata{metadataKey: raw},
		})
	}
	return calls, true
}

func parallelMetadata(options OpenAIPartOptions) (OpenAIParallelToolCall, bool) {
	meta := options.ParallelToolCall
	if meta == nil || meta.Index < 0 || meta.Count <= meta.Index {
		return OpenAIParallelToolCall{}, false
	}
	return *meta, true
}

func sameParallelToolCall(first, second OpenAIParallelToolCall) bool {
	return first.ItemID == second.ItemID && first.ToolCallID == second.ToolCallID && first.ToolName == second.ToolName && first.Input == second.Input && first.Count == second.Count
}

func collectParallelToolResultGroups(prompt []provider.Message, providerOptionsName string, enabled bool) map[string]*parallelToolResultGroup {
	groups := make(map[string]*parallelToolResultGroup)
	if !enabled {
		return groups
	}
	for _, message := range prompt {
		if message.Role != provider.RoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolResult {
				continue
			}
			meta, ok := parallelMetadata(openAIPartOptionsFor(part.ProviderOptions, providerOptionsName))
			if !ok {
				continue
			}
			group := groups[meta.ToolCallID]
			if group == nil {
				groups[meta.ToolCallID] = &parallelToolResultGroup{metadata: meta, results: map[int]provider.ContentPart{meta.Index: part}}
				continue
			}
			if !sameParallelToolCall(group.metadata, meta) {
				group.invalid = true
				continue
			}
			if _, exists := group.results[meta.Index]; exists {
				group.invalid = true
				continue
			}
			group.results[meta.Index] = part
		}
	}
	for id, group := range groups {
		if group.invalid || len(group.results) != group.metadata.Count {
			delete(groups, id)
			continue
		}
		for index := 0; index < group.metadata.Count; index++ {
			if _, ok := group.results[index]; !ok {
				delete(groups, id)
				break
			}
		}
	}
	return groups
}
