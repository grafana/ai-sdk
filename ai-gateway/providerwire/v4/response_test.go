package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
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
			Raw:          json.RawMessage(`{"native_usage":true}`),
		},
		Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "native warning"}},
		Request:  &provider.RequestMetadata{Body: json.RawMessage(`{"private-request":true}`)},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ID: "private-response", ModelID: "private-model", Provider: "private-provider",
		}, Headers: map[string]string{"Authorization": "private-token"}, Body: json.RawMessage(`{"private-body":true}`)},
	}

	mapped, err := mapUnarySuccess(result, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	require.True(t, json.Valid(body))
	compiled := compileWireSchema(t, unarySuccessSchemaJSON)
	require.NoError(t, compiled.Validate(body))
	assert.JSONEq(t, `{
		"content":[{"type":"text","text":""},{"type":"text","text":"quote=\" slash=\\ newline=\n snowman=☃ html=<>&"}],
		"finishReason":{"unified":"other","raw":"raw-stop"},
		"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0},"raw":{"native_usage":true}},
		"warnings":[{"type":"other","message":"native warning"}],
		"response":{"id":"private-response","modelId":"private-model"}
	}`, string(body))
	assert.Contains(t, string(body), `html=\u003c\u003e\u0026`)
	for _, private := range []string{"private-provider", "private-request", "private-token", "private-body", "Authorization"} {
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
			{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "undotted"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "invalid." + string([]byte{0xff})}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}},
			{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "example.kind", ProviderExecuted: true}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}},
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

func TestUnaryProviderToolResult_BoundsAndPreliminary(t *testing.T) {
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`)}
	result := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"data":"` + strings.Repeat("a", 100) + `"}`)}
	value := validGenerateResult()
	value.Content = []provider.GenerateContentPart{call, result}
	mapped, err := mapUnarySuccess(value, 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	_, err = mapUnarySuccess(value, int64(len(result.Result))-1)
	require.Error(t, err)
	_, ok = encodeUnarySuccess(mapped, int64(len(body))-1)
	assert.False(t, ok)
	result.ProviderMetadata = provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}
	value.Content = []provider.GenerateContentPart{call, result}
	_, err = mapUnarySuccess(value, 512)
	require.Error(t, err)
	result.ProviderMetadata = nil
	result.Preliminary = new(true)
	value.Content = []provider.GenerateContentPart{call, result}
	_, err = mapUnarySuccess(value, 1<<20)
	require.Error(t, err)
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
				h.writeSafeError(recorder, safeError{category: safeInternal}, nil)
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

func customResult(parts ...provider.GenerateContentPart) *provider.GenerateResult {
	return &provider.GenerateResult{Content: parts, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
}

func TestUnaryCustomContent_Output(t *testing.T) {
	opaque := json.RawMessage(`{"itemId":"cmp_1","nested":{"zero":0,"no":false,"none":null,"empty":"","list":[]}}`)
	tests := []struct {
		name string
		part provider.GenerateContentPart
		want string
	}{
		{"absent metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback"}, `{"type":"custom","kind":"anthropic.fallback"}`},
		{"empty metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{}}, `{"type":"custom","kind":"example.kind","providerMetadata":{}}`},
		{"opaque metadata", provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "openai.compaction", ProviderMetadata: provider.ProviderMetadata{"openai": opaque}}, `{"type":"custom","kind":"openai.compaction","providerMetadata":{"openai":` + string(opaque) + `}}`},
	}
	for _, tc := range tests {
		t.Run("custom-only "+tc.name, func(t *testing.T) {
			mapped, err := mapUnarySuccess(customResult(tc.part), 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(body))
			var decoded struct {
				Content []json.RawMessage `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body, &decoded))
			require.Len(t, decoded.Content, 1)
			assert.JSONEq(t, tc.want, string(decoded.Content[0]))
		})
	}

	t.Run("keeps content order", func(t *testing.T) {
		mapped, err := mapUnarySuccess(customResult(
			provider.GenerateContentPart{Type: provider.ContentText, Text: "before"},
			provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind"},
			provider.GenerateContentPart{Type: provider.ContentText, Text: "after"},
		), 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		var decoded struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		}
		require.NoError(t, json.Unmarshal(body, &decoded))
		require.Len(t, decoded.Content, 3)
		assert.Equal(t, []string{"text", "custom", "text"}, []string{decoded.Content[0].Type, decoded.Content[1].Type, decoded.Content[2].Type})
	})

	t.Run("metadata budget is shared", func(t *testing.T) {
		part := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}}
		_, err := mapUnarySuccess(customResult(part), 512)
		require.Error(t, err)
		_, err = mapUnarySuccess(customResult(part), 1<<20)
		require.NoError(t, err)
	})

	t.Run("kind bytes count toward the preflight", func(t *testing.T) {
		part := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example." + strings.Repeat("k", 4096)}
		_, err := mapUnarySuccess(customResult(part), 2048)
		require.Error(t, err)
	})

	t.Run("invalid kinds and metadata fail before HTTP 200", func(t *testing.T) {
		for name, part := range map[string]provider.GenerateContentPart{
			"empty kind":       {Type: provider.ContentCustom},
			"undotted kind":    {Type: provider.ContentCustom, Kind: "private"},
			"invalid UTF-8":    {Type: provider.ContentCustom, Kind: "private." + string([]byte{0xff})},
			"array metadata":   {Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`[]`)}},
			"invalid metadata": {Type: provider.ContentCustom, Kind: "example.kind", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{`)}},
		} {
			t.Run(name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return customResult(part), nil
				}

				response := harness.serve(validRequest(`{"prompt":[]}`))

				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.JSONEq(t, string(canonicalInternalError), response.Body.String())
				assert.NotContains(t, response.Body.String(), "private")
			})
		}
	})

	t.Run("custom-only success is served once", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return customResult(provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"fallback"}`)}}), nil
		}

		response := harness.serve(validRequest(`{"prompt":[]}`))

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Equal(t, 1, harness.model.callCount())
		assert.Contains(t, response.Body.String(), `"kind":"anthropic.fallback"`)
	})
}
