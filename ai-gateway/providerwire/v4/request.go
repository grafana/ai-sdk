package v4

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type unsupportedCapability string

const (
	capabilityReasoningContent unsupportedCapability = "reasoning-content"
	capabilityCustomContent    unsupportedCapability = "custom-content"
	capabilityTools            unsupportedCapability = "tools"
	capabilityToolApprovals    unsupportedCapability = "tool-approvals"
	capabilityStructuredOutput unsupportedCapability = "structured-output"
	capabilityProviderOptions  unsupportedCapability = "provider-options"
	// Policy refusals. These are not unsupported capabilities: the runtime maps
	// provider options and call headers, and refuses only what the host owns.
	capabilityReservedProviderOptions unsupportedCapability = "reserved-provider-options"
	capabilityProtectedCallHeader     unsupportedCapability = "protected-call-header"
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
	Type             provider.ContentPartType   `json:"type"`
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

func mapWireRequest(body []byte, modes ...executionMode) (provider.CallOptions, *requestFailure) {
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
	toolsEnabled := len(modes) == 0 || modes[0] == executionUnary || modes[0] == executionStreaming
	for _, wireMessage := range request.Prompt {
		message, failure := mapWireMessage(wireMessage, toolsEnabled)
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
		if !toolsEnabled {
			return provider.CallOptions{}, unsupportedMappingFailure(capabilityTools)
		}
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
			return provider.CallOptions{}, unsupportedMappingFailure(capabilityStructuredOutput)
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

type historicalToolCall struct {
	name             string
	providerExecuted bool
	completed        bool
}

func unresolvedProviderCalls(prompt []provider.Message) (map[string]string, *requestFailure) {
	calls := make(map[string]historicalToolCall)
	orphanResults := make(map[string]struct{})
	for _, message := range prompt {
		for _, part := range message.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall:
				if _, exists := calls[part.ToolCallID]; exists {
					return nil, invalidMappingFailure()
				}
				if _, exists := orphanResults[part.ToolCallID]; exists {
					return nil, invalidMappingFailure()
				}
				calls[part.ToolCallID] = historicalToolCall{name: part.ToolName, providerExecuted: part.ProviderExecuted}
			case provider.ContentPartTypeToolResult:
				if call, exists := calls[part.ToolCallID]; exists {
					if call.name != part.ToolName || call.completed {
						return nil, invalidMappingFailure()
					}
					call.completed = true
					calls[part.ToolCallID] = call
				} else {
					orphanResults[part.ToolCallID] = struct{}{}
				}
			}
		}
	}
	pending := make(map[string]string)
	for id, call := range calls {
		if call.providerExecuted && !call.completed {
			pending[id] = call.name
		}
	}
	return pending, nil
}

func mapWireMessage(message wireMessage, toolsEnabled bool) (provider.Message, *requestFailure) {
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
			part, failure := mapWirePart(wirePart, message.Role, toolsEnabled)
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

func mapWirePart(part wirePart, role provider.Role, toolsEnabled bool) (provider.ContentPart, *requestFailure) {
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
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityCustomContent)
	case provider.ContentPartTypeToolCall:
		if !toolsEnabled {
			return provider.ContentPart{}, unsupportedMappingFailure(capabilityTools)
		}
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
		if !toolsEnabled || role != provider.RoleTool && role != provider.RoleAssistant {
			return provider.ContentPart{}, unsupportedMappingFailure(capabilityTools)
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
	case provider.ContentPartTypeToolApprovalResponse, provider.ContentPartTypeToolApprovalRequest:
		return provider.ContentPart{}, unsupportedMappingFailure(capabilityToolApprovals)
	default:
		return provider.ContentPart{}, invalidMappingFailure()
	}
}

// ReservedProviderOptionNamespace is the provider-option namespace the host
// owns. A caller that sends it is rejected rather than silently stripped, so
// nothing it carries can reach a backend and the caller learns the option was
// refused. Configuration reads it so a provider cannot be named after it, and
// compares it exactly, because provider-option namespaces are case-significant.
const ReservedProviderOptionNamespace = "grafana"

// IsProtectedCallHeader reports whether a header name carries credentials and so
// may not cross from a request body into a backend call. The inbound
// authenticated edge refuses some of the same names in outer HTTP headers, and
// TestInboundRefusedHeadersAreProtectedCallHeaders in the auth package asserts
// every name it refuses is in this set.
func IsProtectedCallHeader(name string) bool {
	_, protected := protectedCallHeaders[strings.ToLower(name)]
	return protected
}

// protectedCallHeaders never cross from a request body into a backend call.
// Providers apply call headers after their own authorization header, so a
// caller-supplied entry here would choose the credential presented upstream.
// The set covers credential-bearing names only: the HTTP contract retains
// protocol headers such as ai-language-model-id in the body, so rejecting those
// would break a documented round trip.
var protectedCallHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"x-access-token":      {},
	"x-grafana-id":        {},
	"x-api-key":           {},
	"api-key":             {},
	"openai-api-key":      {},
	"anthropic-api-key":   {},
}

// mapWireHeaders copies body-carried call headers, preserving key case because
// the wire contract round-trips a caller's exact spelling.
func mapWireHeaders(headers map[string]string) (map[string]string, *requestFailure) {
	if len(headers) == 0 {
		return nil, nil
	}
	mapped := make(map[string]string, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for name, value := range headers {
		if IsProtectedCallHeader(name) {
			return nil, unsupportedMappingFailure(capabilityProtectedCallHeader)
		}
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

// mapWireProviderOptions converts namespaces to opaque provider options. Each
// namespace value must be a JSON object; nested contents are preserved byte for
// byte, including null, false, zero, empty string, empty object and array.
func mapWireProviderOptions(options map[string]json.RawMessage) (provider.ProviderOptions, *requestFailure) {
	if len(options) == 0 {
		return nil, nil
	}
	// Reserved namespaces are found in their own pass, so a request carrying both
	// a reserved namespace and a malformed one always reports the same refusal
	// rather than whichever the map yielded first. Namespace names are compared
	// exactly, because provider-option namespaces are case-significant.
	for _, namespace := range []string{ReservedProviderOptionNamespace, "gateway", "grafana-ai-sdk"} {
		if _, reserved := options[namespace]; reserved {
			return nil, unsupportedMappingFailure(capabilityReservedProviderOptions)
		}
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
	default:
		return provider.ReasoningProviderDefault, fmt.Errorf("unknown reasoning value")
	}
}
