package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("finish-reason-length", handleFinishReasonLength)
}

type finishReasonLengthModel struct{ simpleTextModel }

func (m *finishReasonLengthModel) ModelID() string { return "test-finish-reason-length" }
func (m *finishReasonLengthModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 4)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "Partial response"}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
	stream <- provider.StreamPart{
		Type: provider.PartFinish,
		FinishReason: &provider.FinishReason{
			Unified: provider.FinishReasonLength,
			Raw:     "model_context_window_exceeded",
		},
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleFinishReasonLength(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &finishReasonLengthModel{},
		aisdk.WithModelMessages(provider.UserText("return a partial response")),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
