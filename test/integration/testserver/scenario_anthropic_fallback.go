package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("anthropic-fallback-content", handleAnthropicFallbackContent)
}

type anthropicFallbackModel struct{}

func (*anthropicFallbackModel) SpecificationVersion() string               { return "v4" }
func (*anthropicFallbackModel) Provider() string                           { return "anthropic" }
func (*anthropicFallbackModel) ModelID() string                            { return "secondary" }
func (*anthropicFallbackModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*anthropicFallbackModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*anthropicFallbackModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := []provider.StreamPart{
		{Type: provider.PartReasoningStart, ID: "before"},
		{Type: provider.PartReasoningDelta, ID: "before", Delta: "primary thinking"},
		{Type: provider.PartReasoningEnd, ID: "before", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig-1"}`)}},
		{Type: provider.PartCustom, Kind: "anthropic.fallback", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`)}},
		{Type: provider.PartReasoningStart, ID: "after"},
		{Type: provider.PartReasoningDelta, ID: "after", Delta: "secondary thinking"},
		{Type: provider.PartReasoningEnd, ID: "after", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig-2"}`)}},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleAnthropicFallbackContent(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &anthropicFallbackModel{}, aisdk.WithModelMessages(provider.UserText("continue")))
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream(aisdk.WithUIMessageStreamReasoning(true), aisdk.WithUIMessageStreamGenerateID(func() string { return "fallback-message" }))); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
