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

	mapped, err := mapUnarySuccess(result)
	require.NoError(t, err)
	body, ok := marshalUnarySuccess(mapped)
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
			provider.FinishReasonInterrupted,
			provider.FinishReasonOther,
		} {
			result := validGenerateResult()
			result.FinishReason.Unified = reason
			_, err := mapUnarySuccess(result)
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
			_, err := mapUnarySuccess(result)
			require.Error(t, err)
		}
	})
}

func TestUnaryProviderToolResult_LargeAndPreliminary(t *testing.T) {
	call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`)}
	result := provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"data":"` + strings.Repeat("a", 9<<20) + `"}`)}
	result.ProviderMetadata = provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 1000) + `"}`)}
	value := validGenerateResult()
	value.Content = []provider.GenerateContentPart{call, result}
	mapped, err := mapUnarySuccess(value)
	require.NoError(t, err)
	body, ok := marshalUnarySuccess(mapped)
	require.True(t, ok)
	assert.Greater(t, len(body), 9<<20)
	result.Preliminary = new(true)
	value.Content = []provider.GenerateContentPart{call, result}
	_, err = mapUnarySuccess(value)
	require.Error(t, err)
}

func TestUnarySuccess_NoSizeLimit(t *testing.T) {
	t.Run("large text", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = strings.Repeat("x", 9<<20)
		recorder := httptest.NewRecorder()
		require.True(t, newTestHandler(t).writeUnarySuccess(recorder, result))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Greater(t, recorder.Body.Len(), 9<<20)
	})

	t.Run("many content parts", func(t *testing.T) {
		result := validGenerateResult()
		result.Content = make([]provider.GenerateContentPart, 100_001)
		for i := range result.Content {
			result.Content[i] = provider.GenerateContentPart{Type: provider.ContentText, Text: "x"}
		}
		mapped, err := mapUnarySuccess(result)
		require.NoError(t, err)
		assert.Len(t, mapped.Content, 100_001)
	})

	t.Run("escaping expansion succeeds", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = strings.Repeat("\x00", 1<<20)
		recorder := httptest.NewRecorder()
		require.True(t, newTestHandler(t).writeUnarySuccess(recorder, result))
		assert.Contains(t, recorder.Body.String(), `\u0000`)
		assert.Greater(t, recorder.Body.Len(), 6<<20)
	})

	t.Run("invalid UTF-8 fails before success commitment", func(t *testing.T) {
		result := validGenerateResult()
		result.Content[0].Text = string([]byte{0xff})
		h := newTestHandler(t)
		recorder := httptest.NewRecorder()
		assert.False(t, h.writeUnarySuccess(recorder, result))
		assert.Empty(t, recorder.Body.String())
	})

	t.Run("invalid finish reason UTF-8 fails", func(t *testing.T) {
		result := validGenerateResult()
		result.FinishReason.Raw = string([]byte{0xff})
		_, err := mapUnarySuccess(result)
		require.Error(t, err)
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
			mapped, err := mapUnarySuccess(customResult(tc.part))
			require.NoError(t, err)
			body, ok := marshalUnarySuccess(mapped)
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
		))
		require.NoError(t, err)
		body, ok := marshalUnarySuccess(mapped)
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

	t.Run("large metadata and kind succeed", func(t *testing.T) {
		part := provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "example." + strings.Repeat("k", 9<<20), ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 9<<20) + `"}`)}}
		mapped, err := mapUnarySuccess(customResult(part))
		require.NoError(t, err)
		body, ok := marshalUnarySuccess(mapped)
		require.True(t, ok)
		assert.Greater(t, len(body), 18<<20)
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

func marshalUnarySuccess(value unarySuccess) ([]byte, bool) {
	body, err := json.Marshal(value)
	return body, err == nil
}
