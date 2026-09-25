package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

const parallelToolName = "parallel"

type parallelToolCallMetadata struct {
	ItemID     string `json:"itemId"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Input      string `json:"input"`
	Index      int    `json:"index"`
	Count      int    `json:"count"`
}

func (b buildResult) isUndeclaredParallelTool(name string) bool {
	_, declared := b.functionTools[parallelToolName]
	return name == parallelToolName && !declared
}

func (b buildResult) expandParallelToolCall(call responses.ResponseFunctionToolCall, namespace string) []provider.GenerateContentPart {
	if !b.isUndeclaredParallelTool(call.Name) {
		return nil
	}
	var input struct {
		ToolUses []struct {
			RecipientName string          `json:"recipient_name"`
			Parameters    json.RawMessage `json:"parameters"`
		} `json:"tool_uses"`
	}
	if json.Unmarshal([]byte(call.Arguments), &input) != nil || len(input.ToolUses) == 0 {
		return nil
	}
	parts := make([]provider.GenerateContentPart, 0, len(input.ToolUses))
	for index, use := range input.ToolUses {
		name, prefixed := strings.CutPrefix(use.RecipientName, "functions.")
		if _, available := b.functionTools[name]; !prefixed || name == "" || !available {
			return nil
		}
		parameters := bytes.TrimSpace(use.Parameters)
		if len(parameters) == 0 || parameters[0] != '{' {
			return nil
		}
		var compact bytes.Buffer
		if json.Compact(&compact, parameters) != nil {
			return nil
		}
		metadata, err := json.Marshal(struct {
			ParallelToolCall parallelToolCallMetadata `json:"parallelToolCall"`
		}{ParallelToolCall: parallelToolCallMetadata{
			ItemID: call.ID, ToolCallID: call.CallID, ToolName: call.Name,
			Input: call.Arguments, Index: index, Count: len(input.ToolUses),
		}})
		if err != nil {
			return nil
		}
		parts = append(parts, provider.GenerateContentPart{
			Type: provider.ContentToolCall, ToolCallID: fmt.Sprintf("%s_%d", call.CallID, index),
			ToolName: name, Input: append(json.RawMessage(nil), compact.Bytes()...),
			ProviderMetadata: provider.ProviderMetadata{namespace: metadata},
		})
	}
	return parts
}

type parallelToolResultGroup struct {
	metadata parallelToolCallMetadata
	results  map[int]provider.ContentPart
	invalid  bool
}

func parallelMetadata(part provider.ContentPart, namespace string) *parallelToolCallMetadata {
	options, ok, err := provider.ResolveOption[map[string]json.RawMessage](part.ProviderOptions, namespace)
	if err != nil || !ok {
		return nil
	}
	raw := options["parallelToolCall"]
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	for _, key := range []string{"itemId", "toolCallId", "toolName", "input", "index", "count"} {
		if len(fields[key]) == 0 || isJSONNull(fields[key]) {
			return nil
		}
	}
	var metadata parallelToolCallMetadata
	if json.Unmarshal(raw, &metadata) != nil || metadata.Index < 0 || metadata.Count <= metadata.Index {
		return nil
	}
	return &metadata
}

func sameParallelCall(left, right parallelToolCallMetadata) bool {
	left.Index, right.Index = 0, 0
	return left == right
}

func collectParallelResults(prompt []provider.Message, namespace string) map[string]*parallelToolResultGroup {
	groups := make(map[string]*parallelToolResultGroup)
	for _, message := range prompt {
		if message.Role != provider.RoleTool {
			continue
		}
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolResult {
				continue
			}
			metadata := parallelMetadata(part, namespace)
			if metadata == nil {
				continue
			}
			group := groups[metadata.ToolCallID]
			if group == nil {
				group = &parallelToolResultGroup{metadata: *metadata, results: make(map[int]provider.ContentPart)}
				groups[metadata.ToolCallID] = group
			}
			_, duplicate := group.results[metadata.Index]
			if duplicate || !sameParallelCall(group.metadata, *metadata) {
				group.invalid = true
				continue
			}
			group.results[metadata.Index] = part
		}
	}
	for id, group := range groups {
		if group.invalid || len(group.results) != group.metadata.Count {
			delete(groups, id)
		}
	}
	return groups
}

func (c inputConversionContext) parallelGroup(part provider.ContentPart) *parallelToolResultGroup {
	metadata := parallelMetadata(part, c.providerOptionsName)
	if metadata == nil {
		return nil
	}
	group := c.parallelResults[metadata.ToolCallID]
	if group == nil || !sameParallelCall(group.metadata, *metadata) {
		return nil
	}
	return group
}

func (g *parallelToolResultGroup) output(ctx inputConversionContext) (*responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	texts := make([]string, g.metadata.Count)
	breakpoints := make([]*PromptCacheBreakpoint, g.metadata.Count)
	var warnings []provider.Warning
	hasBreakpoint := false
	for index := range texts {
		part := g.results[index]
		if part.Output == nil || part.Output.Type != provider.ToolOutputContent {
			texts[index] = toolResultOutputString(part.Output, ctx.hasOutputSchema(part.ToolName))
			breakpoints[index] = ctx.scalarResultBreakpoint(part)
			hasBreakpoint = hasBreakpoint || breakpoints[index] != nil
			continue
		}
		_, content, outputWarnings, err := convertFunctionResultOutput(part, ctx)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, outputWarnings...)
		encoded, err := json.Marshal(content)
		if err != nil {
			return nil, nil, fmt.Errorf("openai: marshaling parallel tool result: %w", err)
		}
		texts[index] = string(encoded)
	}
	if !hasBreakpoint {
		item := responses.ResponseInputItemParamOfFunctionCallOutput(g.metadata.ToolCallID, strings.Join(texts, "\n"))
		return &item, warnings, nil
	}
	content := make(responses.ResponseFunctionCallOutputItemListParam, len(texts))
	for index, text := range texts {
		if index > 0 {
			text = "\n" + text
		}
		part := responses.ResponseInputTextContentParam{Text: text}
		if breakpoints[index] != nil {
			part.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoints[index]})
		}
		content[index] = responses.ResponseFunctionCallOutputItemUnionParam{OfInputText: &part}
	}
	item := responses.ResponseInputItemParamOfFunctionCallOutput(g.metadata.ToolCallID, content)
	return &item, warnings, nil
}
