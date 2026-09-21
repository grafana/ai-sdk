package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_DocumentNameSanitation(t *testing.T) {
	for _, tc := range []struct{ filename, want string }{
		{"John's report.txt", "Johns report"},
		{"invoice #123.txt", "invoice 123"},
		{"a&b.txt", "ab"},
		{"résumé.txt", "rsum"},
		{"분기보고서.txt", "document-1"},
		{"Report -  Final.txt", "Report - Final"},
		{"a\tb.txt", "a b"},
		{"a\u00a0\u2028\ufeffb.txt", "a b"},
		{"a\u0085b.txt", "ab"},
		{strings.Repeat("a", 201) + ".txt", strings.Repeat("a", 200)},
		{strings.Repeat("a", 199) + " b.txt", strings.Repeat("a", 199)},
		{".txt", "document-1"},
		{"", "document-1"},
		{" \t.txt", "document-1"},
		{"report (final) [v2]_draft.txt", "report (final) [v2]draft"},
		{"archive.tar.gz", "archive"},
		{"valid-name.pdf", "valid-name"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			for _, site := range []string{"input bytes", "input text", "tool result"} {
				t.Run(site, func(t *testing.T) {
					data := provider.DataContent{Base64: "SGVsbG8="}
					if site == "input text" {
						data = provider.DataContent{Text: "Hello"}
					}
					citations := provider.BuildProviderOptions(FilePartOptions{Citations: &FilePartCitations{Enabled: true}})
					part := provider.FilePart("text/plain", data)
					part.Filename = tc.filename
					part.ProviderOptions = citations
					opts := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}}
					if site == "tool result" {
						opts = toolResultFileCallOptions(provider.ToolResultContentValue{Type: provider.ToolContentFile, Data: &data, MediaType: "text/plain", Filename: tc.filename, ProviderOptions: citations})
					}
					before, err := json.Marshal(opts)
					require.NoError(t, err)
					req, warnings, _ := mustBuildRequest(t, testAnthropicModel, opts)
					document := req.Messages[0].Content[0].Document
					if site == "tool result" {
						document = req.Messages[0].Content[0].ToolResult.Content[0].Document
					}
					require.NotNil(t, document)
					assert.Equal(t, tc.want, document.Name)
					assert.Equal(t, "txt", document.Format)
					assert.Equal(t, "SGVsbG8=", document.Source.Bytes)
					require.NotNil(t, document.Citations)
					assert.True(t, document.Citations.Enabled)
					assert.Empty(t, warnings)
					after, err := json.Marshal(opts)
					require.NoError(t, err)
					assert.JSONEq(t, string(before), string(after))
				})
			}
		})
	}
}

func TestBuildDocumentBlock_FallbackCounter(t *testing.T) {
	counter := 0
	for _, tc := range []struct{ name, want string }{{".pdf", "document-1"}, {"named.pdf", "named"}, {"分.pdf", "document-2"}, {"", "document-3"}} {
		document, err := buildDocumentBlock("application/pdf", tc.name, "AA==", nil, &counter)
		require.NoError(t, err)
		assert.Equal(t, tc.want, document.Name)
	}
	assert.Equal(t, 3, counter)
}
