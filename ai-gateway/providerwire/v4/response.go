package v4

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	maxJavaScriptSafeInteger = 9007199254740991
	minimumTextPartBytes     = int64(len(`{"type":"text","text":""}`))
)

var errInvalidUnarySuccess = errors.New("providerwire v4: invalid unary success")

type unaryTextPart struct {
	Type provider.GenerateContentType `json:"type"`
	Text string                       `json:"text"`
}

type unaryToolCall struct {
	Type             provider.GenerateContentType `json:"type"`
	ToolCallID       string                       `json:"toolCallId"`
	ToolName         string                       `json:"toolName"`
	Input            string                       `json:"input"`
	ProviderExecuted bool                         `json:"providerExecuted,omitempty"`
	Dynamic          bool                         `json:"dynamic,omitempty"`
	ProviderMetadata provider.ProviderMetadata    `json:"providerMetadata,omitempty"`
}

type unaryToolResult struct {
	Type             provider.GenerateContentType `json:"type"`
	ToolCallID       string                       `json:"toolCallId"`
	ToolName         string                       `json:"toolName"`
	Result           json.RawMessage              `json:"result"`
	IsError          bool                         `json:"isError,omitempty"`
	Dynamic          bool                         `json:"dynamic,omitempty"`
	Preliminary      bool                         `json:"preliminary,omitempty"`
	ProviderMetadata provider.ProviderMetadata    `json:"providerMetadata,omitempty"`
}

type unaryMappingContext struct {
	history  map[string]string
	mcpNames map[string]bool
}

type unaryToolState struct {
	name    string
	preview bool
	final   bool
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
}

type unarySuccess struct {
	Content      []any             `json:"content"`
	FinishReason unaryFinishReason `json:"finishReason"`
	Usage        unaryUsage        `json:"usage"`
}

func mapUnarySuccess(result *provider.GenerateResult, limit int64, contexts ...unaryMappingContext) (unarySuccess, error) {
	if !unarySuccessPreflight(result, limit) {
		return unarySuccess{}, errInvalidUnarySuccess
	}

	mapped := unarySuccess{
		Content: make([]any, 0, len(result.Content)),
		FinishReason: unaryFinishReason{
			Unified: result.FinishReason.Unified,
			Raw:     result.FinishReason.Raw,
		},
	}
	var context unaryMappingContext
	if len(contexts) != 0 {
		context = contexts[0]
	}
	states := make(map[string]unaryToolState, len(context.history))
	for id, name := range context.history {
		states[id] = unaryToolState{name: name}
	}
	for _, part := range result.Content {
		switch part.Type {
		case provider.ContentText:
			if part.ProviderExecuted || part.Dynamic || part.Preliminary || !utf8.ValidString(part.Text) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryTextPart{Type: provider.ContentText, Text: part.Text})
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || !utf8.Valid(part.Input) || part.Preliminary {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			if _, exists := states[part.ToolCallID]; exists {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			states[part.ToolCallID] = unaryToolState{name: part.ToolName}
			metadata, err := mapToolMetadata(part.ProviderMetadata, context.mcpNames, limit)
			if err != nil {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryToolCall{Type: provider.ContentToolCall, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: string(part.Input), ProviderExecuted: part.ProviderExecuted, Dynamic: part.Dynamic, ProviderMetadata: metadata})
		case provider.ContentToolResult:
			state, exists := states[part.ToolCallID]
			if !exists || state.name != part.ToolName || state.final || part.ToolCallID == "" || part.ToolName == "" || !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || len(part.Result) == 0 || !utf8.Valid(part.Result) || !json.Valid(part.Result) || bytes.Equal(bytes.TrimSpace(part.Result), []byte("null")) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			metadata, err := mapToolMetadata(part.ProviderMetadata, context.mcpNames, limit)
			if err != nil {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			state.preview = part.Preliminary
			state.final = !part.Preliminary
			states[part.ToolCallID] = state
			mapped.Content = append(mapped.Content, unaryToolResult{Type: provider.ContentToolResult, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Result: part.Result, IsError: part.IsError, Dynamic: part.Dynamic, Preliminary: part.Preliminary, ProviderMetadata: metadata})
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
	mapped.Usage = unaryUsage{InputTokens: inputUsage, OutputTokens: outputUsage}
	return mapped, nil
}

func unarySuccessPreflight(result *provider.GenerateResult, limit int64) bool {
	if result == nil || limit <= 0 || int64(len(result.Content)) > limit/minimumTextPartBytes {
		return false
	}
	remaining := limit
	for _, part := range result.Content {
		for _, length := range []int{len(part.Text), len(part.ToolCallID), len(part.ToolName), len(part.Input), len(part.Result)} {
			if int64(length) > remaining {
				return false
			}
			remaining -= int64(length)
		}
		if int64(len(part.ProviderMetadata)) > remaining {
			return false
		}
		for namespace, raw := range part.ProviderMetadata {
			if int64(len(namespace)) > remaining {
				return false
			}
			remaining -= int64(len(namespace))
			if int64(len(raw)) > remaining {
				return false
			}
			remaining -= int64(len(raw))
		}
	}
	return int64(len(result.FinishReason.Raw)) <= remaining
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

func encodeUnarySuccess(value unarySuccess, limit int64) ([]byte, bool) {
	body, err := json.Marshal(value)
	if err != nil || int64(len(body)) > limit {
		return nil, false
	}
	return body, true
}

func (h *handler) writeUnarySuccess(w http.ResponseWriter, result *provider.GenerateResult, contexts ...unaryMappingContext) bool {
	mapped, err := mapUnarySuccess(result, h.limits.UnaryResponseBytes, contexts...)
	if err != nil {
		return false
	}
	body, ok := encodeUnarySuccess(mapped, h.limits.UnaryResponseBytes)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}
