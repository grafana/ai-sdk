package openai

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// convertAnnotations converts text output annotations into source content
// parts. generateID provides deterministic source IDs. Titles go in the
// canonical Title field, the same field the streaming adapter and the V4
// source contract use; a document source takes its filename as its title, as
// the registered upstream conversion does.
func convertAnnotations(annotations []responses.ResponseOutputTextAnnotationUnion, generateID func() string, providerName string) []provider.GenerateContentPart {
	var parts []provider.GenerateContentPart
	for _, ann := range annotations {
		switch a := ann.AsAny().(type) {
		case responses.ResponseOutputTextAnnotationURLCitation:
			parts = append(parts, provider.GenerateContentPart{
				Type:       provider.ContentSource,
				ID:         generateID(),
				SourceType: provider.SourceTypeURL,
				URL:        a.URL,
				Title:      a.Title,
			})

		case responses.ResponseOutputTextAnnotationFileCitation:
			parts = append(parts, provider.GenerateContentPart{
				Type:             provider.ContentSource,
				ID:               generateID(),
				SourceType:       provider.SourceTypeDocument,
				MediaType:        "text/plain",
				Title:            a.Filename,
				Filename:         a.Filename,
				ProviderMetadata: sourceMeta(providerName, "file_citation", a.FileID, a.Index),
			})

		case responses.ResponseOutputTextAnnotationContainerFileCitation:
			parts = append(parts, provider.GenerateContentPart{
				Type:             provider.ContentSource,
				ID:               generateID(),
				SourceType:       provider.SourceTypeDocument,
				MediaType:        "text/plain",
				Title:            a.Filename,
				Filename:         a.Filename,
				ProviderMetadata: containerSourceMeta(providerName, a.FileID, a.ContainerID),
			})

		case responses.ResponseOutputTextAnnotationFilePath:
			parts = append(parts, provider.GenerateContentPart{
				Type:             provider.ContentSource,
				ID:               generateID(),
				SourceType:       provider.SourceTypeDocument,
				MediaType:        "application/octet-stream",
				Title:            a.FileID,
				Filename:         a.FileID,
				ProviderMetadata: sourceMeta(providerName, "file_path", a.FileID, a.Index),
			})
		}
	}
	return parts
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
