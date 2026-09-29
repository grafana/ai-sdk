package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("simulated-stream", handleSimulatedStream)
}

type simulatedStreamModel struct{}

func (*simulatedStreamModel) SpecificationVersion() string               { return "v4" }
func (*simulatedStreamModel) Provider() string                           { return "test" }
func (*simulatedStreamModel) ModelID() string                            { return "test-simulated-stream" }
func (*simulatedStreamModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*simulatedStreamModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{
				Type: provider.ContentText, Text: "Generated answer",
				ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"signature":"text-signature"}`)},
			},
			{
				Type: provider.ContentSource, SourceType: provider.SourceTypeDocument,
				ID: "document-1", Title: "Generated Report", MediaType: "application/pdf",
				Filename:         "generated.pdf",
				ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"citation":"page-1"}`)},
			},
			{
				Type: provider.ContentCustom, Kind: "test.custom",
				ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"value":"preserved"}`)},
			},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}, nil
}
func (*simulatedStreamModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	return nil, errors.New("simulated stream must use DoGenerate")
}

func handleSimulatedStream(w http.ResponseWriter, r *http.Request) {
	model := middleware.WrapLanguageModel(&simulatedStreamModel{}, middleware.SimulateStreaming())
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("summarize the report")),
	)
	stream := result.ToUIMessageStream(aisdk.WithUIMessageStreamSources(true))
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
