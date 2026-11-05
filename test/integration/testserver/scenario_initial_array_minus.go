package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("initial-array-minus", handleInitialArrayMinus)
}

type initialArrayMinusModel struct{ simpleTextModel }

func (*initialArrayMinusModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 6)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
	for _, delta := range []string{`{"series":"temp","values":[-`, `3,`, `4]}`} {
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: delta}
	}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleInitialArrayMinus(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &initialArrayMinusModel{},
		aisdk.WithModelMessages(provider.UserText("return a temperature series")),
		aisdk.WithOutput(output.JSON()),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
