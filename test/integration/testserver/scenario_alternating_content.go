package main

import (
	"context"
	"net/http"
	"strconv"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("alternating-content", handleAlternatingContent)
}

type alternatingContentModel struct{ simpleTextModel }

func (m *alternatingContentModel) ModelID() string { return "test-alternating-content" }
func (m *alternatingContentModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-0"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "think"}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-0"}
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "txt-0"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "txt-0", Delta: "answer"}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "txt-0"}
		ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "reasoning-0"}
		ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "again"}
		ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "reasoning-0"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	}()
	return &provider.StreamResult{Stream: ch}, nil
}

func handleAlternatingContent(w http.ResponseWriter, r *http.Request) {
	generated := 0
	result := aisdk.StreamText(r.Context(), &alternatingContentModel{},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithGenerateID(func() string {
			id := "generated-" + strconv.Itoa(generated)
			generated++
			return id
		}),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
