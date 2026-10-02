package v4

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesProjectionPrivacyAndBounds(t *testing.T) {
	base := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native-id", Title: "Public title", MediaType: "text/plain", Filename: "public.txt", ProviderMetadata: provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"startPageNumber":1,"endPageNumber":2,"startCharIndex":-1,"endCharIndex":"bad","citedText":"secret","encryptedIndex":"secret"}`),
		"openai":    json.RawMessage(`{"type":"file_citation","fileId":"secret","index":3}`),
		"private":   json.RawMessage(`{"token":"secret"}`),
	}}
	mapped, err := mapSource(base, 4096)
	require.NoError(t, err)
	encoded, err := json.Marshal(mapped)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
	assert.Contains(t, string(encoded), `"id":"native-id"`)
	assert.Contains(t, string(encoded), `"citation":{"endPageNumber":2,"index":3,"startPageNumber":1}`)
	for _, delta := range []int64{-1, 0, 1} {
		source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: strings.Repeat("<", 80)}
		mapped, err := mapSource(source, 4096)
		require.NoError(t, err)
		b, err := json.Marshal(mapped)
		require.NoError(t, err)
		_, err = mapSource(source, int64(len(b))+delta)
		assert.Equal(t, delta < 0, err != nil)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*provider.SourceInfo)
	}{
		{"utf8", func(s *provider.SourceInfo) { s.Title = string([]byte{255}) }},
		{"unknown", func(s *provider.SourceInfo) { s.SourceType = provider.SourceType("other") }},
		{"huge metadata", func(s *provider.SourceInfo) {
			s.ProviderMetadata = provider.ProviderMetadata{"openai": json.RawMessage(strings.Repeat(" ", 8193))}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := base
			tc.mutate(&source)
			_, err := mapSource(source, 16384)
			require.Error(t, err)
		})
	}
	compiled := compileWireSchema(t, streamEventSchemaJSON)
	require.NoError(t, compiled.Validate(json.RawMessage(encoded)))
}

func TestSourcesOpenAIAndAzureMetadata(t *testing.T) {
	for _, tc := range []struct{ name, namespace, metadata, wantCitation string }{
		{"openai file citation", "openai", `{"type":"file_citation","fileId":"secret","index":4,"unapproved":"secret"}`, `"citation":{"index":4}`},
		{"azure file citation", "azure", `{"type":"file_citation","fileId":"secret","index":4,"unapproved":"secret"}`, `"citation":{"index":4}`},
		{"openai file path", "openai", `{"type":"file_path","fileId":"secret","index":0}`, `"citation":{"index":0}`},
		{"azure file path", "azure", `{"type":"file_path","fileId":"secret","index":0}`, `"citation":{"index":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native-id", Title: "Native title", Filename: "native.txt", MediaType: "text/plain", ProviderMetadata: provider.ProviderMetadata{tc.namespace: json.RawMessage(tc.metadata)}}
			mapped, err := mapSource(source, 4096)
			require.NoError(t, err)
			encoded, err := json.Marshal(mapped)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"title":"Native title"`)
			assert.Contains(t, string(encoded), `"filename":"native.txt"`)
			assert.Contains(t, string(encoded), `"id":"native-id"`)
			assert.Contains(t, string(encoded), tc.wantCitation)
			assert.NotContains(t, string(encoded), "secret")
			assert.NotContains(t, string(encoded), `"azure"`)
			assert.NotContains(t, string(encoded), `"openai"`)
		})
	}
	oversize := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: "https://example.com", ProviderMetadata: provider.ProviderMetadata{"azure": json.RawMessage(strings.Repeat(" ", maxSourceMetadataBytes+1))}}
	_, err := mapSource(oversize, 16384)
	require.ErrorIs(t, err, errInvalidUnarySuccess)
}

func TestSourcesStreamingLifecycle(t *testing.T) {
	source := provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "native-id", URL: "https://public.example", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"fileId":"secret","index":2}`)}}}
	for _, tc := range []struct {
		name    string
		parts   []provider.StreamPart
		failure bool
	}{
		{"interleaved", []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}, source, {Type: provider.PartTextDelta, ID: "text", Delta: "text"}, source, {Type: provider.PartTextEnd, ID: "text"}, finishPart()}, false},
		{"only source", []provider.StreamPart{source, finishPart()}, false},
		{"late metadata", []provider.StreamPart{source, {Type: provider.PartResponseMeta}}, true},
		{"nil source", []provider.StreamPart{{Type: provider.PartSource}}, true},
		{"error continuation", []provider.StreamPart{source, {Type: provider.PartError}, source, finishPart()}, true},
		{"post finish", []provider.StreamPart{finishPart(), source}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			requireStreamBodyMatchesSchema(t, body)
			assert.Equal(t, tc.failure, strings.Contains(body, `"type":"error"`))
			assert.NotContains(t, body, "secret")
			if tc.name == "interleaved" || tc.name == "error continuation" {
				assert.Equal(t, 2, strings.Count(body, `"id":"native-id"`))
				assert.Contains(t, body, `"type":"finish"`)
			}
			if tc.name == "post finish" {
				assert.NotContains(t, body, `"type":"source"`)
			}
		})
	}
}

func TestSourcesCompleteFrameAndAggregateBounds(t *testing.T) {
	source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: strings.Repeat("<", 80)}
	mapped, err := mapSource(source, 4096)
	require.NoError(t, err)
	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartSource, source: mapped}, 4096)
	require.True(t, ok)
	for _, delta := range []int64{-1, 0, 1} {
		_, ok := encodeStreamFrame(streamEvent{typeName: provider.PartSource, source: mapped}, int64(len(frame))+delta)
		assert.Equal(t, delta >= 0, ok)
	}
	part := provider.GenerateContentPart{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "id", URL: "url", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"ignored":"` + strings.Repeat("x", 400) + `"}`)}}
	result := &provider.GenerateResult{Content: []provider.GenerateContentPart{part, part, part}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
	_, err = mapUnarySuccess(result, 1024)
	require.Error(t, err)
	result.Content = result.Content[:1]
	_, err = mapUnarySuccess(result, 1024)
	require.NoError(t, err)
	for _, raw := range []string{`{"type":"source","sourceType":"document","id":"","mediaType":"","title":""}`, `{"type":"source","sourceType":"url","id":"","url":""}`} {
		for _, schemaJSON := range [][]byte{streamEventSchemaJSON, unarySuccessSchemaJSON} {
			compiled := compileWireSchema(t, schemaJSON)
			wrap := func(value string) string {
				if string(schemaJSON) == string(unarySuccessSchemaJSON) {
					return `{"content":[` + value + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
				}
				return value
			}
			require.NoError(t, compiled.Validate(json.RawMessage(wrap(raw))))
			for _, field := range []string{`"unknown":true`, `"providerMetadata":{"openai":{"fileId":"private"}}`, `"providerMetadata":{"citation":{"index":-1}}`} {
				invalid := strings.TrimSuffix(raw, "}") + "," + field + "}"
				require.Error(t, compiled.Validate(json.RawMessage(wrap(invalid))))
			}
		}
	}
}
