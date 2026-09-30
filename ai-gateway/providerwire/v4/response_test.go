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

func TestCallerResponse_WarningsAndIdentity(t *testing.T) {
	warnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "", Details: "detail", Message: "inactive"},
		{Type: provider.WarnCompatibility, Feature: "feature"},
		{Type: provider.WarnDeprecated, Setting: "", Message: "", Details: "inactive"},
		{Type: provider.WarnOther, Message: "caller warning <>&", Feature: "inactive"},
	}
	timestamp := time.Date(2026, 9, 29, 12, 30, 0, 123000000, time.FixedZone("native", 3600))
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			result := validGenerateResult()
			result.Warnings = warnings
			result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-id", ModelID: "native-model", Timestamp: timestamp, Provider: "excluded-provider"}, Headers: map[string]string{"Authorization": "excluded-key"}, Body: json.RawMessage(`{"excluded":"body"}`)}
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "native-id", ModelID: "native-model", Timestamp: timestamp}, finishPart())}, nil
			}
			request := validRequest(`{"prompt":[]}`)
			if streaming {
				request = streamRequest(`{"prompt":[]}`)
			}
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code)
			var warningJSON, identityJSON json.RawMessage
			if streaming {
				requireStreamBodyMatchesSchema(t, response.Body.String())
				frames := strings.Split(strings.TrimSpace(response.Body.String()), "\n\n")
				var start map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &start))
				warningJSON = start["warnings"]
				identityJSON = json.RawMessage(strings.TrimPrefix(frames[1], "data: "))
				assert.JSONEq(t, `{"type":"response-metadata","id":"native-id","modelId":"native-model","timestamp":"2026-09-29T11:30:00.123Z"}`, string(identityJSON))
			} else {
				compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
				require.NoError(t, err)
				require.NoError(t, compiled.Validate(response.Body.Bytes()))
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				warningJSON, identityJSON = body["warnings"], body["response"]
				assert.JSONEq(t, `{"id":"native-id","modelId":"native-model","timestamp":"2026-09-29T11:30:00.123Z"}`, string(identityJSON))
			}
			assert.JSONEq(t, `[{"type":"unsupported","feature":"","details":"detail"},{"type":"compatibility","feature":"feature"},{"type":"deprecated","setting":"","message":""},{"type":"other","message":"caller warning <>&"}]`, string(warningJSON))
			for _, excluded := range []string{"inactive", "excluded", "canonical/model"} {
				assert.NotContains(t, response.Body.String(), excluded)
			}
		})
	}
}

func TestCallerResponse_InvalidWarningsAndIdentity(t *testing.T) {
	invalid := string([]byte{0xff})
	for _, tc := range []struct {
		name    string
		warning provider.Warning
	}{
		{name: "unknown", warning: provider.Warning{Type: provider.WarningType("future")}},
		{name: "utf8", warning: provider.Warning{Type: provider.WarnOther, Message: invalid}},
		{name: "message bytes", warning: provider.Warning{Type: provider.WarnOther, Message: strings.Repeat("x", 4097)}},
		{name: "feature bytes", warning: provider.Warning{Type: provider.WarnUnsupported, Feature: strings.Repeat("☃", 1366)}},
		{name: "details bytes", warning: provider.Warning{Type: provider.WarnCompatibility, Details: strings.Repeat("x", 4097)}},
		{name: "setting bytes", warning: provider.Warning{Type: provider.WarnDeprecated, Setting: strings.Repeat("x", 4097)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Warnings = []provider.Warning{tc.warning}
			_, err := mapUnarySuccess(result, 1<<20)
			require.Error(t, err)
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: result.Warnings}, finishPart())}, nil
			}
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), response.Body.String())
		})
	}
	for _, metadata := range []provider.ResponseMetadata{
		{ID: invalid}, {ModelID: invalid}, {Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		result := validGenerateResult()
		result.Response = &provider.GenerateResponse{ResponseMetadata: metadata}
		_, err := mapUnarySuccess(result, 1<<20)
		require.Error(t, err)
	}
	t.Run("empty identity is not canonical", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartResponseMeta}, finishPart())}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Contains(t, body, `data: {"type":"response-metadata"}`)
		assert.NotContains(t, body, "modelId")
		requireStreamBodyMatchesSchema(t, body)
	})
	t.Run("escaped warning complete bound", func(t *testing.T) {
		result := validGenerateResult()
		result.Warnings = []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("<", 4096)}}
		mapped, err := mapUnarySuccess(result, 1<<20)
		require.NoError(t, err)
		encoded, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		_, ok = encodeUnarySuccess(mapped, int64(len(encoded)-1))
		assert.False(t, ok)
		_, err = mapUnarySuccess(result, 4095)
		require.Error(t, err)
	})
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
		Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "caller warning"}},
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
		"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0},"raw":{"native_usage":true}},
		"warnings":[{"type":"other","message":"caller warning"}],"response":{"id":"private-response","modelId":"private-model"}
	}`, string(body))
	assert.Contains(t, string(body), `html=\u003c\u003e\u0026`)
	for _, private := range []string{"private-provider", "headers"} {
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
