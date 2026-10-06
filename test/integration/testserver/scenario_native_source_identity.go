package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("native-source-identity", handleNativeSourceIdentity)
}

type nativeSourceIdentityModel struct{ sourceDocumentModel }

func (*nativeSourceIdentityModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	sources := []provider.SourceInfo{
		{SourceType: provider.SourceTypeURL, ID: "native-shared", URL: "https://example.com", Title: "Native URL"},
		{SourceType: provider.SourceTypeURL, ID: "native-shared", URL: "https://example.com/repeated"},
		{SourceType: provider.SourceTypeDocument, ID: "native-shared", MediaType: "application/octet-stream", Title: "file-native", Filename: "file-native"},
		{SourceType: provider.SourceTypeDocument, ID: "", MediaType: "text/plain", Title: ""},
	}
	parts := make(chan provider.StreamPart, len(sources)+3)
	parts <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "provider hook only"}}}
	parts <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "native-response", ModelID: "native model ☃", Timestamp: time.Date(2026, 9, 30, 12, 0, 0, 123000000, time.UTC)}
	for _, source := range sources {
		parts <- provider.StreamPart{Type: provider.PartSource, Source: &source}
	}
	parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func handleNativeSourceIdentity(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &nativeSourceIdentityModel{}, aisdk.WithModelMessages(provider.UserText("sources")))
	stream := result.ToUIMessageStream(aisdk.WithUIMessageStreamSources(true), aisdk.WithUIMessageStreamMessageMetadata(func(part aisdk.TextStreamPart) json.RawMessage {
		switch part := part.(type) {
		case aisdk.StreamFinishStep:
			if part.Response == nil {
				return nil
			}
			body, err := json.Marshal(map[string]any{"nativeResponse": map[string]string{"id": part.Response.ID, "modelId": part.Response.ModelID, "timestamp": part.Response.Timestamp.UTC().Format(time.RFC3339Nano)}})
			if err != nil {
				return nil
			}
			return body
		default:
			return nil
		}
	}))
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
