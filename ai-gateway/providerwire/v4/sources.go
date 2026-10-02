package v4

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

type urlSource struct {
	Type             provider.GenerateContentType `json:"type"`
	SourceType       provider.SourceType          `json:"sourceType"`
	ID               string                       `json:"id"`
	URL              string                       `json:"url"`
	Title            string                       `json:"title,omitempty"`
	ProviderMetadata *provider.ProviderMetadata   `json:"providerMetadata,omitempty"`
}

type documentSource struct {
	Type             provider.GenerateContentType `json:"type"`
	SourceType       provider.SourceType          `json:"sourceType"`
	ID               string                       `json:"id"`
	MediaType        string                       `json:"mediaType"`
	Title            string                       `json:"title"`
	Filename         string                       `json:"filename,omitempty"`
	ProviderMetadata *provider.ProviderMetadata   `json:"providerMetadata,omitempty"`
}

func unarySource(part provider.GenerateContentPart) provider.SourceInfo {
	title := part.Title
	if title == "" {
		title = part.Text
	}
	return provider.SourceInfo{SourceType: part.SourceType, ID: part.ID, URL: part.URL, Title: title, MediaType: part.MediaType, Filename: part.Filename, ProviderMetadata: part.ProviderMetadata}
}

func sourcePreflight(source provider.SourceInfo, limit int64) bool {
	for _, value := range []string{source.ID, source.URL, source.Title, source.MediaType, source.Filename} {
		if int64(len(value)) > limit {
			return false
		}
		limit -= int64(len(value))
	}
	return metadataFits(source.ProviderMetadata, &limit)
}

func mapSource(source provider.SourceInfo, limit int64) (any, error) {
	if !sourcePreflight(source, limit) {
		return nil, errInvalidUnarySuccess
	}
	for _, value := range []string{source.ID, source.URL, source.Title, source.MediaType, source.Filename} {
		if !utf8.ValidString(value) {
			return nil, errInvalidUnarySuccess
		}
	}
	if source.SourceType != provider.SourceTypeURL && source.SourceType != provider.SourceTypeDocument {
		return nil, errInvalidUnarySuccess
	}
	metadata, err := mapMetadata(source.ProviderMetadata)
	if err != nil {
		return nil, err
	}
	var mapped any
	if source.SourceType == provider.SourceTypeURL {
		mapped = urlSource{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title, ProviderMetadata: metadata}
	} else {
		mapped = documentSource{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, MediaType: source.MediaType, Title: source.Title, Filename: source.Filename, ProviderMetadata: metadata}
	}
	encoded, err := json.Marshal(mapped)
	if err != nil || int64(len(encoded)) > limit {
		return nil, errInvalidUnarySuccess
	}
	return mapped, nil
}
