package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("reasoning", handleReasoning)
}

type reasoningModel struct{ simpleTextModel }

func (m *reasoningModel) ModelID() string { return "test-reasoning" }
func (m *reasoningModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-1"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-1", Delta: "First thought."}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-1"}
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-2"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-2", Delta: "Second thought."}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-2"}
		ch <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{},
		}
	}()
	return &provider.StreamResult{Stream: ch}, nil
}

func handleReasoning(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &reasoningModel{},
		aisdk.WithModelMessages(provider.UserText("think")),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
