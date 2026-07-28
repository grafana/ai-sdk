package main

import (
	"context"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("text-stream", handleTextStream)
}

type textStreamModel struct{}

func (m *textStreamModel) SpecificationVersion() string               { return "v4" }
func (m *textStreamModel) Provider() string                           { return "test" }
func (m *textStreamModel) ModelID() string                            { return "test-text-stream" }
func (m *textStreamModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *textStreamModel) DoGenerate(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *textStreamModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"name":"Alice",`}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `"age":30,`}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `"active":true}`}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(8)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(12)}},
		}
	}()
	return &provider.StreamResult{Stream: ch}, nil
}

func handleTextStream(w http.ResponseWriter, r *http.Request) {
	model := &textStreamModel{}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("generate json")),
	)
	if err := aisdk.WriteTextStream(w, result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
