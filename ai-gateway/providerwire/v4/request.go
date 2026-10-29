package v4

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

type requestFailureReason string

const (
	capabilityTools               requestFailureReason = "tools"
	capabilityToolApprovals       requestFailureReason = "tool-approvals"
	capabilityStructuredOutput    requestFailureReason = "structured-output"
	capabilityProviderOptions     requestFailureReason = "provider-options"
	policyReservedProviderOptions requestFailureReason = "reserved-provider-options"
	policyProtectedCallHeader     requestFailureReason = "protected-call-header"
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
	ApprovalID       string                     `json:"approvalId"`
	Approved         *bool                      `json:"approved"`
	Reason           string                     `json:"reason"`
	Type             provider.ContentPartType   `json:"type"`
	Kind             string                     `json:"kind"`
	Text             string                     `json:"text"`
	Data             json.RawMessage            `json:"data"`
	MediaType        string                     `json:"mediaType"`
	Filename         *string                    `json:"filename"`
	ProviderOptions  map[string]json.RawMessage `json:"providerOptions"`
	ToolCallID       string                     `json:"toolCallId"`
	ToolName         string                     `json:"toolName"`
	Input            json.RawMessage            `json:"input"`
	Output           *wireToolOutput            `json:"output"`
	ProviderExecuted bool                       `json:"providerExecuted"`
}

type wireResponseFormat struct {
	Type        provider.ResponseFormatType `json:"type"`
	Schema      json.RawMessage             `json:"schema"`
	Name        *string                     `json:"name"`
	Description string                      `json:"description"`
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
	wireReasoningMax             wireReasoning = "max"
)

func mapWireRequest(body []byte) (provider.CallOptions, *requestFailure) {
	var request wireRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return provider.CallOptions{}, invalidMappingFailure()
	}

	return mapRequest(request)
}

func mapRequest(request wireRequest) (provider.CallOptions, *requestFailure) {
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

	headers, failure := mapWireHeaders(request.Headers)
	if failure != nil {
		return provider.CallOptions{}, failure
	}
	options.Headers = headers
	if len(request.Tools) > 0 || len(request.ToolChoice) > 0 {
		tools, choice, failure := mapFunctionTools(request.Tools, request.ToolChoice)
		if failure != nil {
			return provider.CallOptions{}, failure
		}
		options.Tools, options.ToolChoice = tools, choice
	}
	if request.ResponseFormat != nil {
		switch request.ResponseFormat.Type {
		case provider.ResponseFormatText:
		case provider.ResponseFormatJSON:
			options.ResponseFormat = &provider.ResponseFormat{Type: request.ResponseFormat.Type, Schema: request.ResponseFormat.Schema, Name: request.ResponseFormat.Name, Description: request.ResponseFormat.Description}
		default:
			return provider.CallOptions{}, invalidMappingFailure()
		}
	}
	rootOptions, failure := mapWireProviderOptions(request.ProviderOptions)
	if failure != nil {
		return provider.CallOptions{}, failure
	}
	options.ProviderOptions = rootOptions
	options.IncludeRawChunks = request.IncludeRawChunks
	if request.Reasoning != nil {
		reasoning, err := mapWireReasoning(*request.Reasoning)
		if err != nil {
			return provider.CallOptions{}, invalidMappingFailure()
		}
		options.Reasoning = reasoning
	}
	return options, nil
}

func unresolvedProviderCalls(prompt []provider.Message) (map[string]string, *requestFailure) {
	history, err := execution.RebuildToolHistory(prompt)
	if err != nil {
		return nil, invalidMappingFailure()
	}
	return map[string]string(history), nil
}

func mapWireMessage(message wireMessage) (provider.Message, *requestFailure) {
	messageOptions, failure := mapWireProviderOptions(message.ProviderOptions)
	if failure != nil {
		return provider.Message{}, failure
	}

	switch message.Role {
	case provider.RoleSystem:
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		system := provider.NewSystemMessage(text)
		system.ProviderOptions = messageOptions
		return system, nil
	case provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		var wireParts []wirePart
		if err := json.Unmarshal(message.Content, &wireParts); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		parts := make([]provider.ContentPart, 0, len(wireParts))
		for _, wirePart := range wireParts {
			part, failure := mapWirePart(wirePart, message.Role)
			if failure != nil {
				return provider.Message{}, failure
			}
			parts = append(parts, part)
		}
		var mapped provider.Message
		switch message.Role {
		case provider.RoleUser:
			mapped = provider.NewUserMessage(parts...)
		case provider.RoleTool:
			mapped = provider.NewToolMessage(parts...)
		default:
			mapped = provider.NewAssistantMessage(parts...)
		}
		mapped.ProviderOptions = messageOptions
		return mapped, nil
	default:
		return provider.Message{}, invalidMappingFailure()
	}
}

