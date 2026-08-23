package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	t.Run("complete text warnings usage finish and metadata", func(t *testing.T) {
		zero := 0
		maximum := maxJavaScriptSafeInteger
		timestamp := time.Date(2026, 8, 22, 1, 2, 3, 456000000, time.FixedZone("offset", 2*60*60))
		result := &provider.GenerateResult{
			Content: []provider.GenerateContentPart{
				{Type: provider.ContentText, Text: ""},
				{Type: provider.ContentText, Text: "quote=\" slash=\\ newline=\n snowman=☃"},
			},
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther, Raw: ""},
			Usage: provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: &maximum, NoCache: &zero, CacheRead: &zero, CacheWrite: &zero},
				OutputTokens: provider.OutputTokenUsage{Total: &zero, Text: &zero, Reasoning: &zero},
				Raw:          json.RawMessage(`{"private":true}`),
			},
			Warnings: []provider.Warning{
				{Type: provider.WarnUnsupported, Feature: "", Details: "details"},
				{Type: provider.WarnCompatibility, Feature: ""},
				{Type: provider.WarnDeprecated, Setting: "", Message: ""},
				{Type: provider.WarnOther, Message: ""},
			},
			Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
				ID: "response-id", ModelID: "backend-model", Provider: "private-provider", Timestamp: timestamp,
			}},
		}

		mapped, err := mapUnarySuccess(result, "canonical/model", 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		require.True(t, json.Valid(body))
		h := newTestHandler(t, testLimits())
		require.NoError(t, h.unarySuccessSchema.Validate(body))
		assert.JSONEq(t, `{
			"content":[{"type":"text","text":""},{"type":"text","text":"quote=\" slash=\\ newline=\n snowman=☃"}],
			"finishReason":{"unified":"other"},
			"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0}},
			"warnings":[
				{"type":"unsupported","feature":"model capability","details":"a requested model capability is unsupported"},
				{"type":"compatibility","feature":"model compatibility","details":"a requested setting was adjusted for model compatibility"},
				{"type":"deprecated","setting":"model setting","message":"a requested model setting is deprecated"},
				{"type":"other","message":"the model reported a warning"}
			],
			"response":{"id":"response-id","modelId":"canonical/model","timestamp":"2026-08-21T23:02:03.456Z"}
		}`, string(body))
	})

	t.Run("timestamp normalizes sub-minute offsets without changing the instant", func(t *testing.T) {
		timestamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("seconds", 1))
		result := validGenerateResult()
		result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Timestamp: timestamp}}
		mapped, err := mapUnarySuccess(result, "canonical", 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		assert.Contains(t, string(body), `"timestamp":"2025-12-31T23:59:59Z"`)
		assert.Equal(t, timestamp.UnixNano(), mapped.timestamp.UnixNano())
	})

	t.Run("inactive warning fields are ignored", func(t *testing.T) {
		invalidUTF8 := string([]byte{0xff})
		result := validGenerateResult()
		result.Warnings = []provider.Warning{
			{Type: provider.WarnOther, Message: "ok", Feature: invalidUTF8, Details: strings.Repeat("x", 1<<20)},
			{Type: provider.WarnUnsupported, Feature: "feature", Setting: invalidUTF8, Message: invalidUTF8},
		}
		mapped, err := mapUnarySuccess(result, "canonical", 512)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 512)
		require.True(t, ok)
		assert.NotContains(t, string(body), "feature\":\"�")
		assert.NotContains(t, string(body), strings.Repeat("x", 32))
	})

	t.Run("all registered finish reasons", func(t *testing.T) {
		for _, reason := range []provider.UnifiedFinishReason{
			provider.FinishReasonStop,
			provider.FinishReasonLength,
			provider.FinishReasonContentFilter,
			provider.FinishReasonToolCalls,
			provider.FinishReasonError,
			provider.FinishReasonOther,
		} {
			result := validGenerateResult()
			result.FinishReason = provider.FinishReason{Unified: reason, Raw: "native"}
			mapped, err := mapUnarySuccess(result, "canonical", 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			assert.Contains(t, string(body), `"unified":"`+string(reason)+`","raw":"native"`)
		}
	})

	t.Run("absent response metadata still emits canonical identity", func(t *testing.T) {
		mapped, err := mapUnarySuccess(validGenerateResult(), "canonical/model", 1<<20)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		assert.Contains(t, string(body), `"response":{"modelId":"canonical/model"}`)
		assert.NotContains(t, string(body), `"id":`)
		assert.NotContains(t, string(body), `"timestamp":`)
	})

	t.Run("absent zero and maximum usage", func(t *testing.T) {
		zero := 0
		maximum := maxJavaScriptSafeInteger
		for _, usage := range []provider.Usage{
			{},
			{InputTokens: provider.InputTokenUsage{Total: &zero}, OutputTokens: provider.OutputTokenUsage{Reasoning: &zero}},
			{InputTokens: provider.InputTokenUsage{CacheRead: &maximum}, OutputTokens: provider.OutputTokenUsage{Total: &maximum}},
		} {
			result := validGenerateResult()
			result.Usage = usage
			mapped, err := mapUnarySuccess(result, "canonical", 1<<20)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 1<<20)
			require.True(t, ok)
			require.Contains(t, string(body), `"inputTokens":{`)
			require.Contains(t, string(body), `"outputTokens":{`)
		}
	})
}

