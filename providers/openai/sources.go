package openai

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

const (
	annotationURLCitation  = "url_citation"
	annotationFileCitation = "file_citation"
)

// convertAnnotation converts a text output annotation into a source content
// part. generateID provides deterministic source IDs. Titles go in the
// canonical Title field, the same field the streaming adapter and the V4
// source contract use; a document source takes its filename as its title, as
// the registered upstream conversion does.
func convertAnnotation(ann responses.ResponseOutputTextAnnotationUnion, generateID func() string, providerName string) (provider.GenerateContentPart, bool) {
	switch a := ann.AsAny().(type) {
	case responses.ResponseOutputTextAnnotationURLCitation:
		return provider.GenerateContentPart{
			Type:       provider.ContentSource,
			ID:         generateID(),
			SourceType: provider.SourceTypeURL,
			URL:        a.URL,
			Title:      a.Title,
		}, true

	case responses.ResponseOutputTextAnnotationFileCitation:
		return provider.GenerateContentPart{
			Type:             provider.ContentSource,
			ID:               generateID(),
			SourceType:       provider.SourceTypeDocument,
			MediaType:        "text/plain",
			Title:            a.Filename,
			Filename:         a.Filename,
			ProviderMetadata: sourceMeta(providerName, "file_citation", a.FileID, a.Index),
		}, true

	case responses.ResponseOutputTextAnnotationContainerFileCitation:
		return provider.GenerateContentPart{
			Type:             provider.ContentSource,
			ID:               generateID(),
			SourceType:       provider.SourceTypeDocument,
			MediaType:        "text/plain",
			Title:            a.Filename,
			Filename:         a.Filename,
			ProviderMetadata: containerSourceMeta(providerName, a.FileID, a.ContainerID),
		}, true

	case responses.ResponseOutputTextAnnotationFilePath:
		return provider.GenerateContentPart{
			Type:             provider.ContentSource,
			ID:               generateID(),
			SourceType:       provider.SourceTypeDocument,
			MediaType:        "application/octet-stream",
			Title:            a.FileID,
			Filename:         a.FileID,
			ProviderMetadata: sourceMeta(providerName, "file_path", a.FileID, a.Index),
		}, true
	}
	return provider.GenerateContentPart{}, false
}

func webSearchSources(action responses.ResponseFunctionWebSearchActionUnion, generateID func() string) []provider.GenerateContentPart {
	var parts []provider.GenerateContentPart
	if action.Type == "search" {
		for _, source := range action.Sources {
			if source.Type == "url" {
				parts = append(parts, provider.GenerateContentPart{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: generateID(), URL: source.URL})
			}
		}
	}
	return parts
}

func fileSearchSources(results []responses.ResponseFileSearchToolCallResult, retrievedIDs map[string]bool, generateID func() string, providerName string) []provider.GenerateContentPart {
	var parts []provider.GenerateContentPart
	for _, result := range results {
		if retrievedIDs[result.FileID] {
			continue
		}
		retrievedIDs[result.FileID] = true
		metadata, _ := json.Marshal(map[string]string{"type": "file_search", "fileId": result.FileID})
		parts = append(parts, provider.GenerateContentPart{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: generateID(), MediaType: "text/plain", Title: result.Filename, Filename: result.Filename, ProviderMetadata: provider.ProviderMetadata{providerName: metadata}})
	}
	return parts
}

func convertCitationAnnotations(annotations []responses.ResponseOutputTextAnnotationUnion, providerName string) []provider.Citation {
	var citations []provider.Citation
	for _, annotation := range annotations {
		var id string
		var start, end *int
		switch value := annotation.AsAny().(type) {
		case responses.ResponseOutputTextAnnotationURLCitation:
			id = value.URL
			start, end = new(int(value.StartIndex)), new(int(value.EndIndex))
		case responses.ResponseOutputTextAnnotationFileCitation:
			id = value.FileID
		case responses.ResponseOutputTextAnnotationContainerFileCitation:
			id = value.FileID
			start, end = new(int(value.StartIndex)), new(int(value.EndIndex))
		default:
			continue
		}
		part, _ := convertAnnotation(annotation, func() string { return id }, providerName)
		citations = append(citations, provider.Citation{Source: sourceInfo(part), StartIndex: start, EndIndex: end})
	}
	return citations
}

func sourceInfo(part provider.GenerateContentPart) provider.SourceInfo {
	return provider.SourceInfo{SourceType: part.SourceType, ID: part.ID, URL: part.URL, Title: part.Title, MediaType: part.MediaType, Filename: part.Filename, ProviderMetadata: part.ProviderMetadata}
}

func fallbackSourceSuppressed(annotationType string, hasWebSources, hasFileResults bool) bool {
	return (annotationType == annotationURLCitation && hasWebSources) || (annotationType == annotationFileCitation && hasFileResults)
}

func sourceMeta(providerName, citationType, fileID string, index int64) provider.ProviderMetadata {
	b, _ := json.Marshal(map[string]any{
		"type":   citationType,
		"fileId": fileID,
		"index":  index,
	})
	return provider.ProviderMetadata{providerName: b}
}

func containerSourceMeta(providerName, fileID, containerID string) provider.ProviderMetadata {
	b, _ := json.Marshal(map[string]any{
		"type":        "container_file_citation",
		"fileId":      fileID,
		"containerId": containerID,
	})
	return provider.ProviderMetadata{providerName: b}
}
