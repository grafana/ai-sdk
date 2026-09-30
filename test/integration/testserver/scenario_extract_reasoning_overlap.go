package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("extract-reasoning-overlap", handleExtractReasoningOverlap)
}

type overlappingReasoningModel struct{ simpleTextModel }

func (*overlappingReasoningModel) ModelID() string { return "extract-reasoning-overlap" }
func (*overlappingReasoningModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
		{Type: provider.PartTextDelta, ID: "a", Delta: "<think>A"},
		{Type: provider.PartTextDelta, ID: "b", Delta: "<think>B"},
		{Type: provider.PartTextDelta, ID: "a", Delta: "1</think>Alpha."},
		{Type: provider.PartTextDelta, ID: "b", Delta: "2</think>Beta."},
		{Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartTextEnd, ID: "b"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleExtractReasoningOverlap(w http.ResponseWriter, r *http.Request) {
	model := middleware.WrapLanguageModel(&overlappingReasoningModel{}, middleware.ExtractReasoning(middleware.ExtractReasoningOptions{TagName: "think"}))
	result := aisdk.StreamText(r.Context(), model, aisdk.WithModelMessages(provider.UserText("Explain the answer")))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