func TestUnaryWarningNormalizationIsValueSafe(t *testing.T) {
	privateValues := []string{
		"credential=secret", "https://provider.invalid/private", `{"private":"body"}`,
		"Authorization: secret", "private-provider", "backend-private-model", "arbitrary provider prose",
	}
	warnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: privateValues[0], Details: privateValues[1]},
		{Type: provider.WarnCompatibility, Feature: privateValues[2], Details: privateValues[3]},
		{Type: provider.WarnDeprecated, Setting: privateValues[4], Message: privateValues[5]},
		{Type: provider.WarnOther, Message: privateValues[6]},
	}
	mapped, err := mapWarnings(warnings, 1<<20)
	require.NoError(t, err)
	result := validGenerateResult()
	result.Warnings = warnings
	response, err := mapUnarySuccess(result, "canonical/public", 1<<20)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(response, 1<<20)
	require.True(t, ok)
	for _, private := range privateValues {
		assert.NotContains(t, string(body), private)
	}
	assert.Equal(t, []unaryWarning{
		{typeName: provider.WarnUnsupported, feature: warningUnsupportedFeature, details: warningUnsupportedDetails},
		{typeName: provider.WarnCompatibility, feature: warningCompatibilityFeature, details: warningCompatibilityDetails},
		{typeName: provider.WarnDeprecated, setting: warningDeprecatedSetting, message: warningDeprecatedMessage},
		{typeName: provider.WarnOther, message: warningOtherMessage},
	}, mapped)
	assert.Contains(t, string(body), `"modelId":"canonical/public"`)

	empty, err := mapWarnings([]provider.Warning{
		{Type: provider.WarnUnsupported},
		{Type: provider.WarnCompatibility},
		{Type: provider.WarnDeprecated},
		{Type: provider.WarnOther},
	}, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, mapped, empty)

	_, err = mapWarnings([]provider.Warning{{Type: provider.WarningType("future")}}, 1<<20)
	assert.Error(t, err)
	tooMany := make([]provider.Warning, 1_000)
	for i := range tooMany {
		tooMany[i].Type = provider.WarnOther
	}
	_, err = mapWarnings(tooMany, 128)
	assert.Error(t, err)
}

func TestUnarySuccessMappingRejectsInvalidProviderResults(t *testing.T) {
	negative := -1
	tooLarge := maxJavaScriptSafeInteger + 1
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name    string
		result  *provider.GenerateResult
		modelID string
	}{
		{name: "nil result", modelID: "canonical"},
		{name: "non text content", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentReasoning}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, modelID: "canonical"},
		{name: "unknown content", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.GenerateContentType("future")}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, modelID: "canonical"},
		{name: "unknown warning", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Warnings: []provider.Warning{{Type: provider.WarningType("future")}}}, modelID: "canonical"},
		{name: "unknown finish", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.UnifiedFinishReason("future")}}, modelID: "canonical"},
		{name: "negative usage", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{InputTokens: provider.InputTokenUsage{Total: &negative}}}, modelID: "canonical"},
		{name: "usage above javascript maximum", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &tooLarge}}}, modelID: "canonical"},
		{name: "invalid content utf8", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: invalidUTF8}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, modelID: "canonical"},
		{name: "invalid finish utf8", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: invalidUTF8}}, modelID: "canonical"},
		{name: "invalid response id utf8", result: &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: invalidUTF8}}}, modelID: "canonical"},
		{name: "invalid model id utf8", result: validGenerateResult(), modelID: invalidUTF8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mapUnarySuccess(tc.result, tc.modelID, 1<<20)
			assert.Error(t, err)
			if tc.result != nil && tc.modelID == "canonical" {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return tc.result, nil
				}
				response := harness.serve(validRequest(`{"prompt":[]}`))
				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.Equal(t, string(canonicalInternalError), response.Body.String())
			}
		})
	}
}

