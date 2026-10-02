package v4

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeResponse_CompleteBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *provider.GenerateResult
		part   provider.StreamPart
	}{
		{"warnings", &provider.GenerateResult{Warnings: []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("<\x00\\\"", 80)}}}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("<\x00\\\"", 80)}}}},
		{"source", &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: strings.Repeat("i", 1100), Title: strings.Repeat("<", 80)}}}, provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: strings.Repeat("i", 1100), Title: strings.Repeat("<", 80)}}},
		{"identity", &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: strings.Repeat("<", 80), ModelID: "native model ☃", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC)}}}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: strings.Repeat("<", 80), ModelID: "native model ☃", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.result.FinishReason.Unified = provider.FinishReasonStop
			mapped, err := mapUnarySuccess(tc.result, 16384)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 16384)
			require.True(t, ok)
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.part, finishPart())}, nil
			}
			frames := strings.Split(harness.serve(streamRequest(`{"prompt":[]}`)).Body.String(), "\n\n")
			i := 1
			if tc.part.Type == provider.PartStreamStart {
				i = 0
			}
			frame := frames[i] + "\n\n"
			for _, delta := range []int64{-1, 0, 1} {
				limits := testLimits()
				limits.UnaryResponseBytes = int64(len(body)) + delta
				h := newTestHandler(t, limits)
				w := httptest.NewRecorder()
				assert.Equal(t, delta >= 0, h.writeUnarySuccess(w, tc.result))
				if delta < 0 {
					assert.Empty(t, w.Body.String())
				} else {
					assert.Equal(t, body, w.Body.Bytes())
				}
				limits.StreamFrameBytes = int64(len(frame)) + delta
				harness := newRuntimeHarness(t, limits)
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(tc.part, finishPart())}, nil
				}
				got := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
				requireStreamBodyMatchesSchema(t, got)
				if delta < 0 {
					assert.NotContains(t, got, frame)
					assert.Equal(t, 1, strings.Count(got, `"code":"internal_error"`))
				} else {
					assert.Contains(t, got, frame)
					assert.Contains(t, got, `"type":"finish"`)
				}
			}
		})
	}
}

func TestNativeResponse_SourceUTF8(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*provider.SourceInfo)
	}{
		{"id", func(s *provider.SourceInfo) { s.ID = string([]byte{255}) }},
		{"url", func(s *provider.SourceInfo) { s.URL = string([]byte{255}) }},
		{"title", func(s *provider.SourceInfo) { s.Title = string([]byte{255}) }},
		{"media type", func(s *provider.SourceInfo) { s.MediaType = string([]byte{255}) }},
		{"filename", func(s *provider.SourceInfo) { s.Filename = string([]byte{255}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native"}
			tc.mutate(&source)
			result := validGenerateResult()
			result.Content = []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title, MediaType: source.MediaType, Filename: source.Filename}}
			_, err := mapUnarySuccess(result, 4096)
			require.Error(t, err)
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartSource, Source: &source}, finishPart())}, nil
			}
			assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), harness.serve(streamRequest(`{"prompt":[]}`)).Body.String())
		})
	}
}

func TestNativeResponse_AggregatePreflight(t *testing.T) {
	result := validGenerateResult()
	result.Content[0].Text = strings.Repeat("c", 80)
	result.Warnings = []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("w", 80)}}
	result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: strings.Repeat("i", 80)}}
	assert.False(t, unarySuccessPreflight(result, 200))
	assert.True(t, unarySuccessPreflight(result, 400))
	mapped, err := mapUnarySuccess(result, 4096)
	require.NoError(t, err)
	body, ok := encodeUnarySuccess(mapped, 4096)
	require.True(t, ok)
	for _, delta := range []int64{-1, 0, 1} {
		_, ok := encodeUnarySuccess(mapped, int64(len(body))+delta)
		assert.Equal(t, delta >= 0, ok)
	}
	result.Warnings = make([]provider.Warning, 1000)
	assert.False(t, unarySuccessPreflight(result, 400))
	warnings := []provider.Warning{{Type: provider.WarnOther, Message: string([]byte{255})}, {Type: provider.WarnOther, Message: strings.Repeat("x", 1000)}}
	remaining := int64(400)
	assert.False(t, warningsPreflight(warnings, &remaining))
	_, err = mapStreamWarnings(warnings, 400)
	require.Error(t, err)
}

