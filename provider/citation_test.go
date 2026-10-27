package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCitation_MarshalJSON(t *testing.T) {
	start, end, cited := 0, 4, ""
	cases := []struct {
		name     string
		citation Citation
		want     string
	}{
		{
			name:     "url without title omits optional fields",
			citation: Citation{Source: SourceInfo{SourceType: SourceTypeURL, ID: "u", URL: "https://example.com", Filename: "ignored"}},
			want:     `{"source":{"type":"source","sourceType":"url","id":"u","url":"https://example.com"}}`,
		},
		{
			name:     "url with title, offsets and metadata",
			citation: Citation{Source: SourceInfo{SourceType: SourceTypeURL, ID: "u", URL: "https://example.com", Title: "Example", ProviderMetadata: ProviderMetadata{"test": json.RawMessage(`{"a":1}`)}}, StartIndex: &start, EndIndex: &end},
			want:     `{"source":{"type":"source","sourceType":"url","id":"u","url":"https://example.com","title":"Example","providerMetadata":{"test":{"a":1}}},"startIndex":0,"endIndex":4}`,
		},
		{
			name:     "document keeps required empty fields",
			citation: Citation{Source: SourceInfo{SourceType: SourceTypeDocument, URL: "ignored"}, CitedText: &cited},
			want:     `{"source":{"type":"source","sourceType":"document","id":"","mediaType":"","title":""},"citedText":""}`,
		},
		{
			name:     "empty metadata map is kept",
			citation: Citation{Source: SourceInfo{SourceType: SourceTypeURL, ID: "u", URL: "url", ProviderMetadata: ProviderMetadata{}}},
			want:     `{"source":{"type":"source","sourceType":"url","id":"u","url":"url","providerMetadata":{}}}`,
		},
		{
			name:     "document with filename",
			citation: Citation{Source: SourceInfo{SourceType: SourceTypeDocument, ID: "d", MediaType: "application/pdf", Title: "Report", Filename: "report.pdf"}},
			want:     `{"source":{"type":"source","sourceType":"document","id":"d","mediaType":"application/pdf","title":"Report","filename":"report.pdf"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.citation)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(data))
			assert.Regexp(t, `^\{"source":`, string(data))

			var decoded Citation
			require.NoError(t, json.Unmarshal(data, &decoded))
			roundTrip, err := json.Marshal(decoded)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(roundTrip))
		})
	}
}
