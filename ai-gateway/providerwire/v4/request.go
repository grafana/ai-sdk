package v4

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type unsupportedCapability string

const (
	capabilityFiles            unsupportedCapability = "files"
	capabilityReasoningContent unsupportedCapability = "reasoning-content"
	capabilityCustomContent    unsupportedCapability = "custom-content"
	capabilityTools            unsupportedCapability = "tools"
	capabilityToolApprovals    unsupportedCapability = "tool-approvals"
	capabilityStructuredOutput unsupportedCapability = "structured-output"
	capabilityProviderOptions  unsupportedCapability = "provider-options"
	capabilityBodyHeaders      unsupportedCapability = "body-headers"
	capabilityRawOutput        unsupportedCapability = "raw-output"
)

type wireRequest struct {
	Prompt           []wireMessage              `json:"prompt"`
	MaxOutputTokens  *int                       `json:"maxOutputTokens"`
	Temperature      *float64                   `json:"temperature"`
	StopSequences    []string                   `json:"stopSequences"`
	TopP             *float64                   `json:"topP"`
	TopK             *int                       `json:"topK"`
	PresencePenalty  *float64                   `json:"presencePenalty"`
	FrequencyPenalty *float64                   `json:"frequencyPenalty"`
	ResponseFormat   *wireResponseFormat        `json:"responseFormat"`
	Seed             *int                       `json:"seed"`
	Tools            []json.RawMessage          `json:"tools"`
	ToolChoice       json.RawMessage            `json:"toolChoice"`
	IncludeRawChunks bool                       `json:"includeRawChunks"`
	Headers          map[string]string          `json:"headers"`
	Reasoning        *wireReasoning             `json:"reasoning"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions"`
}

type wireMessage struct {
	Role            provider.Role              `json:"role"`
	Content         json.RawMessage            `json:"content"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wirePart struct {
	Type            provider.ContentPartType   `json:"type"`
	Text            string                     `json:"text"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wireResponseFormat struct {
	Type provider.ResponseFormatType `json:"type"`
}

type wireReasoning string

const (
	wireReasoningProviderDefault wireReasoning = "provider-default"
	wireReasoningNone            wireReasoning = "none"
	wireReasoningMinimal         wireReasoning = "minimal"
	wireReasoningLow             wireReasoning = "low"
	wireReasoningMedium          wireReasoning = "medium"
	wireReasoningHigh            wireReasoning = "high"
	wireReasoningXHigh           wireReasoning = "xhigh"
)

func mapWireRequest(body []byte) (provider.CallOptions, *requestFailure) {
	var request wireRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}

	options := provider.CallOptions{
		MaxOutputTokens:  request.MaxOutputTokens,
		Temperature:      request.Temperature,
		TopP:             request.TopP,
		TopK:             request.TopK,
		PresencePenalty:  request.PresencePenalty,
		FrequencyPenalty: request.FrequencyPenalty,
		StopSequences:    request.StopSequences,
		Seed:             request.Seed,
	}
	for _, wireMessage := range request.Prompt {
		message, failure := mapWireMessage(wireMessage)
		if failure != nil {
			return provider.CallOptions{}, failure
		}
		options.Prompt = append(options.Prompt, message)
	}

	if len(request.Headers) > 0 {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityBodyHeaders)
	}
	if len(request.Tools) > 0 || len(request.ToolChoice) > 0 {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityTools)
	}
	if request.ResponseFormat != nil {
		switch request.ResponseFormat.Type {
		case provider.ResponseFormatText:
		case provider.ResponseFormatJSON:
			return provider.CallOptions{}, unsupportedMappingFailure(capabilityStructuredOutput)
		default:
			return provider.CallOptions{}, invalidMappingFailure()
		}
	}
	if !providerOptionsEmpty(request.ProviderOptions) {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityProviderOptions)
	}
	if request.IncludeRawChunks {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityRawOutput)
	}
	if request.Reasoning != nil {
		reasoning, err := mapWireReasoning(*request.Reasoning)
		if err != nil {
			return provider.CallOptions{}, invalidMappingFailure()
		}
		options.Reasoning = reasoning
	}
	return options, nil
}

func mapWireMessage(message wireMessage) (provider.Message, *requestFailure) {
	if !providerOptionsEmpty(message.ProviderOptions) {
		return provider.Message{}, unsupportedMappingFailure(capabilityProviderOptions)
	}

	switch message.Role {
	case provider.RoleSystem:
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		return provider.NewSystemMessage(text), nil
	case provider.RoleUser, provider.RoleAssistant:
		var wireParts []wirePart
		if err := json.Unmarshal(message.Content, &wireParts); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		parts := make([]provider.ContentPart, 0, len(wireParts))
		for _, wirePart := range wireParts {
			part, failure := mapWirePart(wirePart)
			if failure != nil {
				return provider.Message{}, failure
			}
			parts = append(parts, part)
		}
		if message.Role == provider.RoleUser {
			return provider.NewUserMessage(parts...), nil
		}
		return provider.NewAssistantMessage(parts...), nil
	case provider.RoleTool:
		var wireParts []wirePart
		if err := json.Unmarshal(message.Content, &wireParts); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		for _, part := range wireParts {
			if part.Type == provider.ContentPartTypeToolApprovalResponse {
				return provider.Message{}, unsupportedMappingFailure(capabilityToolApprovals)
			}
		}
		return provider.Message{}, unsupportedMappingFailure(capabilityTools)
	default:
		return provider.Message{}, invalidMappingFailure()
	}
}

func mapWirePart(part wirePart) (provider.ContentPart, *requestFailure) {
	if !providerOptionsEmpty(part.ProviderOptions) {
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityProviderOptions)
	}
	switch part.Type {
	case provider.ContentPartTypeText:
		return provider.TextPart(part.Text), nil
	case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityFiles)
	case provider.ContentPartTypeReasoning:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityReasoningContent)
	case provider.ContentPartTypeCustom:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityCustomContent)
	case provider.ContentPartTypeToolCall, provider.ContentPartTypeToolResult:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityTools)
	case provider.ContentPartTypeToolApprovalResponse, provider.ContentPartTypeToolApprovalRequest:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityToolApprovals)
	default:
		return provider.ContentPart{}, invalidMappingFailure()
	}
}

func providerOptionsEmpty(options map[string]json.RawMessage) bool {
	for _, raw := range options {
		var namespace map[string]json.RawMessage
		if err := json.Unmarshal(raw, &namespace); err != nil || len(namespace) > 0 {
			return false
		}
	}
	return true
}

func mapWireReasoning(value wireReasoning) (provider.ReasoningEffort, error) {
	switch value {
	case wireReasoningProviderDefault:
		return provider.ReasoningProviderDefault, nil
	case wireReasoningNone:
		return provider.ReasoningNone, nil
	case wireReasoningMinimal:
		return provider.ReasoningMinimal, nil
	case wireReasoningLow:
		return provider.ReasoningLow, nil
	case wireReasoningMedium:
		return provider.ReasoningMedium, nil
	case wireReasoningHigh:
		return provider.ReasoningHigh, nil
	case wireReasoningXHigh:
		return provider.ReasoningXHigh, nil
	default:
		return provider.ReasoningProviderDefault, fmt.Errorf("unknown reasoning value")
	}
}
