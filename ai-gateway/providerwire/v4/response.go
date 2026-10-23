package v4

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

const (
	maxJavaScriptSafeInteger = 9007199254740991
)

var errInvalidUnarySuccess = errors.New("providerwire v4: invalid unary success")

type unaryTextPart struct {
	Type     provider.GenerateContentType `json:"type"`
	Text     string                       `json:"text"`
	Metadata provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type unaryCustomPart struct {
	Type     provider.GenerateContentType `json:"type"`
	Kind     string                       `json:"kind"`
	Metadata provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type unaryToolCall struct {
	Type             provider.GenerateContentType `json:"type"`
	ToolCallID       string                       `json:"toolCallId"`
	ToolName         string                       `json:"toolName"`
	Input            string                       `json:"input"`
	ProviderExecuted bool                         `json:"providerExecuted,omitempty"`
	Dynamic          *bool                        `json:"dynamic,omitempty"`
	Metadata         provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type unaryToolResult struct {
	Type        provider.GenerateContentType `json:"type"`
	ToolCallID  string                       `json:"toolCallId"`
	ToolName    string                       `json:"toolName"`
	Result      json.RawMessage              `json:"result"`
	IsError     bool                         `json:"isError,omitempty"`
	Dynamic     *bool                        `json:"dynamic,omitempty"`
	Preliminary *bool                        `json:"preliminary,omitempty"`
	Metadata    provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type unaryMappingContext struct {
	history  map[string]string
	overview *execution.Overview
}

type unaryToolState struct {
	historical       bool
	providerExecuted bool
	approvalID       string
	name             string
	preview          bool
	final            bool
}

type unaryFinishReason struct {
	Unified provider.UnifiedFinishReason `json:"unified"`
	Raw     string                       `json:"raw,omitempty"`
}

type unaryInputTokenUsage struct {
	Total      *int `json:"total,omitempty"`
	NoCache    *int `json:"noCache,omitempty"`
	CacheRead  *int `json:"cacheRead,omitempty"`
	CacheWrite *int `json:"cacheWrite,omitempty"`
}

type unaryOutputTokenUsage struct {
	Total     *int `json:"total,omitempty"`
	Text      *int `json:"text,omitempty"`
	Reasoning *int `json:"reasoning,omitempty"`
}

type unaryUsage struct {
	InputTokens  unaryInputTokenUsage  `json:"inputTokens"`
	OutputTokens unaryOutputTokenUsage `json:"outputTokens"`
	Raw          json.RawMessage       `json:"raw,omitempty"`
}

type unarySuccess struct {
	Content      []any                     `json:"content"`
	FinishReason unaryFinishReason         `json:"finishReason"`
	Usage        unaryUsage                `json:"usage"`
	Warnings     []responseWarning         `json:"warnings"`
	Response     *responseIdentity         `json:"response,omitempty"`
	Metadata     provider.ProviderMetadata `json:"providerMetadata,omitzero"`
}

func mapUnarySuccess(result *provider.GenerateResult, contexts ...unaryMappingContext) (unarySuccess, error) {
	if result == nil || !validMetadata(result.ProviderMetadata) {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	for _, part := range result.Content {
		if !validMetadata(part.ProviderMetadata) {
			return unarySuccess{}, errInvalidUnarySuccess
		}
	}

	mapped := unarySuccess{
		Content:  make([]any, 0, len(result.Content)),
		Metadata: result.ProviderMetadata,
		FinishReason: unaryFinishReason{
			Unified: result.FinishReason.Unified,
			Raw:     result.FinishReason.Raw,
		},
	}
	warnings, err := mapWarnings(result.Warnings)
	if err != nil {
		return unarySuccess{}, err
	}
	mapped.Warnings = warnings
	if result.Response != nil {
		identity, err := mapResponseIdentity(result.Response.ID, result.Response.ModelID, result.Response.Timestamp)
		if err != nil {
			return unarySuccess{}, err
		}
		mapped.Response = &identity
	}
	var context unaryMappingContext
	if len(contexts) != 0 {
		context = contexts[0]
	}
	states := make(map[string]unaryToolState, len(context.history))
	for id, name := range context.history {
		states[id] = unaryToolState{name: name, providerExecuted: true, historical: true}
	}
	approvals := make(map[string]struct{})
	for _, part := range result.Content {
		if part.Type != provider.ContentToolCall && part.Type != provider.ContentToolResult && (part.ProviderExecuted || (part.Dynamic != nil && *part.Dynamic) || (part.Preliminary != nil && *part.Preliminary)) {
			return unarySuccess{}, errInvalidUnarySuccess
		}
		switch part.Type {
		case provider.ContentSource:
			source, err := mapSource(unarySource(part))
			if err != nil {
				return unarySuccess{}, err
			}
			mapped.Content = append(mapped.Content, source)
		case provider.ContentText:
			if !utf8.ValidString(part.Text) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryTextPart{Type: provider.ContentText, Text: part.Text, Metadata: part.ProviderMetadata})
		case provider.ContentCustom:
			if !validCustomKind(part.Kind) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryCustomPart{Type: provider.ContentCustom, Kind: part.Kind, Metadata: part.ProviderMetadata})
		case provider.ContentReasoning, provider.ContentReasoningFile, provider.ContentFile:
			if part.Type == provider.ContentReasoning {
				if !utf8.ValidString(part.Text) {
					return unarySuccess{}, errInvalidUnarySuccess
				}
				mapped.Content = append(mapped.Content, reasoningTextPart{Type: part.Type, Text: part.Text, Metadata: part.ProviderMetadata})
			} else {
				data := unaryReasoningFile(part.Data)
				if !validReasoningFile(data, part.MediaType) {
					return unarySuccess{}, errInvalidUnarySuccess
				}
				mapped.Content = append(mapped.Content, reasoningFilePart{Type: string(part.Type), MediaType: part.MediaType, Data: projectReasoningFile(data), Metadata: part.ProviderMetadata})
			}
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || !utf8.Valid(part.Input) || (part.Preliminary != nil && *part.Preliminary) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			if prior, exists := states[part.ToolCallID]; exists && (!prior.historical || prior.final || prior.preview || !part.ProviderExecuted || prior.name != part.ToolName) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			states[part.ToolCallID] = unaryToolState{name: part.ToolName, providerExecuted: part.ProviderExecuted, approvalID: states[part.ToolCallID].approvalID}
			mapped.Content = append(mapped.Content, unaryToolCall{Type: provider.ContentToolCall, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: string(part.Input), ProviderExecuted: part.ProviderExecuted, Dynamic: part.Dynamic, Metadata: part.ProviderMetadata})
		case provider.ContentToolApprovalRequest:
			state, exists := states[part.ToolCallID]
			_, duplicate := approvals[part.ApprovalID]
			if !exists || !state.providerExecuted || state.final || state.preview || state.approvalID != "" || duplicate || part.ApprovalID == "" || !utf8.ValidString(part.ApprovalID) || !utf8.ValidString(part.ToolCallID) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			state.approvalID = part.ApprovalID
			states[part.ToolCallID] = state
			approvals[part.ApprovalID] = struct{}{}
			mapped.Content = append(mapped.Content, approvalRequestEvent{Type: "tool-approval-request", ApprovalID: part.ApprovalID, ToolCallID: part.ToolCallID, Metadata: part.ProviderMetadata})
		case provider.ContentToolResult:
			state, exists := states[part.ToolCallID]
			if !exists || state.name != part.ToolName || state.final || part.ToolCallID == "" || part.ToolName == "" || !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || len(part.Result) == 0 || !utf8.Valid(part.Result) || !json.Valid(part.Result) || bytes.Equal(bytes.TrimSpace(part.Result), []byte("null")) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			state.preview = part.Preliminary != nil && *part.Preliminary
			state.final = !state.preview
			states[part.ToolCallID] = state
			mapped.Content = append(mapped.Content, unaryToolResult{Type: provider.ContentToolResult, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Result: part.Result, IsError: part.IsError, Dynamic: part.Dynamic, Preliminary: part.Preliminary, Metadata: part.ProviderMetadata})
		default:
			return unarySuccess{}, errInvalidUnarySuccess
		}
	}
	for _, state := range states {
		if state.preview {
			return unarySuccess{}, errInvalidUnarySuccess
		}
	}
	if !utf8.ValidString(result.FinishReason.Raw) {
		return unarySuccess{}, errInvalidUnarySuccess
	}

	switch result.FinishReason.Unified {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
	default:
		return unarySuccess{}, errInvalidUnarySuccess
	}

	inputUsage, err := mapInputUsage(result.Usage.InputTokens)
	if err != nil {
		return unarySuccess{}, err
	}
	outputUsage, err := mapOutputUsage(result.Usage.OutputTokens)
	if err != nil {
		return unarySuccess{}, err
	}
	if !validRawUsage(result.Usage.Raw) {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	mapped.Usage = unaryUsage{InputTokens: inputUsage, OutputTokens: outputUsage, Raw: result.Usage.Raw}
	return mapped, nil
}

func validMetadata(metadata provider.ProviderMetadata) bool {
	for key, raw := range metadata {
		if !utf8.ValidString(key) || !utf8.Valid(raw) || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			return false
		}
	}
	return true
}

func validCustomKind(kind string) bool {
	return utf8.ValidString(kind) && strings.Contains(kind, ".")
}

func mapInputUsage(usage provider.InputTokenUsage) (unaryInputTokenUsage, error) {
	values := []*int{usage.Total, usage.NoCache, usage.CacheRead, usage.CacheWrite}
	if !validTokenCounts(values...) {
		return unaryInputTokenUsage{}, errInvalidUnarySuccess
	}
	return unaryInputTokenUsage{
		Total:      usage.Total,
		NoCache:    usage.NoCache,
		CacheRead:  usage.CacheRead,
		CacheWrite: usage.CacheWrite,
	}, nil
}

func mapOutputUsage(usage provider.OutputTokenUsage) (unaryOutputTokenUsage, error) {
	values := []*int{usage.Total, usage.Text, usage.Reasoning}
	if !validTokenCounts(values...) {
		return unaryOutputTokenUsage{}, errInvalidUnarySuccess
	}
	return unaryOutputTokenUsage{Total: usage.Total, Text: usage.Text, Reasoning: usage.Reasoning}, nil
}

func validTokenCounts(values ...*int) bool {
	for _, value := range values {
		if value != nil && (*value < 0 || int64(*value) > maxJavaScriptSafeInteger) {
			return false
		}
	}
	return true
}

func validRawUsage(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func (h *handler) writeUnarySuccess(w http.ResponseWriter, result *provider.GenerateResult, contexts ...unaryMappingContext) bool {
	mapped, err := mapUnarySuccess(result, contexts...)
	if err != nil {
		return false
	}
	if len(contexts) > 0 {
		if metadata := execution.Metadata(contexts[0].overview, mapped.Metadata); metadata != nil {
			mapped.Metadata = metadata
		}
	}
	body, err := json.Marshal(mapped)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}
