package main

import (
	"context"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("simple-text", handleSimpleText)
}

type simpleTextModel struct{}

func (m *simpleTextModel) SpecificationVersion() string               { return "v4" }
func (m *simpleTextModel) Provider() string                           { return "test" }
func (m *simpleTextModel) ModelID() string                            { return "test-simple" }
func (m *simpleTextModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *simpleTextModel) DoGenerate(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *simpleTextModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "Hello, "}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "world!"}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}},
		}
	}()
	return &provider.StreamResult{Stream: ch}, nil
}

func handleSimpleText(w http.ResponseWriter, r *http.Request) {
	model := &simpleTextModel{}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("hello")),
	)
	stream := result.ToUIMessageStream()
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func intPtr(v int) *int { return &v }
