package v4

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validGenerateResult() *provider.GenerateResult {
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "ok"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
}

func TestUnarySuccessMapping(t *testing.T) {
	zero := 0
	maximum := maxJavaScriptSafeInteger
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: ""},
			{Type: provider.ContentText, Text: "quote=\" slash=\\ newline=\n snowman=☃"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther, Raw: "raw-stop"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &maximum, NoCache: &zero, CacheRead: &zero, CacheWrite: &zero},
			OutputTokens: provider.OutputTokenUsage{Total: &zero, Text: &zero, Reasoning: &zero},
			Raw:          json.RawMessage(`{"private":true}`),
		},
		Warnings: []provider.Warning{{Type: provider.WarningType("future"), Message: "private warning"}},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ID: "private-response", ModelID: "private-model", Provider: "private-provider",
		}},
	}

	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	require.True(t, json.Valid(body))
	compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(body))
	assert.JSONEq(t, `{
		"content":[{"type":"text","text":""},{"type":"text","text":"quote=\" slash=\\ newline=\n snowman=☃"}],
		"finishReason":{"unified":"other","raw":"raw-stop"},
		"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0}}
	}`, string(body))
	for _, private := range []string{"private warning", "private-response", "private-model", "private-provider", `"private":true`, "warnings", "response"} {
		assert.NotContains(t, string(body), private)
	}
}

func TestUnarySuccessValidation(t *testing.T) {
	t.Run("finish reasons", func(t *testing.T) {
		for _, reason := range []provider.UnifiedFinishReason{
			provider.FinishReasonStop,
			provider.FinishReasonLength,
			provider.FinishReasonContentFilter,
			provider.FinishReasonToolCalls,
			provider.FinishReasonError,
			provider.FinishReasonOther,
		} {
			result := validGenerateResult()
			result.FinishReason.Unified = reason
			_, err := mapUnarySuccess(result, 1<<20)
			require.NoError(t, err)
		}
	})

	t.Run("invalid provider results", func(t *testing.T) {
		negative := -1
		tooLarge := maxJavaScriptSafeInteger + 1
		tests := []*provider.GenerateResult{
			nil,
			{Content: []provider.GenerateContentPart{{Type: provider.ContentReasoning}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentText}}, FinishReason: provider.FinishReason{Unified: provider.UnifiedFinishReason("future")}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentText}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{InputTokens: provider.InputTokenUsage{Total: &negative}}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentText}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &tooLarge}}},
		}
		for _, result := range tests {
			_, err := mapUnarySuccess(result, 1<<20)
			require.Error(t, err)
		}
	})
}

func TestUnarySuccessBoundaries(t *testing.T) {
	result := validGenerateResult()
	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	complete, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)

	for _, tc := range []struct {
		name   string
		limit  int64
		status int
	}{
		{name: "below", limit: int64(len(complete) - 1), status: http.StatusInternalServerError},
		{name: "at", limit: int64(len(complete)), status: http.StatusOK},
		{name: "above", limit: int64(len(complete) + 1), status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.UnaryResponseBytes = tc.limit
			h := newTestHandler(t, limits)
			recorder := httptest.NewRecorder()
			if !h.writeUnarySuccess(recorder, result) {
				h.writeSafeError(recorder, safeError{category: safeInternal})
			}
			assert.Equal(t, tc.status, recorder.Code)
			assert.LessOrEqual(t, recorder.Body.Len(), max(int(tc.limit), len(canonicalInternalError)))
		})
	}

	t.Run("invalid UTF-8 fails before success commitment", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = string([]byte{0xff})
		h := newTestHandler(t, testLimits())
		recorder := httptest.NewRecorder()
		assert.False(t, h.writeUnarySuccess(recorder, result))
		assert.Empty(t, recorder.Body.String())
	})

	t.Run("oversized raw string is rejected before UTF-8 scanning", func(t *testing.T) {
		buffer := newBoundedDocument(64)
		buffer.appendJSONString(strings.Repeat("x", 1<<20) + string([]byte{0xff}))
		assert.True(t, buffer.overflow)
		assert.False(t, buffer.invalid)
		assert.Empty(t, buffer.data)
	})

	t.Run("oversized first text stops before later parts and fields", func(t *testing.T) {
		mapped := unarySuccess{
			content: []string{
				strings.Repeat("x", 1<<20),
				string([]byte{0xff}),
			},
			finishReason: provider.FinishReason{
				Unified: provider.FinishReasonStop,
				Raw:     string([]byte{0xff}),
			},
		}
		body, ok := encodeUnarySuccess(mapped, 128)
		assert.False(t, ok)
		assert.Nil(t, body)
	})

	t.Run("oversized raw finish stops before usage fields", func(t *testing.T) {
		one := 1
		mapped := unarySuccess{
			finishReason: provider.FinishReason{
				Unified: provider.FinishReasonStop,
				Raw:     strings.Repeat("x", 1<<20) + string([]byte{0xff}),
			},
			inputUsage: unaryTokenUsage{total: &one},
		}
		body, ok := encodeUnarySuccess(mapped, 128)
		assert.False(t, ok)
		assert.Nil(t, body)
	})
}
