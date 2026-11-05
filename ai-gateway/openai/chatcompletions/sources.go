package chatcompletions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"unicode/utf8"
)

type annotation struct {
	Type        string      `json:"type"`
	URLCitation urlCitation `json:"url_citation"`
}
type urlCitation struct {
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
	Title      string `json:"title"`
	URL        string `json:"url"`
}

// Only URL citations with native spans have a standard Chat representation.
// Other citations and retrieved sources remain available without invented spans.
func mapCitations(citations []provider.Citation) []annotation {
	var result []annotation
	for _, c := range citations {
		if c.Source.SourceType == provider.SourceTypeURL && c.StartIndex != nil && c.EndIndex != nil {
			result = append(result, annotation{Type: "url_citation", URLCitation: urlCitation{StartIndex: *c.StartIndex, EndIndex: *c.EndIndex, Title: c.Source.Title, URL: c.Source.URL}})
		}
	}
	return result
}
func sourceDiagnostics(d *diagnostics, source *provider.SourceInfo, citations []provider.Citation) *diagnostics {
	if source == nil && len(citations) == 0 {
		return d
	}
	if d == nil {
		d = &diagnostics{}
	}
	if source != nil {
		d.Sources = append(d.Sources, *source)
	}
	d.Citations = append(d.Citations, citations...)
	return d
}

func validSource(s provider.SourceInfo) bool {
	if s.SourceType != provider.SourceTypeURL && s.SourceType != provider.SourceTypeDocument {
		return false
	}
	if s.SourceType == provider.SourceTypeURL && s.URL == "" {
		return false
	}
	if s.SourceType == provider.SourceTypeDocument && (s.Title == "" || s.MediaType == "") {
		return false
	}
	for _, v := range []string{s.ID, s.URL, s.Title, s.MediaType, s.Filename} {
		if !utf8.ValidString(v) {
			return false
		}
	}
	for k, v := range s.ProviderMetadata {
		if !utf8.ValidString(k) || !utf8.Valid(v) || !json.Valid(v) {
			return false
		}
	}
	return true
}
func validCitations(citations []provider.Citation) bool {
	for _, c := range citations {
		if !validSource(c.Source) || (c.CitedText != nil && !utf8.ValidString(*c.CitedText)) {
			return false
		}
		if c.StartIndex != nil && *c.StartIndex < 0 || c.EndIndex != nil && *c.EndIndex < 0 {
			return false
		}
		if c.StartIndex != nil && c.EndIndex != nil && *c.EndIndex < *c.StartIndex {
			return false
		}
	}
	return true
}
