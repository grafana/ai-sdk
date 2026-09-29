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
	maxRawUsageBytes         = 1 << 20
	minimumTextPartBytes     = int64(len(`{"type":"text","text":""}`))
)

var errInvalidUnarySuccess = errors.New("providerwire v4: invalid unary success")

type unaryTextPart struct {
	Type             provider.GenerateContentType `json:"type"`
	Text             string                       `json:"text"`
	ProviderMetadata *projectedMetadata           `json:"providerMetadata,omitempty"`
}

type unaryToolCall struct {
	Type             provider.GenerateContentType `json:"type"`
	ToolCallID       string                       `json:"toolCallId"`
	ToolName         string                       `json:"toolName"`
	Input            string                       `json:"input"`
	ProviderMetadata *projectedMetadata           `json:"providerMetadata,omitempty"`
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
	Content      []any             `json:"content"`
	FinishReason unaryFinishReason `json:"finishReason"`
	Usage        unaryUsage        `json:"usage"`
}

func mapUnarySuccess(result *provider.GenerateResult, limit int64) (unarySuccess, error) {
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
	for _, part := range result.Content {
		if part.ProviderExecuted || (part.Dynamic != nil && *part.Dynamic) || (part.Preliminary != nil && *part.Preliminary) {
			return unarySuccess{}, errInvalidUnarySuccess
		}
		switch part.Type {
		case provider.ContentText:
			if !utf8.ValidString(part.Text) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			metadata, err := projectProviderMetadata(part.ProviderMetadata, false, "", limit)
			if err != nil {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryTextPart{Type: provider.ContentText, Text: part.Text, ProviderMetadata: metadata})
		case provider.ContentToolCall:
			if part.ToolCallID == "" || part.ToolName == "" || !utf8.ValidString(part.ToolCallID) || !utf8.ValidString(part.ToolName) || !utf8.Valid(part.Input) {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			metadata, err := projectProviderMetadata(part.ProviderMetadata, true, "", limit)
			if err != nil {
				return unarySuccess{}, errInvalidUnarySuccess
			}
			mapped.Content = append(mapped.Content, unaryToolCall{Type: provider.ContentToolCall, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: string(part.Input), ProviderMetadata: metadata})
		default:
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
	if !validRawUsage(result.Usage.Raw, limit) {
		return unarySuccess{}, errInvalidUnarySuccess
	}
	mapped.Usage = unaryUsage{InputTokens: inputUsage, OutputTokens: outputUsage, Raw: result.Usage.Raw}
	return mapped, nil
}

func unarySuccessPreflight(result *provider.GenerateResult, limit int64) bool {
	if result == nil || limit <= 0 || int64(len(result.Content)) > limit/minimumTextPartBytes {
		return false
	}
	remaining := limit
	for _, part := range result.Content {
		for _, length := range []int{len(part.Text), len(part.ToolCallID), len(part.ToolName), len(part.Input)} {
			if int64(length) > remaining {
				return false
			}
			remaining -= int64(length)
		}
		for _, name := range []string{"openai", "anthropic"} {
			if name == "anthropic" && part.Type != provider.ContentToolCall {
				continue
			}
			if raw, ok := part.ProviderMetadata[name]; ok {
				if int64(len(raw)) > remaining {
					return false
				}
				remaining -= int64(len(raw))
			}
		}
	}
	if int64(len(result.FinishReason.Raw)) > remaining {
		return false
	}
	remaining -= int64(len(result.FinishReason.Raw))
	return int64(len(result.Usage.Raw)) <= remaining && len(result.Usage.Raw) <= maxRawUsageBytes
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

func validRawUsage(raw json.RawMessage, limit int64) bool {
	if len(raw) == 0 {
		return true
	}
	if int64(len(raw)) > limit || len(raw) > maxRawUsageBytes || !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func encodeUnarySuccess(value unarySuccess, limit int64) ([]byte, bool) {
	body, err := json.Marshal(value)
	if err != nil || int64(len(body)) > limit {
		return nil, false
	}
	return body, true
}

func (h *handler) writeUnarySuccess(w http.ResponseWriter, result *provider.GenerateResult) bool {
	mapped, err := mapUnarySuccess(result, h.limits.UnaryResponseBytes)
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
