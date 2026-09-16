package v4

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

type unsupportedCapability string

const (
	capabilityFiles                   unsupportedCapability = "files"
	capabilityReasoningContent        unsupportedCapability = "reasoning-content"
	capabilityCustomContent           unsupportedCapability = "custom-content"
	capabilityTools                   unsupportedCapability = "tools"
	capabilityToolApprovals           unsupportedCapability = "tool-approvals"
	capabilityStructuredOutput        unsupportedCapability = "structured-output"
	capabilityReservedProviderOptions unsupportedCapability = "reserved-provider-options"
	capabilityProtectedCallHeader     unsupportedCapability = "protected-call-header"
	capabilityProtectedProviderOption unsupportedCapability = "protected-provider-option"
	capabilityRawOutput               unsupportedCapability = "raw-output"
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

	headers, failure := mapWireHeaders(request.Headers)
	if failure != nil {
		return provider.CallOptions{}, failure
	}
	options.Headers = headers
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
	rootOptions, failure := mapWireProviderOptions(request.ProviderOptions)
	if failure != nil {
		return provider.CallOptions{}, failure
	}
	options.ProviderOptions = rootOptions
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
	switch message.Role {
	case provider.RoleSystem:
		messageOptions, failure := mapWireProviderOptions(message.ProviderOptions)
		if failure != nil {
			return provider.Message{}, failure
		}
		var text string
		if err := json.Unmarshal(message.Content, &text); err != nil {
			return provider.Message{}, invalidMappingFailure()
		}
		system := provider.NewSystemMessage(text)
		system.ProviderOptions = messageOptions
		return system, nil
	case provider.RoleUser, provider.RoleAssistant:
		messageOptions, failure := mapWireProviderOptions(message.ProviderOptions)
		if failure != nil {
			return provider.Message{}, failure
		}
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
		mapped := provider.NewAssistantMessage(parts...)
		if message.Role == provider.RoleUser {
			mapped = provider.NewUserMessage(parts...)
		}
		mapped.ProviderOptions = messageOptions
		return mapped, nil
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
	switch part.Type {
	case provider.ContentPartTypeText:
		partOptions, failure := mapWireProviderOptions(part.ProviderOptions)
		if failure != nil {
			return provider.ContentPart{}, failure
		}
		text := provider.TextPart(part.Text)
		text.ProviderOptions = partOptions
		return text, nil
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
	// The reserved namespace is found in its own pass, so a request carrying both
	// a reserved namespace and a malformed one always reports the same refusal
	// rather than whichever the map yielded first. Namespace names are compared
	// exactly, because provider-option namespaces are case-significant. Between a
	// malformed namespace and a protected field the refusal is whichever the map
	// yields first, which the request schema keeps unreachable over HTTP by
	// requiring every namespace to be an object.
	if _, reserved := options[ReservedProviderOptionNamespace]; reserved {
		return nil, unsupportedMappingFailure(capabilityReservedProviderOptions)
	}
	mapped := make(provider.ProviderOptions, len(options))
	for namespace, raw := range options {
		fields, ok := jsonObject(raw)
		if !ok {
			return nil, invalidMappingFailure()
		}
		for field := range fields {
			if _, protected := protectedProviderOptionFields[normalizeProviderOptionField(field)]; protected {
				return nil, unsupportedMappingFailure(capabilityProtectedProviderOption)
			}
		}
		mapped[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
	}
	return mapped, nil
}

// protectedProviderOptionFields name decisions this runtime has already made,
// so a caller may not make them again through a provider namespace. Providers
// merge unknown option fields into the request body, which is how a field here
// would otherwise reach a backend: model, fallbacks and the prompt fields
// would redirect the call the catalog resolved and telemetry reports, the tool
// fields (including the legacy functions pair) would run tools this runtime
// never mapped on the host's credentials, the response format fields would
// restate the structured output this runtime refuses at the wire level, and
// the stream fields would answer in a transport the runtime is not reading.
//
// Entries are normalized by normalizeProviderOptionField, because providers do
// not read field names exactly: providers/anthropic decodes with encoding/json,
// which matches names case-insensitively, so MCPServers reaches mcp_servers.
//
// After resolution the catalog's ProviderOptionPolicy narrows options further,
// to the resolved backend's namespaces and, where it lists them, its fields.
//
// ponytail: this list still carries openai-compatible, whose policy forwards
// every field because passing endpoint-specific fields through is that
// provider's purpose. Whether the Gateway restricts them is open on #115.
var protectedProviderOptionFields = map[string]struct{}{
	"model":          {},
	"fallbacks":      {},
	"messages":       {},
	"prompt":         {},
	"tools":          {},
	"toolchoice":     {},
	"functions":      {},
	"functioncall":   {},
	"mcpservers":     {},
	"container":      {},
	"responseformat": {},
	"stream":         {},
	"streamoptions":  {},
	// Message and part providers spread option fields over the entry they build
	// (upstream does the same), so these would restructure a mapped message:
	// a tool role or tool calls would restore tools, and a part type or media
	// field would restore the files the wire refuses.
	"role":       {},
	"toolcalls":  {},
	"toolcallid": {},
	"type":       {},
	"imageurl":   {},
	"inputaudio": {},
	"file":       {},
}

// normalizeProviderOptionField folds the spellings a provider may read for one
// field (case, snake_case, kebab-case) to a single key.
func normalizeProviderOptionField(field string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(field))
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