func TestNativeResponse_InvalidValues(t *testing.T) {
	invalid := string([]byte{255})
	for _, tc := range []struct {
		name   string
		mutate func(*provider.GenerateResult)
		part   provider.StreamPart
	}{
		{"warning feature", func(r *provider.GenerateResult) {
			r.Warnings = []provider.Warning{{Type: provider.WarnUnsupported, Feature: invalid}}
		}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: invalid}}}},
		{"warning details", func(r *provider.GenerateResult) {
			r.Warnings = []provider.Warning{{Type: provider.WarnCompatibility, Details: invalid}}
		}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnCompatibility, Details: invalid}}}},
		{"warning setting", func(r *provider.GenerateResult) {
			r.Warnings = []provider.Warning{{Type: provider.WarnDeprecated, Setting: invalid}}
		}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnDeprecated, Setting: invalid}}}},
		{"warning message", func(r *provider.GenerateResult) {
			r.Warnings = []provider.Warning{{Type: provider.WarnOther, Message: invalid}}
		}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: invalid}}}},
		{"warning type", func(r *provider.GenerateResult) {
			r.Warnings = []provider.Warning{{Type: provider.WarningType("future")}}
		}, provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarningType("future")}}}},
		{"response id", func(r *provider.GenerateResult) {
			r.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: invalid}}
		}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: invalid}},
		{"response model", func(r *provider.GenerateResult) {
			r.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ModelID: invalid}}
		}, provider.StreamPart{Type: provider.PartResponseMeta, ModelID: invalid}},
		{"timestamp", func(r *provider.GenerateResult) {
			r.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
		}, provider.StreamPart{Type: provider.PartResponseMeta, Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			tc.mutate(result)
			_, err := mapUnarySuccess(result, 4096)
			require.Error(t, err)
			h := newTestHandler(t, testLimits())
			w := httptest.NewRecorder()
			assert.False(t, h.writeUnarySuccess(w, result))
			assert.Empty(t, w.Body.String())
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.part, finishPart())}, nil
			}
			assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), harness.serve(streamRequest(`{"prompt":[]}`)).Body.String())
		})
	}
}

func TestNativeResponse_Schema(t *testing.T) {
	for _, unary := range []bool{false, true} {
		data := streamEventSchemaJSON
		if unary {
			data = unarySuccessSchemaJSON
		}
		compiled, err := schema.CompileSchema(data)
		require.NoError(t, err)
		wrap := func(warnings, identity string) string {
			if unary {
				return `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":` + warnings + `,"response":` + identity + `}`
			}
			return `{"type":"stream-start","warnings":` + warnings + `}`
		}
		require.NoError(t, compiled.Validate(json.RawMessage(wrap(nativeWarningsJSON, `{}`))))
		for _, warning := range []string{`{"type":"other"}`, `{"type":"other","message":null}`, `{"type":"other","message":"","feature":"inactive"}`, `{"type":"deprecated","message":""}`, `{"type":"unsupported","feature":1}`, `{"type":"compatibility","feature":"","details":null}`, `{"type":"unknown"}`} {
			assert.Error(t, compiled.Validate(json.RawMessage(wrap("["+warning+"]", `{}`))), warning)
		}
		for _, identity := range []string{`{}`, `{"id":"","modelId":""}`, `{"id":"native","modelId":"native model ☃","timestamp":"2026-01-01T00:00:00.123456789Z"}`} {
			value := wrap(`[]`, identity)
			if !unary {
				value = `{"type":"response-metadata",` + strings.TrimPrefix(identity, "{")
				if identity == `{}` {
					value = `{"type":"response-metadata"}`
				}
			}
			require.NoError(t, compiled.Validate(json.RawMessage(value)))
		}
		for _, identity := range []string{`{"id":null}`, `{"modelId":1}`, `{"timestamp":null}`, `{"timestamp":"bad"}`, `{"timestamp":"2026-01-01T00:00:00,123Z"}`, `{"provider":"inactive"}`} {
			value := wrap(`[]`, identity)
			if !unary {
				value = `{"type":"response-metadata",` + strings.TrimPrefix(identity, "{")
			}
			assert.Error(t, compiled.Validate(json.RawMessage(value)), value)
		}
	}
}
