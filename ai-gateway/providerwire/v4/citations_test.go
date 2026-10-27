package v4

import (
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCitationProjection_Validity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*provider.Citation)
		valid  bool
	}{
		{"negative index", func(c *provider.Citation) { c.StartIndex = new(-1) }, false},
		{"unsafe index", func(c *provider.Citation) { c.EndIndex = new(maxJavaScriptSafeInteger + 1) }, false},
		{"unknown source", func(c *provider.Citation) { c.Source.SourceType = "other" }, false},
		{"invalid source UTF8", func(c *provider.Citation) { c.Source.Title = string([]byte{255}) }, false},
		{"invalid cited text UTF8", func(c *provider.Citation) { c.CitedText = new(string([]byte{255})) }, false},
		{"invalid source metadata", func(c *provider.Citation) {
			c.Source.ProviderMetadata = provider.ProviderMetadata{"test": []byte(`[]`)}
		}, false},
		{"large cited text", func(c *provider.Citation) { c.CitedText = new(strings.Repeat("x", 2<<20)) }, true},
		{"large source metadata", func(c *provider.Citation) {
			c.Source.ProviderMetadata = provider.ProviderMetadata{"test": []byte(`{"x":"` + strings.Repeat("x", 2<<20) + `"}`)}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			citation := provider.Citation{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "u", URL: "url"}}
			tc.mutate(&citation)
			result := &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "answer", Citations: []provider.Citation{citation}}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
			_, err := mapUnarySuccess(result)
			_, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextEnd, id: "t", citations: []provider.Citation{citation}})
			if tc.valid {
				require.NoError(t, err)
				assert.True(t, ok)
			} else {
				require.Error(t, err)
				assert.False(t, ok)
			}
		})
	}
	citation := provider.Citation{Source: provider.SourceInfo{SourceType: provider.SourceTypeDocument, MediaType: "text/plain"}, CitedText: new("")}
	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartTextEnd, id: "t", citations: []provider.Citation{citation}})
	require.True(t, ok)
	assert.Contains(t, string(frame), `"citedText":""`)
	assert.Contains(t, string(frame), `"title":""`)
}
