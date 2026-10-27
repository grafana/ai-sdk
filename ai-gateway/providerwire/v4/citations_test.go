package v4

import (
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCitationProjection_BoundsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*provider.Citation)
	}{
		{"negative index", func(c *provider.Citation) { c.StartIndex = new(-1) }},
		{"unsafe index", func(c *provider.Citation) { c.EndIndex = new(maxJavaScriptSafeInteger + 1) }},
		{"unknown source", func(c *provider.Citation) { c.Source.SourceType = "other" }},
		{"invalid source UTF8", func(c *provider.Citation) { c.Source.Title = string([]byte{255}) }},
		{"invalid cited text UTF8", func(c *provider.Citation) { c.CitedText = new(string([]byte{255})) }},
		{"oversized cited text", func(c *provider.Citation) { c.CitedText = new(strings.Repeat("x", 1025)) }},
		{"oversized source metadata", func(c *provider.Citation) {
			c.Source.ProviderMetadata = provider.ProviderMetadata{"test": []byte(`{"x":"` + strings.Repeat("x", 1025) + `"}`)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			citation := provider.Citation{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "u", URL: "url"}}
			tc.mutate(&citation)
			result := &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "answer", Citations: []provider.Citation{citation}}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
			_, err := mapUnarySuccess(result, 1024)
			require.Error(t, err)
			_, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextEnd, id: "t", citations: []provider.Citation{citation}}, 1024)
			assert.False(t, ok)
		})
	}
	citation := provider.Citation{Source: provider.SourceInfo{SourceType: provider.SourceTypeDocument, MediaType: "text/plain"}, CitedText: new("")}
	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextEnd, id: "t", citations: []provider.Citation{citation}}, 1024)
	require.True(t, ok)
	assert.Contains(t, string(frame), `"citedText":""`)
	assert.Contains(t, string(frame), `"title":""`)
	remaining := int64(150)
	assert.False(t, citationsPreflight([]provider.Citation{{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, URL: strings.Repeat("x", 80)}}, {Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, URL: strings.Repeat("x", 80)}}}, &remaining))
}
