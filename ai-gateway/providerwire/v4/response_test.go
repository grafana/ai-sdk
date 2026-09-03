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
			{Type: provider.ContentText, Text: "quote=\" slash=\\ newline=\n snowman=☃ html=<>&"},
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
		"content":[{"type":"text","text":""},{"type":"text","text":"quote=\" slash=\\ newline=\n snowman=☃ html=<>&"}],
		"finishReason":{"unified":"other","raw":"raw-stop"},
		"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0}}
	}`, string(body))
	assert.Contains(t, string(body), `html=\u003c\u003e\u0026`)
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

	t.Run("raw-size preflight precedes UTF-8 validation", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = strings.Repeat("x", 128) + string([]byte{0xff})
		assert.False(t, unarySuccessPreflight(result, 128))
		_, err := mapUnarySuccess(result, 128)
		require.Error(t, err)
	})

	t.Run("bounded preflight rejects excessive content count", func(t *testing.T) {
		limit := minimumTextPartBytes * 2
		result := validGenerateResult()
		result.Content = make([]provider.GenerateContentPart, 3)
		assert.False(t, unarySuccessPreflight(result, limit))
	})

	t.Run("aggregate raw-string accounting is overflow safe", func(t *testing.T) {
		result := validGenerateResult()
		result.Content = []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: strings.Repeat("a", 40)},
			{Type: provider.ContentText, Text: strings.Repeat("b", 40)},
		}
		result.FinishReason.Raw = strings.Repeat("r", 40)
		assert.False(t, unarySuccessPreflight(result, 100))
	})

	t.Run("invalid UTF-8 fails after bounded preflight", func(t *testing.T) {
		result := validGenerateResult()
		result.FinishReason.Raw = string([]byte{0xff})
		assert.True(t, unarySuccessPreflight(result, 128))
		_, err := mapUnarySuccess(result, 128)
		require.Error(t, err)
	})

	t.Run("escaping expansion is checked before commitment", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = strings.Repeat("\x00", 16)
		mapped, err := mapUnarySuccess(result, 1<<20)
		require.NoError(t, err)
		complete, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		assert.Contains(t, string(complete), `\u0000`)

		limit := int64(len(complete) - 1)
		assert.True(t, unarySuccessPreflight(result, limit))
		body, ok := encodeUnarySuccess(mapped, limit)
		assert.False(t, ok)
		assert.Nil(t, body)

		limits := testLimits()
		limits.UnaryResponseBytes = limit
		h := newTestHandler(t, limits)
		recorder := httptest.NewRecorder()
		assert.False(t, h.writeUnarySuccess(recorder, result))
		assert.Empty(t, recorder.Body.String())
	})
}
