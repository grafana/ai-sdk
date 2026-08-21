package anthropic

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync/atomic"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

const (
	defaultIDSize   = 7
	urlSafeAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-_"
)

var fallbackCounter atomic.Int64

func defaultGenerateID() string {
	bytes := make([]byte, defaultIDSize)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "cit-" + strconv.FormatInt(fallbackCounter.Add(1), 10)
	}
	id := make([]byte, defaultIDSize)
	for i, b := range bytes {
		id[i] = urlSafeAlphabet[b%byte(len(urlSafeAlphabet))]
	}
	return string(id)
}

type citationDocument struct {
	title     string
	filename  string
	mediaType string
}

func extractCitationDocuments(prompt []provider.Message) []citationDocument {
	var docs []citationDocument
	for _, msg := range prompt {
		if msg.Role != provider.RoleUser {
			continue
		}
		for _, fp := range msg.Content {
			if fp.Type != provider.ContentPartTypeFile {
				continue
			}
			if fp.MediaType != "application/pdf" && fp.MediaType != "text/plain" {
				continue
			}
			if !hasCitationsEnabled(fp.ProviderOptions) {
				continue
			}
			if fp.Data == nil || !hasFileData(*fp.Data) {
				continue
			}
			filename := ""
			if fp.FilePartFilename != nil {
				filename = *fp.FilePartFilename
			}
			title := filename
			if title == "" {
				title = "Untitled Document"
			}
			docs = append(docs, citationDocument{
				title:     title,
				filename:  filename,
				mediaType: fp.MediaType,
			})
		}
	}
	return docs
}

func hasFileData(data provider.DataContent) bool {
	return data.Base64 != "" || len(data.Bytes) > 0 || data.URL != ""
}

func hasCitationsEnabled(opts provider.ProviderOptions) bool {
	raw := extractRawJSON(opts)
	if raw == nil {
		return false
	}
	var data struct {
		Citations struct {
			Enabled bool `json:"enabled"`
		} `json:"citations"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return false
	}
	return data.Citations.Enabled
}

func marshalWebSearchCitationMetadata(citations []json.RawMessage) (provider.ProviderMetadata, error) {
	if len(citations) == 0 {
		return nil, nil
	}
	metadata, err := json.Marshal(map[string]any{"citations": citations})
	if err != nil {
		return nil, fmt.Errorf("marshaling web search citation metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": metadata}, nil
}

func extractCitations(opts provider.ProviderOptions) []anthropic.BetaTextCitationParamUnion {
	raw := extractRawJSON(opts)
	if raw == nil {
		return nil
	}
	var metadata struct {
		Citations []json.RawMessage `json:"citations"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return nil
	}
	citations := make([]anthropic.BetaTextCitationParamUnion, 0, len(metadata.Citations))
	for _, rawCitation := range metadata.Citations {
		var citation anthropic.BetaTextCitationParamUnion
		if json.Unmarshal(rawCitation, &citation) != nil || !hasCitationVariant(citation) {
			continue
		}
		citations = append(citations, citation)
	}
	return citations
}

func hasCitationVariant(citation anthropic.BetaTextCitationParamUnion) bool {
	return citation.OfCharLocation != nil ||
		citation.OfPageLocation != nil ||
		citation.OfContentBlockLocation != nil ||
		citation.OfWebSearchResultLocation != nil ||
		citation.OfSearchResultLocation != nil
}

func createCitationSource(citation any, docs []citationDocument, generateID func() string) (*provider.SourceInfo, error) {
	switch c := citation.(type) {
	case anthropic.BetaCitationsWebSearchResultLocation:
		metaJSON, err := json.Marshal(map[string]any{
			"citedText":      c.CitedText,
			"encryptedIndex": c.EncryptedIndex,
		})
		if err != nil {
			return nil, fmt.Errorf("marshaling web search citation metadata: %w", err)
		}
		return &provider.SourceInfo{
			SourceType: provider.SourceTypeURL,
			ID:         generateID(),
			URL:        c.URL,
			Title:      c.Title,
			ProviderMetadata: provider.ProviderMetadata{
				"anthropic": metaJSON,
			},
		}, nil

	case anthropic.BetaCitationPageLocation:
		if int(c.DocumentIndex) >= len(docs) {
			return nil, nil
		}
		doc := docs[c.DocumentIndex]
		title := doc.title
		if c.DocumentTitle != "" {
			title = c.DocumentTitle
		}
		metaJSON, err := json.Marshal(map[string]any{
			"citedText":       c.CitedText,
			"startPageNumber": c.StartPageNumber,
			"endPageNumber":   c.EndPageNumber,
		})
		if err != nil {
			return nil, fmt.Errorf("marshaling page citation metadata: %w", err)
		}
		return &provider.SourceInfo{
			SourceType: provider.SourceTypeDocument,
			ID:         generateID(),
			MediaType:  doc.mediaType,
			Title:      title,
			Filename:   doc.filename,
			ProviderMetadata: provider.ProviderMetadata{
				"anthropic": metaJSON,
			},
		}, nil

	case anthropic.BetaCitationCharLocation:
		if int(c.DocumentIndex) >= len(docs) {
			return nil, nil
		}
		doc := docs[c.DocumentIndex]
		title := doc.title
		if c.DocumentTitle != "" {
			title = c.DocumentTitle
		}
		metaJSON, err := json.Marshal(map[string]any{
			"citedText":      c.CitedText,
			"startCharIndex": c.StartCharIndex,
			"endCharIndex":   c.EndCharIndex,
		})
		if err != nil {
			return nil, fmt.Errorf("marshaling char citation metadata: %w", err)
		}
		return &provider.SourceInfo{
			SourceType: provider.SourceTypeDocument,
			ID:         generateID(),
			MediaType:  doc.mediaType,
			Title:      title,
			Filename:   doc.filename,
			ProviderMetadata: provider.ProviderMetadata{
				"anthropic": metaJSON,
			},
		}, nil

	default:
		return nil, nil
	}
}
