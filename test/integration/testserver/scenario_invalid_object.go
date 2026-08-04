package main

import (
	"context"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("invalid-object", handleInvalidObject)
}

type invalidObjectModel struct{}

func (*invalidObjectModel) SpecificationVersion() string               { return "v4" }
func (*invalidObjectModel) Provider() string                           { return "test" }
func (*invalidObjectModel) ModelID() string                            { return "test-invalid-object" }
func (*invalidObjectModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*invalidObjectModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*invalidObjectModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 6)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "invalid-object"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "invalid-object", Delta: `{"name":"Alice",`}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "invalid-object", Delta: `"age":"thirty",`}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "invalid-object", Delta: `"active":true}`}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "invalid-object"}
	stream <- provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        &provider.Usage{},
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleInvalidObject(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &invalidObjectModel{},
		aisdk.WithModelMessages(provider.UserText("generate invalid object")),
	)
	if err := aisdk.WriteTextStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
