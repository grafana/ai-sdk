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
	Citations []provider.Citation          `json:"citations,omitzero"`
	Type      provider.GenerateContentType `json:"type"`
	Text      string                       `json:"text"`
	Metadata  provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
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
	history  execution.ToolHistory
	overview *execution.Overview
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
	var context unaryMappingContext
	if len(contexts) != 0 {
		context = contexts[0]
	}
	if execution.ValidateGenerateResult(result, context.history) != nil || !validMetadata(result.ProviderMetadata) {
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
	for _, part := range result.Content {
		switch part.Type {
		case provider.ContentSource:
			source, err := mapSource(unarySource(part))
			if err != nil {
				return unarySuccess{}, err
			}
			mapped.Content = append(mapped.Content, source)
		case provider.ContentText:
			if !utf8.ValidString(part.Text) || !validCitations(part.Citations) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryTextPart{Type: provider.ContentText, Text: part.Text, Citations: part.Citations, Metadata: part.ProviderMetadata})
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
			if !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || !utf8.Valid(part.Input) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryToolCall{Type: provider.ContentToolCall, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: string(part.Input), ProviderExecuted: part.ProviderExecuted, Dynamic: part.Dynamic, Metadata: part.ProviderMetadata})
		case provider.ContentToolApprovalRequest:
			if !utf8.ValidString(part.ApprovalID) || !utf8.ValidString(part.ToolCallID) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, approvalRequestEvent{Type: "tool-approval-request", ApprovalID: part.ApprovalID, ToolCallID: part.ToolCallID, Metadata: part.ProviderMetadata})
		case provider.ContentToolResult:
			if !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || !utf8.Valid(part.Result) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryToolResult{Type: provider.ContentToolResult, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Result: part.Result, IsError: part.IsError, Dynamic: part.Dynamic, Preliminary: part.Preliminary, Metadata: part.ProviderMetadata})
		default:
			return unarySuccess{}, errInvalidUnarySuccess
		}
	}
	if !utf8.ValidString(result.FinishReason.Raw) {
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
