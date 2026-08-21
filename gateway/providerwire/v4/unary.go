package v4

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type generateContentType string

const generateContentText generateContentType = "text"

type textContentDTO struct {
	Type generateContentType `json:"type"`
	Text string              `json:"text"`
}

type finishReasonDTO struct {
	Unified provider.UnifiedFinishReason `json:"unified"`
	Raw     string                       `json:"raw,omitempty"`
}

type inputUsageDTO struct {
	Total      *int `json:"total,omitempty"`
	NoCache    *int `json:"noCache,omitempty"`
	CacheRead  *int `json:"cacheRead,omitempty"`
	CacheWrite *int `json:"cacheWrite,omitempty"`
}

type outputUsageDTO struct {
	Total     *int `json:"total,omitempty"`
	Text      *int `json:"text,omitempty"`
	Reasoning *int `json:"reasoning,omitempty"`
}

type usageDTO struct {
	InputTokens  inputUsageDTO  `json:"inputTokens"`
	OutputTokens outputUsageDTO `json:"outputTokens"`
}

type unaryResponseMetadataDTO struct {
	ModelID string `json:"modelId"`
}

type unaryResponseDTO struct {
	Content      []textContentDTO         `json:"content"`
	FinishReason finishReasonDTO          `json:"finishReason"`
	Usage        usageDTO                 `json:"usage"`
	Warnings     []struct{}               `json:"warnings"`
	Response     unaryResponseMetadataDTO `json:"response"`
}

func (h *Handler) encodeUnary(result *provider.GenerateResult, canonicalModelID string) ([]byte, error) {
	if !utf8.ValidString(canonicalModelID) {
		return nil, errors.New("providerwire v4: canonical model ID is not valid UTF-8")
	}
	if result == nil {
		return nil, errors.New("providerwire v4: nil generate result")
	}
	if len(result.Warnings) != 0 {
		return nil, errors.New("providerwire v4: provider warnings are unsupported")
	}
	finish, err := mapFinishReason(result.FinishReason)
	if err != nil {
		return nil, err
	}
	usage, err := mapUsage(result.Usage)
	if err != nil {
		return nil, err
	}
	content := make([]textContentDTO, 0, len(result.Content))
	for _, part := range result.Content {
		if err := validateTextContent(part); err != nil {
			return nil, err
		}
		content = append(content, textContentDTO{Type: generateContentText, Text: part.Text})
	}
	dto := unaryResponseDTO{
		Content:      content,
		FinishReason: finish,
		Usage:        usage,
		Warnings:     []struct{}{},
		Response:     unaryResponseMetadataDTO{ModelID: canonicalModelID},
	}
	body, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("providerwire v4: encoding unary result: %w", err)
	}
	if err := h.schemas.unary.Validate(body); err != nil {
		return nil, fmt.Errorf("providerwire v4: validating unary result: %w", err)
	}
	if int64(len(body)) > h.maxUnaryResponseBytes {
		return nil, errors.New("providerwire v4: unary result exceeds configured limit")
	}
	return body, nil
}

func validateTextContent(part provider.GenerateContentPart) error {
	if part.Type != provider.ContentText {
		return errors.New("providerwire v4: non-text unary content is unsupported")
	}
	if !utf8.ValidString(part.Text) {
		return errors.New("providerwire v4: text content is not valid UTF-8")
	}
	if part.ID != "" || part.Kind != "" || part.ApprovalID != "" || part.ToolCallID != "" || part.ToolName != "" || len(part.Input) != 0 || len(part.Result) != 0 || part.IsError || part.Preliminary != nil || part.ProviderExecuted || part.Dynamic != nil || part.SourceType != "" || part.URL != "" || part.Title != "" || part.Data != nil || part.MediaType != "" || part.Filename != "" {
		return errors.New("providerwire v4: invalid text unary content")
	}
	return nil
}

func mapFinishReason(reason provider.FinishReason) (finishReasonDTO, error) {
	if !utf8.ValidString(reason.Raw) {
		return finishReasonDTO{}, errors.New("providerwire v4: raw finish reason is not valid UTF-8")
	}
	switch reason.Unified {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return finishReasonDTO{Unified: reason.Unified, Raw: reason.Raw}, nil
	default:
		return finishReasonDTO{}, errors.New("providerwire v4: invalid finish reason")
	}
}

const maxSafeInteger = 1<<53 - 1

func mapUsage(usage provider.Usage) (usageDTO, error) {
	for _, value := range []*int{
		usage.InputTokens.Total,
		usage.InputTokens.NoCache,
		usage.InputTokens.CacheRead,
		usage.InputTokens.CacheWrite,
		usage.OutputTokens.Total,
		usage.OutputTokens.Text,
		usage.OutputTokens.Reasoning,
	} {
		if value != nil && (*value < 0 || int64(*value) > maxSafeInteger) {
			return usageDTO{}, errors.New("providerwire v4: usage counter is not safely representable")
		}
	}
	return usageDTO{
		InputTokens: inputUsageDTO{
			Total:      usage.InputTokens.Total,
			NoCache:    usage.InputTokens.NoCache,
			CacheRead:  usage.InputTokens.CacheRead,
			CacheWrite: usage.InputTokens.CacheWrite,
		},
		OutputTokens: outputUsageDTO{
			Total:     usage.OutputTokens.Total,
			Text:      usage.OutputTokens.Text,
			Reasoning: usage.OutputTokens.Reasoning,
		},
	}, nil
}

func (h *Handler) serveUnary(w http.ResponseWriter, model provider.LanguageModel, options provider.CallOptions, canonicalModelID string, callContext context.Context) {
	callResult, ok := awaitModelCall(callContext, func() (*provider.GenerateResult, error) {
		return model.DoGenerate(callContext, options)
	})
	if !ok {
		h.writeError(w, contextFailureValue(callContext))
		return
	}
	if callResult.err != nil {
		h.writeError(w, reduceProviderError(callContext, callResult.err))
		return
	}
	result := callResult.result
	body, err := h.encodeUnary(result, canonicalModelID)
	if err != nil {
		h.writeError(w, canonicalInternal)
		return
	}
	w.Header().Set("Content-Type", MIMEJSON)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
