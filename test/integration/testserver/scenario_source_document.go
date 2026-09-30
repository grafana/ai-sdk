package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("source-document", handleSourceDocument)
}

type sourceDocumentModel struct{}

func (m *sourceDocumentModel) SpecificationVersion() string               { return "v4" }
func (m *sourceDocumentModel) Provider() string                           { return "test" }
func (m *sourceDocumentModel) ModelID() string                            { return "test-source-document" }
func (m *sourceDocumentModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *sourceDocumentModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *sourceDocumentModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := make(chan provider.StreamPart, 4)
	parts <- provider.StreamPart{
		Type: provider.PartSource,
		Source: &provider.SourceInfo{
			SourceType:       provider.SourceTypeDocument,
			ID:               "source-1",
			MediaType:        "application/pdf",
			Title:            "Financial Report",
			Filename:         "financial-report.pdf",
			ProviderMetadata: provider.ProviderMetadata{"citation": json.RawMessage(`{"startPageNumber":1}`)},
		},
	}
	parts <- provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "source-2", URL: "https://example.com", Title: "URL", ProviderMetadata: provider.ProviderMetadata{"citation": json.RawMessage(`{"index":0}`)}}}
	parts <- provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "source-3", MediaType: "text/plain", Title: ""}}
	parts <- provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
	}
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func handleSourceDocument(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &sourceDocumentModel{},
		aisdk.WithModelMessages(provider.UserText("summarize")),
	)
	stream := result.ToUIMessageStream(aisdk.WithUIMessageStreamSources(true))
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
