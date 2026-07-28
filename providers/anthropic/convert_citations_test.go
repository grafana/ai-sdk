package anthropic

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func citationOpts(enabled bool) provider.ProviderOptions {
	raw, _ := json.Marshal(map[string]any{
		"citations": map[string]any{"enabled": enabled},
	})
	return provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: raw}}
}

func TestExtractCitationDocuments(t *testing.T) {
	t.Run("pdf with citations enabled", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "report.pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		require.Len(t, docs, 1)
		assert.Equal(t, "report.pdf", docs[0].title)
		assert.Equal(t, "report.pdf", docs[0].filename)
		assert.Equal(t, "application/pdf", docs[0].mediaType)
	})

	t.Run("text with citations enabled", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "text/plain",
					Filename:        "notes.txt",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		require.Len(t, docs, 1)
		assert.Equal(t, "notes.txt", docs[0].title)
		assert.Equal(t, "text/plain", docs[0].mediaType)
	})

	t.Run("file without citations excluded", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType: "application/pdf",
					Filename:  "report.pdf",
					Data:      &provider.DataContent{Base64: "abc"},
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		assert.Empty(t, docs)
	})

	t.Run("citations explicitly disabled excluded", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "report.pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(false),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		assert.Empty(t, docs)
	})

	t.Run("non-citation media type excluded", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "image/png",
					Filename:        "photo.png",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		assert.Empty(t, docs)
	})

	t.Run("missing filename defaults to Untitled Document", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		require.Len(t, docs, 1)
		assert.Equal(t, "Untitled Document", docs[0].title)
		assert.Equal(t, "", docs[0].filename)
	})

	t.Run("multiple documents preserve order", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "first.pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "text/plain",
					Filename:        "second.txt",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "third.pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		require.Len(t, docs, 3)
		assert.Equal(t, "first.pdf", docs[0].filename)
		assert.Equal(t, "second.txt", docs[1].filename)
		assert.Equal(t, "third.pdf", docs[2].filename)
	})

	t.Run("non-user messages ignored", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleSystem, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "you are helpful"}}},
			provider.Message{Role: provider.RoleAssistant, Content: []provider.ContentPart{
				provider.TextPart("hi"),
			}},
		}
		docs := extractCitationDocuments(prompt)
		assert.Empty(t, docs)
	})

	t.Run("file without data excluded to match sent documents", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "empty.pdf",
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		assert.Empty(t, docs, "file without Data should not be tracked since it won't be sent to Anthropic")
	})

	t.Run("only files with data are tracked", func(t *testing.T) {
		prompt := []provider.Message{
			provider.Message{Role: provider.RoleUser, Content: []provider.ContentPart{
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "empty.pdf",
					ProviderOptions: citationOpts(true),
				},
				provider.ContentPart{Type: provider.ContentPartTypeFile,
					MediaType:       "application/pdf",
					Filename:        "valid.pdf",
					Data:            &provider.DataContent{Base64: "abc"},
					ProviderOptions: citationOpts(true),
				},
			}},
		}
		docs := extractCitationDocuments(prompt)
		require.Len(t, docs, 1)
		assert.Equal(t, "valid.pdf", docs[0].filename)
	})
}

func seqIDGenerator() func() string {
	n := 0
	return func() string {
		n++
		return "id-" + strconv.Itoa(n)
	}
}

func TestCreateCitationSource(t *testing.T) {
	docs := []citationDocument{
		{title: "Report", filename: "report.pdf", mediaType: "application/pdf"},
		{title: "Notes", filename: "notes.txt", mediaType: "text/plain"},
	}

	t.Run("web search result location", func(t *testing.T) {
		citation := anthropic.BetaCitationsWebSearchResultLocation{
			URL:            "https://example.com",
			Title:          "Example Page",
			CitedText:      "some text",
			EncryptedIndex: "enc123",
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		require.NoError(t, err)
		require.NotNil(t, src)
		assert.Equal(t, provider.SourceTypeURL, src.SourceType)
		assert.Equal(t, "https://example.com", src.URL)
		assert.Equal(t, "Example Page", src.Title)
		assert.NotEmpty(t, src.ID)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(src.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, "some text", meta["citedText"])
		assert.Equal(t, "enc123", meta["encryptedIndex"])
	})

	t.Run("page location with valid document index", func(t *testing.T) {
		citation := anthropic.BetaCitationPageLocation{
			DocumentIndex:   0,
			CitedText:       "page text",
			StartPageNumber: 1,
			EndPageNumber:   3,
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		require.NoError(t, err)
		require.NotNil(t, src)
		assert.Equal(t, provider.SourceTypeDocument, src.SourceType)
		assert.Equal(t, "application/pdf", src.MediaType)
		assert.Equal(t, "Report", src.Title)
		assert.Equal(t, "report.pdf", src.Filename)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(src.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, "page text", meta["citedText"])
		assert.Equal(t, float64(1), meta["startPageNumber"])
		assert.Equal(t, float64(3), meta["endPageNumber"])
	})

	t.Run("char location with valid document index", func(t *testing.T) {
		citation := anthropic.BetaCitationCharLocation{
			DocumentIndex:  1,
			CitedText:      "char text",
			StartCharIndex: 10,
			EndCharIndex:   50,
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		require.NoError(t, err)
		require.NotNil(t, src)
		assert.Equal(t, provider.SourceTypeDocument, src.SourceType)
		assert.Equal(t, "text/plain", src.MediaType)
		assert.Equal(t, "Notes", src.Title)
		assert.Equal(t, "notes.txt", src.Filename)

		var meta map[string]any
		require.NoError(t, json.Unmarshal(src.ProviderMetadata["anthropic"], &meta))
		assert.Equal(t, "char text", meta["citedText"])
		assert.Equal(t, float64(10), meta["startCharIndex"])
		assert.Equal(t, float64(50), meta["endCharIndex"])
	})

	t.Run("document title override from citation", func(t *testing.T) {
		citation := anthropic.BetaCitationPageLocation{
			DocumentIndex: 0,
			DocumentTitle: "Override Title",
			CitedText:     "text",
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		require.NoError(t, err)
		require.NotNil(t, src)
		assert.Equal(t, "Override Title", src.Title)
	})

	t.Run("out-of-range document index returns false", func(t *testing.T) {
		citation := anthropic.BetaCitationPageLocation{
			DocumentIndex: 99,
			CitedText:     "text",
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		assert.NoError(t, err)
		assert.Nil(t, src)
	})

	t.Run("char location out-of-range returns false", func(t *testing.T) {
		citation := anthropic.BetaCitationCharLocation{
			DocumentIndex: 5,
			CitedText:     "text",
		}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		assert.NoError(t, err)
		assert.Nil(t, src)
	})

	t.Run("unknown citation type returns false", func(t *testing.T) {
		citation := anthropic.BetaCitationContentBlockLocation{}
		src, err := createCitationSource(citation, docs, seqIDGenerator())
		assert.NoError(t, err)
		assert.Nil(t, src)
	})

	t.Run("each call gets unique ID", func(t *testing.T) {
		gen := seqIDGenerator()
		c1 := anthropic.BetaCitationsWebSearchResultLocation{URL: "https://a.com", Title: "A"}
		c2 := anthropic.BetaCitationsWebSearchResultLocation{URL: "https://b.com", Title: "B"}
		s1, err := createCitationSource(c1, docs, gen)
		require.NoError(t, err)
		s2, err := createCitationSource(c2, docs, gen)
		require.NoError(t, err)
		assert.NotEqual(t, s1.ID, s2.ID)
	})
}
