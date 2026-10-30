package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("finish-reason-length", handleFinishReasonLength)
	registerScenario("finish-reason-interrupted", handleFinishReasonInterrupted)
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

type finishReasonInterruptedModel struct{ simpleTextModel }

func (m *finishReasonInterruptedModel) ModelID() string { return "test-finish-reason-interrupted" }
func (m *finishReasonInterruptedModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "Let me check"}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "lookup", Input: `{"city":"Paris"}`}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonInterrupted}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleFinishReasonInterrupted(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &finishReasonInterruptedModel{},
		aisdk.WithModelMessages(provider.UserText("look up the weather")),
		aisdk.WithTools(aisdk.ToolSet{"lookup": {}}),
		aisdk.WithStopWhen(aisdk.StepCountIs(3)),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
