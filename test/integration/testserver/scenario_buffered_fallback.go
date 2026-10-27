package main

import (
	"context"
	"net/http"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("buffered-model-fallback", func(w http.ResponseWriter, r *http.Request) {
		model, err := bufferedFallbackScenario()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		result := aisdk.StreamText(r.Context(), model, aisdk.WithModelMessages(provider.UserText("decide")), aisdk.WithMaxRetries(0))
		if err := aisdk.WriteUIMessageStream(w, result); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
}

type bufferedFallbackModel struct {
	simpleTextModel
	failed bool
}

func (m *bufferedFallbackModel) Provider() string {
	if m.failed {
		return "openai"
	}
	return "anthropic"
}
func (m *bufferedFallbackModel) ModelID() string {
	if m.failed {
		return "gpt-6-luna"
	}
	return "claude-haiku-5-5"
}
func (m *bufferedFallbackModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := []provider.StreamPart{
		{Type: provider.PartStreamStart},
		{Type: provider.PartResponseMeta, Provider: m.Provider(), ModelID: m.ModelID()},
		{Type: provider.PartTextStart, ID: "answer"},
		{Type: provider.PartTextDelta, ID: "answer", Delta: "Haiku decision"},
		{Type: provider.PartTextEnd, ID: "answer"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(50)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}},
	}
	if m.failed {
		parts[3].Delta = "discarded Luna decision"
		parts = append(parts[:4], provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "upstream interrupted", StatusCode: 503})})
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, p := range parts {
		stream <- p
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func bufferedFallbackScenario() (*fallback.Model, error) {
	model, err := fallback.New(&bufferedFallbackModel{failed: true}, &bufferedFallbackModel{})
	if err != nil {
		return nil, err
	}
	return model.WithBufferedStream(1<<20, time.Second)
}
