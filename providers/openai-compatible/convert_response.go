package openaicompatible

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

func parseGenerateResponse(body []byte, headers http.Header, providerName, metadataKey string, generateID func() string) (*provider.GenerateResult, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openai: decoding response: %w", err)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0] == nil {
		return nil, fmt.Errorf("openai: response contained no choices")
	}

	choice := parsed.Choices[0]
	content, err := convertOpenAICompatibleContent(choice.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("openai: decoding response content: %w", err)
	}
	if choice.Message.ReasoningContent != "" {
		content = append(content, provider.GenerateContentPart{
			Type: provider.ContentReasoning,
			Text: choice.Message.ReasoningContent,
		})
	} else if choice.Message.Reasoning != "" {
		content = append(content, provider.GenerateContentPart{
			Type: provider.ContentReasoning,
			Text: choice.Message.Reasoning,
		})
	}
	for _, toolCall := range choice.Message.ToolCalls {
		id := toolCall.ID
		if id == "" {
			id = generateID()
		}
		input := strings.TrimSpace(toolCall.Function.Arguments)
		if input == "" {
			input = "{}"
		}
		part := provider.GenerateContentPart{
			Type:       provider.ContentToolCall,
			ToolCallID: id,
			ToolName:   toolCall.Function.Name,
			Input:      json.RawMessage(input),
		}
		if meta := toolCallProviderMetadata(metadataKey, toolCall.ExtraContent); meta != nil {
			part.ProviderMetadata = meta
		}
		content = append(content, part)
	}

	return &provider.GenerateResult{
		Content:          content,
		FinishReason:     mapFinishReason(choice.FinishReason),
		Usage:            convertUsage(parsed.Usage),
		ProviderMetadata: responseProviderMetadata(metadataKey, parsed.Usage),
		Response: &provider.GenerateResponse{
			ResponseMetadata: responseMetadata(parsed.ID, parsed.Model, providerName, parsed.Created),
			Headers:          flattenHeaders(headers),
			Body:             json.RawMessage(append([]byte(nil), body...)),
		},
	}, nil
}

func convertOpenAICompatibleContent(raw json.RawMessage) ([]provider.GenerateContentPart, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return nil, nil
		}
		return []provider.GenerateContentPart{{Type: provider.ContentText, Text: text}}, nil
	}

	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, errors.New("content must be a string or an array of content parts")
	}
	content := make([]provider.GenerateContentPart, 0, len(parts))
	for _, part := range parts {
		var partType string
		if err := json.Unmarshal(part["type"], &partType); err != nil {
			return nil, errors.New("content part type must be a string")
		}
		switch partType {
		case "text":
			var value string
			if json.Unmarshal(part["text"], &value) == nil && value != "" {
				content = append(content, provider.GenerateContentPart{Type: provider.ContentText, Text: value})
			}
		case "thinking":
			var chunks []json.RawMessage
			if json.Unmarshal(part["thinking"], &chunks) != nil {
				continue
			}
			var reasoning strings.Builder
			for _, rawChunk := range chunks {
				var chunk map[string]json.RawMessage
				if json.Unmarshal(rawChunk, &chunk) != nil {
					continue
				}
				var chunkType, value string
				if json.Unmarshal(chunk["type"], &chunkType) == nil && chunkType == "text" && json.Unmarshal(chunk["text"], &value) == nil {
					reasoning.WriteString(value)
				}
			}
			if reasoning.Len() > 0 {
				content = append(content, provider.GenerateContentPart{Type: provider.ContentReasoning, Text: reasoning.String()})
			}
		}
	}
	return content, nil
}

func resolveMetadataKey(opts provider.ProviderOptions, providerName string) string {
	rawName := providerOptionsName(providerName)
	camelName := toCamelCase(rawName)
	if camelName != rawName {
		if _, ok := opts[camelName]; ok {
			return camelName
		}
	}
	return rawName
}

func responseMetadata(id, modelID, providerName string, created *int64) provider.ResponseMetadata {
	md := provider.ResponseMetadata{
		ID:       id,
		ModelID:  modelID,
		Provider: providerName,
	}
	if created != nil {
		md.Timestamp = time.Unix(*created, 0).UTC()
	}
	return md
}

func mapFinishReason(reason *string) provider.FinishReason {
	if reason == nil {
		return provider.FinishReason{Unified: provider.FinishReasonOther}
	}
	switch *reason {
	case "stop":
		return provider.FinishReason{Unified: provider.FinishReasonStop, Raw: *reason}
	case "length":
		return provider.FinishReason{Unified: provider.FinishReasonLength, Raw: *reason}
	case "content_filter":
		return provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: *reason}
	case "function_call", "tool_calls":
		return provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: *reason}
	default:
		return provider.FinishReason{Unified: provider.FinishReasonOther, Raw: *reason}
	}
}

func convertUsage(usage *openAIUsage) provider.Usage {
	if usage == nil {
		return provider.Usage{}
	}

	inputTotal := intValue(usage.PromptTokens)
	outputTotal := intValue(usage.CompletionTokens)
	cacheRead := 0
	if usage.PromptTokensDetails != nil {
		cacheRead = intValue(usage.PromptTokensDetails.CachedTokens)
	}
	reasoning := 0
	if usage.CompletionTokensDetails != nil {
		reasoning = intValue(usage.CompletionTokensDetails.ReasoningTokens)
	}

	raw := json.RawMessage(append([]byte(nil), usage.Raw...))
	if len(raw) == 0 {
		raw, _ = json.Marshal(usage)
	}
	return provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:     ptr(inputTotal),
			NoCache:   ptr(inputTotal - cacheRead),
			CacheRead: ptr(cacheRead),
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     ptr(outputTotal),
			Text:      ptr(max(0, outputTotal-reasoning)),
			Reasoning: ptr(reasoning),
		},
		Raw: raw,
	}
}

func responseProviderMetadata(metadataKey string, usage *openAIUsage) provider.ProviderMetadata {
	if metadataKey == "" {
		return nil
	}

	payload := map[string]any{}
	if usage != nil && usage.CompletionTokensDetails != nil {
		if usage.CompletionTokensDetails.AcceptedPredictionTokens != nil {
			payload["acceptedPredictionTokens"] = *usage.CompletionTokensDetails.AcceptedPredictionTokens
		}
		if usage.CompletionTokensDetails.RejectedPredictionTokens != nil {
			payload["rejectedPredictionTokens"] = *usage.CompletionTokensDetails.RejectedPredictionTokens
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return provider.ProviderMetadata{metadataKey: raw}
}

func toolCallProviderMetadata(metadataKey string, extra *extraContent) provider.ProviderMetadata {
	if metadataKey == "" || extra == nil || extra.Google == nil || extra.Google.ThoughtSignature == "" {
		return nil
	}

	raw, err := json.Marshal(map[string]string{
		"thoughtSignature": extra.Google.ThoughtSignature,
	})
	if err != nil {
		return nil
	}
	return provider.ProviderMetadata{metadataKey: raw}
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func ptr(v int) *int { return &v }

func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}
