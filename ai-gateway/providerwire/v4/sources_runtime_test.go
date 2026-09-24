package v4

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesProjectionPrivacyAndBounds(t *testing.T) {
	base := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "private-id", Title: "Public title", MediaType: "text/plain", Filename: "public.txt", ProviderMetadata: provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"startPageNumber":1,"endPageNumber":2,"startCharIndex":-1,"endCharIndex":"bad","citedText":"secret","encryptedIndex":"secret"}`),
		"openai":    json.RawMessage(`{"type":"file_citation","fileId":"secret","index":3}`),
		"private":   json.RawMessage(`{"token":"secret"}`),
	}}
	ids := make(sourceIDs)
	mapped, err := mapSource(base, ids, 4096)
	require.NoError(t, err)
	encoded, err := json.Marshal(mapped)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
	assert.NotContains(t, string(encoded), "private-id")
	assert.Contains(t, string(encoded), `"citation":{"endPageNumber":2,"index":3,"startPageNumber":1}`)
	for _, delta := range []int64{-1, 0, 1} {
		source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: strings.Repeat("<", 80)}
		mapped, err := mapSource(source, make(sourceIDs), 4096)
		require.NoError(t, err)
		b, _ := json.Marshal(mapped)
		_, err = mapSource(source, make(sourceIDs), int64(len(b))+delta)
		assert.Equal(t, delta < 0, err != nil)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*provider.SourceInfo)
	}{
		{"empty id", func(s *provider.SourceInfo) { s.ID = "" }},
		{"long id", func(s *provider.SourceInfo) { s.ID = strings.Repeat("a", 1025) }},
		{"utf8", func(s *provider.SourceInfo) { s.Title = string([]byte{255}) }},
		{"unknown", func(s *provider.SourceInfo) { s.SourceType = "other" }},
		{"huge metadata", func(s *provider.SourceInfo) {
			s.ProviderMetadata = provider.ProviderMetadata{"openai": json.RawMessage(strings.Repeat(" ", 8193))}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := base
			tc.mutate(&source)
			ids := make(sourceIDs)
			_, err := mapSource(source, ids, 16384)
			require.Error(t, err)
			assert.Empty(t, ids)
		})
	}
	document := base
	document.ProviderMetadata = provider.ProviderMetadata{"openai": json.RawMessage(`{"type":"file_path","fileId":"file-secret","index":0}`)}
	document.Title, document.Filename = "file-secret", "file-secret"
	mapped, err = mapSource(document, make(sourceIDs), 4096)
	require.NoError(t, err)
	encoded, _ = json.Marshal(mapped)
	assert.Contains(t, string(encoded), `"title":"Document"`)
	assert.NotContains(t, string(encoded), "file-secret")
	assert.NotContains(t, string(encoded), "filename")
	document.SourceType = provider.SourceTypeURL
	_, err = mapSource(document, ids, 4096)
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	compiled, err := schema.CompileSchema(streamEventSchemaJSON)
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(json.RawMessage(encoded)))
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
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			body := response.Body.String()
			requireStreamBodyMatchesSchema(t, body)
			assert.Equal(t, tc.failure, strings.Contains(body, `"type":"error"`))
			assert.NotContains(t, body, "native-id")
			assert.NotContains(t, body, "secret")
			if tc.name == "interleaved" {
				assert.Equal(t, 2, strings.Count(body, `"id":"source-1"`))
				assert.Contains(t, body, `"type":"finish"`)
			}
			if tc.name == "post finish" {
				assert.NotContains(t, body, `"type":"source"`)
			}
			if tc.name == "error continuation" {
				assert.Equal(t, 2, strings.Count(body, `"type":"source"`))
				assert.Contains(t, body, `"type":"finish"`)
			}
		})
	}
}

func TestSourcesCompleteFrameAndAggregateBounds(t *testing.T) {
	source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: strings.Repeat("<", 80)}
	mapped, err := mapSource(source, make(sourceIDs), 4096)
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
	for _, raw := range []string{
		`{"type":"source","sourceType":"document","id":"source-1","mediaType":"","title":""}`,
		`{"type":"source","sourceType":"url","id":"source-1","url":""}`,
	} {
		for _, schemaJSON := range [][]byte{streamEventSchemaJSON, unarySuccessSchemaJSON} {
			compiled, err := schema.CompileSchema(schemaJSON)
			require.NoError(t, err)
			wrap := func(value string) string {
				if string(schemaJSON) == string(unarySuccessSchemaJSON) {
					return `{"content":[` + value + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
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