func TestUnarySuccessPrivacyAndCanonicalIdentity(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	total := 3
	timestamp := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	harness.model.generate = func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{
			Content: []provider.GenerateContentPart{{
				Type: provider.ContentText, Text: "", ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"part-private-metadata"}`)},
			}},
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "native-stop"},
			Usage: provider.Usage{
				InputTokens: provider.InputTokenUsage{Total: &total},
				Raw:         json.RawMessage(`{"secret":"raw-usage-private"}`),
			},
			Warnings:         []provider.Warning{{Type: provider.WarnOther, Message: "public warning"}},
			ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"result-private-metadata"}`)},
			Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"secret":"request-private-body"}`)},
			Response: &provider.GenerateResponse{
				ResponseMetadata: provider.ResponseMetadata{ID: "response-public", ModelID: "backend-private", Provider: "provider-private", Timestamp: timestamp},
				Headers:          map[string]string{"Authorization": "secret-header"},
				Body:             json.RawMessage(`{"secret":"response-private-body"}`),
			},
		}, nil
	}
	req := validRequest(`{"prompt":[]}`)
	req.Header.Set(HeaderModelID, "alias-private")
	response := harness.serve(req)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	require.NoError(t, harness.handler.unarySuccessSchema.Validate(response.Body.Bytes()))
	assert.JSONEq(t, `{
		"content":[{"type":"text","text":""}],
		"finishReason":{"unified":"stop","raw":"native-stop"},
		"usage":{"inputTokens":{"total":3},"outputTokens":{}},
		"warnings":[{"type":"other","message":"the model reported a warning"}],
		"response":{"id":"response-public","modelId":"canonical/model","timestamp":"2026-08-22T00:00:00Z"}
	}`, response.Body.String())
	for _, private := range []string{
		"alias-private", "backend-private", "provider-private", "secret-header", "raw-usage-private",
		"request-private-body", "response-private-body", "result-private-metadata", "part-private-metadata", "Authorization",
	} {
		assert.NotContains(t, response.Body.String(), private)
	}
}

func TestUnarySuccessBoundaries(t *testing.T) {
	result := validGenerateResult()
	mapped, err := mapUnarySuccess(result, "canonical/model", 1<<20)
	require.NoError(t, err)
	complete, ok := encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)

	tests := []struct {
		name   string
		limit  int64
		status int
	}{
		{name: "below limit", limit: int64(len(complete) + 1), status: http.StatusOK},
		{name: "exact limit", limit: int64(len(complete)), status: http.StatusOK},
		{name: "one byte above limit", limit: int64(len(complete) - 1), status: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.UnaryResponseBytes = tc.limit
			harness := newRuntimeHarness(t, limits)
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.LessOrEqual(t, response.Body.Len(), int(max(tc.limit, limits.ErrorResponseBytes)))
		})
	}

	t.Run("error below exact and above boundary", func(t *testing.T) {
		ordinary, _, ok := encodeSafeError(safeError{category: safeInvalidRequest}, 1<<20)
		require.True(t, ok)
		for _, tc := range []struct {
			name   string
			limit  int64
			status int
		}{
			{name: "below", limit: int64(len(ordinary) + 1), status: http.StatusBadRequest},
			{name: "exact", limit: int64(len(ordinary)), status: http.StatusBadRequest},
			{name: "above", limit: int64(len(ordinary) - 1), status: http.StatusInternalServerError},
		} {
			t.Run(tc.name, func(t *testing.T) {
				limits := testLimits()
				limits.ErrorResponseBytes = tc.limit
				h := newTestHandler(t, limits)
				response := httptest.NewRecorder()
				h.writeSafeError(response, safeError{category: safeInvalidRequest})
				assert.Equal(t, tc.status, response.Code)
				assert.LessOrEqual(t, response.Body.Len(), int(tc.limit))
			})
		}
	})

	t.Run("oversized provider text and writer retain at most limit", func(t *testing.T) {
		oversized := strings.Repeat("x", 1<<20)
		result := validGenerateResult()
		result.Content[0].Text = oversized
		_, err := mapUnarySuccess(result, "canonical", 128)
		assert.Error(t, err)

		limits := testLimits()
		limits.UnaryResponseBytes = 128
		harness := newRuntimeHarness(t, limits)
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			result := validGenerateResult()
			result.Content[0].Text = oversized
			return result, nil
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)

		buffer := newBoundedDocument(64)
		buffer.appendJSONString(oversized)
		assert.True(t, buffer.overflow)
		assert.LessOrEqual(t, len(buffer.data), 64)
	})

	t.Run("huge content and warning slices fail before dto allocation", func(t *testing.T) {
		content := make([]provider.GenerateContentPart, 1_000)
		warnings := make([]provider.Warning, 1_000)
		for i := range content {
			content[i] = provider.GenerateContentPart{Type: provider.ContentText}
			warnings[i] = provider.Warning{Type: provider.WarnOther}
		}
		_, err := mapUnarySuccess(&provider.GenerateResult{Content: content, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, "canonical", 128)
		assert.Error(t, err)
		_, err = mapUnarySuccess(&provider.GenerateResult{Warnings: warnings, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, "canonical", 128)
		assert.Error(t, err)
	})

	t.Run("forced success schema rejection happens before commit", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		rejecting, err := schema.CompileSchema(json.RawMessage(`false`))
		require.NoError(t, err)
		harness.handler.unarySuccessSchema = rejecting
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
	})
}
