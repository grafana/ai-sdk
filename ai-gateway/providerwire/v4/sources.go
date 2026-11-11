package v4

import (
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type urlSource struct {
	Type             provider.GenerateContentType `json:"type"`
	SourceType       provider.SourceType          `json:"sourceType"`
	ID               string                       `json:"id"`
	URL              string                       `json:"url"`
	Title            string                       `json:"title,omitempty"`
	ProviderMetadata provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

type documentSource struct {
	Type             provider.GenerateContentType `json:"type"`
	SourceType       provider.SourceType          `json:"sourceType"`
	ID               string                       `json:"id"`
	MediaType        string                       `json:"mediaType"`
	Title            string                       `json:"title"`
	Filename         string                       `json:"filename,omitempty"`
	ProviderMetadata provider.ProviderMetadata    `json:"providerMetadata,omitzero"`
}

func unarySource(part provider.GenerateContentPart) provider.SourceInfo {
	title := part.Title
	if title == "" {
		title = part.Text
	}
	return provider.SourceInfo{SourceType: part.SourceType, ID: part.ID, URL: part.URL, Title: title, MediaType: part.MediaType, Filename: part.Filename, ProviderMetadata: part.ProviderMetadata}
}

func validSourceFields(source provider.SourceInfo) bool {
	if source.SourceType != provider.SourceTypeURL && source.SourceType != provider.SourceTypeDocument || !validMetadata(source.ProviderMetadata) {
		return false
	}
	for _, value := range []string{source.ID, source.URL, source.Title, source.MediaType, source.Filename} {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func mapSource(source provider.SourceInfo) (any, error) {
	if !validSourceFields(source) {
		return nil, errInvalidUnarySuccess
	}
	if source.SourceType == provider.SourceTypeURL {
		return urlSource{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title, ProviderMetadata: source.ProviderMetadata}, nil
	}
	return documentSource{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, MediaType: source.MediaType, Title: source.Title, Filename: source.Filename, ProviderMetadata: source.ProviderMetadata}, nil
}
