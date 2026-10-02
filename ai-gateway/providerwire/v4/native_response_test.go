package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nativeWarnings() []provider.Warning {
	return []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "native model ☃", Details: "limit=42", Message: "inactive"},
		{Type: provider.WarnCompatibility, Feature: "", Details: "", Setting: "inactive"},
		{Type: provider.WarnDeprecated, Setting: "", Message: "use native setting", Details: "inactive"},
		{Type: provider.WarnOther, Message: "", Feature: "inactive"},
		{Type: provider.WarnOther, Message: "ordinary token-looking text sk-application"},
	}
}

const nativeWarningsJSON = `[{"type":"unsupported","feature":"native model ☃","details":"limit=42"},{"type":"compatibility","feature":""},{"type":"deprecated","setting":"","message":"use native setting"},{"type":"other","message":""},{"type":"other","message":"ordinary token-looking text sk-application"}]`

func TestNativeResponse_Warnings(t *testing.T) {
	t.Run("unary", func(t *testing.T) {
		result := validGenerateResult()
		result.Warnings = nativeWarnings()
		mapped, err := mapUnarySuccess(result, 4096)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 4096)
		require.True(t, ok)
		var value map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &value))
		assert.JSONEq(t, nativeWarningsJSON, string(value["warnings"]))
	})
	t.Run("stream", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: nativeWarnings()}, finishPart())}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		first := strings.TrimPrefix(strings.Split(body, "\n\n")[0], "data: ")
		assert.JSONEq(t, `{"type":"stream-start","warnings":`+nativeWarningsJSON+`}`, first)
	})
}

func TestNativeResponse_Sources(t *testing.T) {
	for _, id := range []string{"native-id", "", strings.Repeat("i", 1025)} {
		t.Run("id length "+strconv.Itoa(len(id)), func(t *testing.T) {
			parts := []provider.GenerateContentPart{
				{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: id, URL: "https://example.com", Title: "native"},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: id, URL: "https://example.com"},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: id, MediaType: "text/plain", Title: "file-native", Text: "legacy", Filename: "file-native", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"type":"file_path","index":0}`)}},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: id},
			}
			result := validGenerateResult()
			result.Content = parts
			mapped, err := mapUnarySuccess(result, 16384)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 16384)
			require.True(t, ok)
			var value struct {
				Content []map[string]any `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body, &value))
			require.Len(t, value.Content, 4)
			for _, source := range value.Content {
				assert.Equal(t, id, source["id"])
			}
			assert.Equal(t, "file-native", value.Content[2]["title"])
			assert.Equal(t, "file-native", value.Content[2]["filename"])
			assert.Equal(t, "", value.Content[3]["title"])
			assert.NotContains(t, value.Content[3], "filename")
			harness := newRuntimeHarness(t, testLimits())
			streamParts := make([]provider.StreamPart, 0, len(parts)+1)
			for _, part := range parts {
				source := unarySource(part)
				streamParts = append(streamParts, provider.StreamPart{Type: provider.PartSource, Source: &source})
			}
			streamParts = append(streamParts, finishPart())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(streamParts...)}, nil
			}
			frames := strings.Split(strings.TrimSpace(harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()), "\n\n")
			require.Len(t, frames, 6)
			for i, source := range value.Content {
				raw, err := json.Marshal(source)
				require.NoError(t, err)
				assert.JSONEq(t, string(raw), strings.TrimPrefix(frames[i+1], "data: "))
			}
		})
	}
}

func TestNativeResponse_Identity(t *testing.T) {
	timestamp := time.Date(2026, 9, 30, 12, 34, 56, 123456789, time.FixedZone("offset", 3600))
	for _, tc := range []struct {
		name     string
		response *provider.GenerateResponse
		want     string
	}{
		{"absent", nil, ""},
		{"empty", &provider.GenerateResponse{}, `{}`},
		{"partial", &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response"}}, `{"id":"native-response"}`},
		{"full", &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response", ModelID: "native model/☃", Timestamp: timestamp, Provider: "not-transported"}, Headers: map[string]string{"Authorization": "not-transported"}, Body: json.RawMessage(`{"private":"not-transported"}`)}, `{"id":"native-response","modelId":"native model/☃","timestamp":"2026-09-30T11:34:56.123456789Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Response = tc.response
			mapped, err := mapUnarySuccess(result, 4096)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 4096)
			require.True(t, ok)
			var value map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &value))
			if tc.want == "" {
				assert.NotContains(t, value, "response")
			} else {
				assert.JSONEq(t, tc.want, string(value["response"]))
			}
			assert.NotContains(t, string(body), "not-transported")
			if tc.response == nil {
				return
			}
			harness := newRuntimeHarness(t, testLimits())
			meta := tc.response.ResponseMetadata
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: meta.ID, ModelID: meta.ModelID, Timestamp: meta.Timestamp}, finishPart())}, nil
			}
			frames := strings.Split(harness.serve(streamRequest(`{"prompt":[]}`)).Body.String(), "\n\n")
			var event map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &event))
			assert.JSONEq(t, `"response-metadata"`, string(event["type"]))
			delete(event, "type")
			encoded, err := json.Marshal(event)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

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

