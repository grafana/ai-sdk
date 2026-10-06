package main

import (
	"context"
	"encoding/json"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() { registerScenario("reasoning-continuation", handleReasoningContinuation) }

type reasoningContinuationModel struct{ simpleTextModel }

func (*reasoningContinuationModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	old := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"old","reasoningEncryptedContent":null}`)}
	final := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"final"}`)}
	parts := []provider.StreamPart{
		{Type: provider.PartReasoningStart, ID: "same", ProviderMetadata: old},
		{Type: provider.PartReasoningStart, ID: "other", ProviderMetadata: old},
		{Type: provider.PartTextStart, ID: "same"},
		{Type: provider.PartReasoningDelta, ID: "same", Delta: "one"},
		{Type: provider.PartReasoningDelta, ID: "other", Delta: "two"},
		{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}, ProviderMetadata: final},
		{Type: provider.PartTextDelta, ID: "same", Delta: "answer"},
		{Type: provider.PartReasoningEnd, ID: "other", ProviderMetadata: provider.ProviderMetadata{}},
		{Type: provider.PartReasoningEnd, ID: "same", ProviderMetadata: final},
		{Type: provider.PartTextEnd, ID: "same"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
	}
	ch := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		ch <- part
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}
func handleReasoningContinuation(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &reasoningContinuationModel{}, aisdk.WithModelMessages(provider.UserText("think")))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