func mapWirePart(part wirePart, role provider.Role) (provider.ContentPart, *requestFailure) {
	// Options are mapped inside the branch that keeps them, so a part type this
	// runtime does not support reports its own family whatever options it carries.
	switch part.Type {
	case provider.ContentPartTypeText:
		partOptions, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		text := provider.TextPart(part.Text)
		text.ProviderOptions = partOptions
		return text, nil
	case provider.ContentPartTypeFile:
		partOptions, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		data, failure := mapWireFileData(part.Data)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		file := provider.FilePart(part.MediaType, data)
		file.Filename = part.Filename
		file.ProviderOptions = partOptions
		return file, nil
	case provider.ContentPartTypeReasoningFile, provider.ContentPartTypeReasoning:
		if role != provider.RoleAssistant {
			return provider.ContentPart{}, invalidMappingFailure()
		}
		options, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		mapped := provider.ReasoningPart(part.Text)
		if part.Type == provider.ContentPartTypeReasoningFile {
			data, failure := mapWireFileData(part.Data)
			if failure != nil || (!data.IsData() && !data.IsURL()) {
				return provider.ContentPart{}, invalidMappingFailure()
			}
			mapped = provider.ReasoningFilePart(part.MediaType, data)
		}
		mapped.ProviderOptions = options
		return mapped, nil
	case provider.ContentPartTypeCustom:
		if role != provider.RoleAssistant || !validCustomKind(part.Kind) {
			return provider.ContentPart{}, invalidMappingFailure()
		}
		options, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		custom := provider.CustomPart(part.Kind)
		custom.ProviderOptions = options
		return custom, nil
	case provider.ContentPartTypeToolCall:
		if role != provider.RoleAssistant {
			return provider.ContentPart{}, invalidMappingFailure()
		}
		options, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		call := provider.ToolCallPart(part.ToolCallID, part.ToolName, part.Input)
		call.ProviderExecuted, call.ProviderOptions = part.ProviderExecuted, options
		return call, nil
	case provider.ContentPartTypeToolResult:
		if role != provider.RoleTool && role != provider.RoleAssistant {
			return provider.ContentPart{}, rejectedMappingFailure(capabilityTools)
		}
		output, failure := mapToolOutput(part.Output)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		options, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		result := provider.ToolResultPart(part.ToolCallID, part.ToolName, output)
		result.ProviderOptions = options
		return result, nil
	case provider.ContentPartTypeToolApprovalResponse:
		if role != provider.RoleTool || part.Approved == nil || part.ApprovalID == "" {
			return provider.ContentPart{}, invalidMappingFailure()
		}
		options, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		result := provider.ToolApprovalResponsePart(part.ApprovalID, *part.Approved, part.Reason)
		result.ProviderOptions = options
		return result, nil
	case provider.ContentPartTypeToolApprovalRequest:
		return provider.ContentPart{}, rejectedMappingFailure(capabilityToolApprovals)
	default:
		return provider.ContentPart{}, invalidMappingFailure()
	}
}

// mapWireHeaders copies body-carried call headers, preserving key case because
// the wire contract round-trips a caller's exact spelling. The execution runtime
// refuses credential-bearing names.
func mapWireHeaders(headers map[string]string) (map[string]string, *requestFailure) {
	if len(headers) == 0 {
		return nil, nil
	}
	mapped := make(map[string]string, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for name, value := range headers {
		// HTTP header names are case-insensitive, so two spellings of one name
		// would reach a backend in map order, with a different value each run.
		lower := strings.ToLower(name)
		if _, duplicate := seen[lower]; duplicate {
			return nil, invalidMappingFailure()
		}
		seen[lower] = struct{}{}
		// The HTTP contract round-trips protocol names in the body, so they are
		// accepted, but they address this runtime rather than a backend and are
		// not forwarded to one.
		if strings.HasPrefix(lower, "ai-language-model-") || strings.HasPrefix(lower, "ai-o11y-") {
			continue
		}
		mapped[name] = value
	}
	if len(mapped) == 0 {
		return nil, nil
	}
	return mapped, nil
}

// mapWireProviderOptions converts namespaces to opaque provider options, the
// call-level gateway controls included, which the execution runtime removes. Each
// namespace value must be a JSON object; nested contents are preserved byte for
// byte, including null, false, zero, empty string, empty object and array.
func mapWireProviderOptions(options map[string]json.RawMessage) (provider.ProviderOptions, *requestFailure) {
	if len(options) == 0 {
		return nil, nil
	}
	mapped := make(provider.ProviderOptions, len(options))
	for namespace, raw := range options {
		if _, ok := jsonObject(raw); !ok {
			return nil, invalidMappingFailure()
		}
		mapped[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
	}
	return mapped, nil
}

// jsonObject decodes raw as a JSON object and reports whether it is one. A JSON
// null decodes into a nil map without error, so the nil check is what rejects it.
func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, false
	}
	return object, true
}

// providerOptionsEmpty reports whether every namespace is an empty object.
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
	case wireReasoningMax:
		return provider.ReasoningMax, nil
	default:
		return provider.ReasoningProviderDefault, fmt.Errorf("unknown reasoning value")
	}
}