func TestNativeResponse_ConsumerObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	harness := newRuntimeHarness(t, testLimits())
	source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native-source", Title: "file-native", Filename: "file-native", MediaType: "text/plain"}
	identity := provider.ResponseMetadata{ID: "native-response", ModelID: "native model ☃", Timestamp: time.Date(2026, 9, 30, 12, 0, 0, 123000000, time.UTC)}
	harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, Title: source.Title, Filename: source.Filename, MediaType: source.MediaType}}, Warnings: nativeWarnings(), Response: &provider.GenerateResponse{ResponseMetadata: identity}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: nativeWarnings()}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: identity.ID, ModelID: identity.ModelID, Timestamp: identity.Timestamp}, provider.StreamPart{Type: provider.PartSource, Source: &source}, finishPart())}, nil
	}
	server := httptest.NewServer(harness.handler)
	defer server.Close()
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "dummy-client-token", BaseURL: server.URL})
	require.NoError(t, err)
	model, err := client.LanguageModel("requested-alias")
	require.NoError(t, err)
	var seen *provider.GenerateResult
	observed := make(chan provider.StreamPart, 4)
	observer := middleware.Middleware{
		WrapGenerate: func(ctx context.Context, params middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			result, err := params.DoGenerate(ctx)
			if err == nil {
				seen = result
			}
			return result, err
		},
		WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
			result, err := params.DoStream(ctx)
			if err != nil {
				return nil, err
			}
			forwarded := make(chan provider.StreamPart)
			go func() {
				defer close(forwarded)
				defer close(observed)
				for {
					select {
					case part, ok := <-result.Stream:
						if !ok {
							return
						}
						select {
						case observed <- part:
						case <-ctx.Done():
							return
						}
						select {
						case forwarded <- part:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			copy := *result
			copy.Stream = forwarded
			return &copy, nil
		},
	}
	var consumerLogs bytes.Buffer
	wrapped := middleware.WrapLanguageModel(model, observer, logger.Middleware(logger.Options{Logger: slog.New(slog.NewJSONHandler(&consumerLogs, nil)), Capture: logger.CaptureOptions{ResponseBody: true, MaxJSONBytes: 16384}}))
	result, err := wrapped.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	require.Same(t, seen, result)
	require.NotNil(t, result.Response)
	assert.Empty(t, result.Response.ID)
	assert.Empty(t, result.Response.ModelID)
	assert.True(t, result.Response.Timestamp.IsZero())
	assert.Equal(t, source.ID, result.Content[0].ID)
	assert.Equal(t, source.Title, result.Content[0].Title)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.Response.Body, &body))
	assert.JSONEq(t, nativeWarningsJSON, string(body["warnings"]))
	assert.JSONEq(t, `{"id":"native-response","modelId":"native model ☃","timestamp":"2026-09-30T12:00:00.123Z"}`, string(body["response"]))
	var captured map[string]json.RawMessage
	for _, line := range bytes.Split(bytes.TrimSpace(consumerLogs.Bytes()), []byte{'\n'}) {
		var record map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &record))
		if value, ok := record["ai_sdk.response.body"]; ok {
			require.NoError(t, json.Unmarshal(value, &captured))
		}
	}
	require.NotNil(t, captured, "actual consumer log destination must contain the response body")
	for _, field := range []string{"warnings", "response", "content"} {
		assert.JSONEq(t, string(body[field]), string(captured[field]))
	}
	stream, err := wrapped.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	var returned []provider.StreamPart
	for part := range stream.Stream {
		returned = append(returned, part)
	}
	var received []provider.StreamPart
	for part := range observed {
		received = append(received, part)
	}
	require.Len(t, returned, 4)
	assert.Equal(t, returned, received)
	assert.Equal(t, identity.ID, returned[1].ResponseID)
	assert.Equal(t, identity.ModelID, returned[1].ModelID)
	assert.Equal(t, identity.Timestamp, returned[1].Timestamp)
	assert.Equal(t, source, *returned[2].Source)
	assert.Equal(t, "requested-alias", harness.resolver.requestedModelID())
	generate, streaming := harness.model.invocationCounts()
	assert.Equal(t, 1, generate)
	assert.Equal(t, 1, streaming)
	assert.NotContains(t, consumerLogs.String(), "dummy-client-token")
}
