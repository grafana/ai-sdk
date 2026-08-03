package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("text-metadata-only-delta", handleTextMetadataOnlyDelta)
}

type textMetadataOnlyDeltaModel struct{}

func (*textMetadataOnlyDeltaModel) SpecificationVersion() string { return "v4" }
func (*textMetadataOnlyDeltaModel) Provider() string             { return "test" }
func (*textMetadataOnlyDeltaModel) ModelID() string              { return "test-text-metadata-only-delta" }
func (*textMetadataOnlyDeltaModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (*textMetadataOnlyDeltaModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*textMetadataOnlyDeltaModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	metadata := provider.ProviderMetadata{"test": json.RawMessage(`{"signature":"test-signature"}`)}
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: `{"value":"ok"}`}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", ProviderMetadata: metadata}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleTextMetadataOnlyDelta(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &textMetadataOnlyDeltaModel{},
		aisdk.WithModelMessages(provider.UserText("return JSON")),
		aisdk.WithOutput(output.JSON()),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
