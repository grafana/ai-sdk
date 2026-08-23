package v4

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

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

func validUnsupportedCapability(capability unsupportedCapability) bool {
	switch capability {
	case capabilityFiles,
		capabilityReasoningContent,
		capabilityCustomContent,
		capabilityTools,
		capabilityToolApprovals,
		capabilityStructuredOutput,
		capabilityProviderOptions,
		capabilityBodyHeaders,
		capabilityRawOutput:
		return true
	default:
		return false
	}
}

type wireRequest struct {
	Prompt           []json.RawMessage          `json:"prompt"`
	MaxOutputTokens  json.RawMessage            `json:"maxOutputTokens"`
	Temperature      json.RawMessage            `json:"temperature"`
	StopSequences    []string                   `json:"stopSequences"`
	TopP             json.RawMessage            `json:"topP"`
	TopK             json.RawMessage            `json:"topK"`
	PresencePenalty  json.RawMessage            `json:"presencePenalty"`
	FrequencyPenalty json.RawMessage            `json:"frequencyPenalty"`
	ResponseFormat   json.RawMessage            `json:"responseFormat"`
	Seed             json.RawMessage            `json:"seed"`
	Tools            []json.RawMessage          `json:"tools"`
	ToolChoice       json.RawMessage            `json:"toolChoice"`
	IncludeRawChunks json.RawMessage            `json:"includeRawChunks"`
	Headers          map[string]string          `json:"headers"`
	Reasoning        json.RawMessage            `json:"reasoning"`
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
	Output          json.RawMessage            `json:"output"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wireTool struct {
	Type            provider.ToolType          `json:"type"`
	ProviderOptions map[string]json.RawMessage `json:"providerOptions"`
}

type wireToolChoice struct {
	Type provider.ToolChoiceType `json:"type"`
}

type wireToolResultOutput struct {
	Type            provider.ToolResultOutputType `json:"type"`
	ProviderOptions map[string]json.RawMessage    `json:"providerOptions"`
	Value           json.RawMessage               `json:"value"`
}

type wireToolResultContent struct {
	Type            provider.ToolResultContentType `json:"type"`
	ProviderOptions map[string]json.RawMessage     `json:"providerOptions"`
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

	options := provider.CallOptions{}
	for _, rawMessage := range request.Prompt {
		message, failure := mapWireMessage(rawMessage)
		if failure != nil {
			return provider.CallOptions{}, failure
		}
		options.Prompt = append(options.Prompt, message)
	}

	if len(request.Headers) > 0 {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityBodyHeaders)
	}
	for _, rawTool := range request.Tools {
		tool, failure := inspectWireTool(rawTool)
		if failure != nil {
			return provider.CallOptions{}, failure
		}
		if !providerOptionsEmpty(tool.ProviderOptions) {
			return provider.CallOptions{}, unsupportedMappingFailure(capabilityProviderOptions)
		}
	}
	if len(request.Tools) > 0 {
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityTools)
	}
	if len(request.ToolChoice) > 0 {
		if failure := inspectWireToolChoice(request.ToolChoice); failure != nil {
			return provider.CallOptions{}, failure
		}
		return provider.CallOptions{}, unsupportedMappingFailure(capabilityTools)
	}
	if len(request.ResponseFormat) > 0 {
		var format wireResponseFormat
		if err := json.Unmarshal(request.ResponseFormat, &format); err != nil {
			return provider.CallOptions{}, invalidMappingFailure()
		}
		switch format.Type {
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
	if len(request.IncludeRawChunks) > 0 {
		var include bool
		if err := json.Unmarshal(request.IncludeRawChunks, &include); err != nil {
			return provider.CallOptions{}, invalidMappingFailure()
		}
		if include {
			return provider.CallOptions{}, unsupportedMappingFailure(capabilityRawOutput)
		}
	}

	var err error
	if options.MaxOutputTokens, err = parseWireInt(request.MaxOutputTokens); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.TopK, err = parseWireInt(request.TopK); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.Seed, err = parseWireInt(request.Seed); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.Temperature, err = parseWireFloat(request.Temperature); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.TopP, err = parseWireFloat(request.TopP); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.PresencePenalty, err = parseWireFloat(request.PresencePenalty); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if options.FrequencyPenalty, err = parseWireFloat(request.FrequencyPenalty); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}
	if len(request.StopSequences) > 0 {
		options.StopSequences = append([]string(nil), request.StopSequences...)
	}
	if len(request.Reasoning) > 0 {
		if options.Reasoning, err = parseWireReasoning(request.Reasoning); err != nil {
			return provider.CallOptions{}, invalidMappingFailure()
		}
	}
	return options, nil
}

func mapWireMessage(raw json.RawMessage) (provider.Message, *requestFailure) {
	var message wireMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return provider.Message{}, invalidMappingFailure()
	}
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
		var rawParts []json.RawMessage
		if err := json.Unmarshal(message.Content, &rawParts); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		parts := make([]provider.ContentPart, 0, len(rawParts))
		for _, rawPart := range rawParts {
			part, failure := mapWirePart(rawPart)
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
		var rawParts []json.RawMessage
		if err := json.Unmarshal(message.Content, &rawParts); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		for _, rawPart := range rawParts {
			if failure := inspectWireToolMessagePart(rawPart); failure != nil {
				return provider.Message{}, failure
			}
		}
		return provider.Message{}, unsupportedMappingFailure(capabilityTools)
	default:
		return provider.Message{}, invalidMappingFailure()
	}
}

func mapWirePart(raw json.RawMessage) (provider.ContentPart, *requestFailure) {
	var part wirePart
	if err := json.Unmarshal(raw, &part); err != nil {
		return provider.ContentPart{}, invalidMappingFailure()
	}
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
	case provider.ContentPartTypeToolCall:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityTools)
	case provider.ContentPartTypeToolResult:
		if failure := inspectWireToolResultOutput(part.Output); failure != nil {
			return provider.ContentPart{}, failure
		}
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityTools)
	case provider.ContentPartTypeToolApprovalResponse, provider.ContentPartTypeToolApprovalRequest:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityToolApprovals)
	default:
		return provider.ContentPart{}, invalidMappingFailure()
	}
}

func inspectWireTool(raw json.RawMessage) (wireTool, *requestFailure) {
	var tool wireTool
	if err := json.Unmarshal(raw, &tool); err != nil {
		return wireTool{}, invalidMappingFailure()
	}
	switch tool.Type {
	case provider.ToolTypeFunction, provider.ToolTypeProvider:
		return tool, nil
	default:
		return wireTool{}, invalidMappingFailure()
	}
}

func inspectWireToolChoice(raw json.RawMessage) *requestFailure {
	var choice wireToolChoice
	if err := json.Unmarshal(raw, &choice); err != nil {
		return invalidMappingFailure()
	}
	switch choice.Type {
	case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired, provider.ToolChoiceTool:
		return nil
	default:
		return invalidMappingFailure()
	}
}

func inspectWireToolMessagePart(raw json.RawMessage) *requestFailure {
	var part wirePart
	if err := json.Unmarshal(raw, &part); err != nil {
		return invalidMappingFailure()
	}
	if !providerOptionsEmpty(part.ProviderOptions) {
		return unsupportedMappingFailure(capabilityProviderOptions)
	}
	switch part.Type {
	case provider.ContentPartTypeToolResult:
		if failure := inspectWireToolResultOutput(part.Output); failure != nil {
			return failure
		}
		return unsupportedMappingFailure(capabilityTools)
	case provider.ContentPartTypeToolApprovalResponse:
		return unsupportedMappingFailure(capabilityToolApprovals)
	default:
		return invalidMappingFailure()
	}
}

func inspectWireToolResultOutput(raw json.RawMessage) *requestFailure {
	var output wireToolResultOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return invalidMappingFailure()
	}
	if !providerOptionsEmpty(output.ProviderOptions) {
		return unsupportedMappingFailure(capabilityProviderOptions)
	}
	switch output.Type {
	case provider.ToolOutputText,
		provider.ToolOutputJSON,
		provider.ToolOutputExecutionDenied,
		provider.ToolOutputErrorText,
		provider.ToolOutputErrorJSON:
		return nil
	case provider.ToolOutputContent:
		var rawParts []json.RawMessage
		if err := json.Unmarshal(output.Value, &rawParts); err != nil {
			return invalidMappingFailure()
		}
		for _, rawPart := range rawParts {
			var part wireToolResultContent
			if err := json.Unmarshal(rawPart, &part); err != nil {
				return invalidMappingFailure()
			}
			if !providerOptionsEmpty(part.ProviderOptions) {
				return unsupportedMappingFailure(capabilityProviderOptions)
			}
			switch part.Type {
			case provider.ToolContentText, provider.ToolContentFile, provider.ToolContentCustom:
			default:
				return invalidMappingFailure()
			}
		}
		return nil
	default:
		return invalidMappingFailure()
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

func parseWireInt(raw json.RawMessage) (*int, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	value, err := strconv.ParseInt(string(raw), 10, strconv.IntSize)
	if err != nil {
		return nil, fmt.Errorf("mapping integer: %w", err)
	}
	result := int(value)
	return &result, nil
}

func parseWireFloat(raw json.RawMessage) (*float64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	value, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("mapping finite number")
	}
	return &value, nil
}

func parseWireReasoning(raw json.RawMessage) (provider.ReasoningEffort, error) {
	var value wireReasoning
	if err := json.Unmarshal(raw, &value); err != nil {
		return provider.ReasoningProviderDefault, err
	}
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
