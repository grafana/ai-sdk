package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() { registerScenario("ordered-error-evidence", handleOrderedErrorEvidence) }

type orderedErrorEvidenceModel struct{}

func (*orderedErrorEvidenceModel) SpecificationVersion() string               { return "v4" }
func (*orderedErrorEvidenceModel) Provider() string                           { return "test" }
func (*orderedErrorEvidenceModel) ModelID() string                            { return "ordered-error-evidence" }
func (*orderedErrorEvidenceModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*orderedErrorEvidenceModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*orderedErrorEvidenceModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	metadata := provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"requestedModelId":"alias","canonicalModelId":"public","attempts":[{"provider":"test","providerInstance":"configured","modelId":"native","outcome":"selected"}]}}`)}
	data, err := json.Marshal(struct {
		Metadata    provider.ProviderMetadata `json:"providerMetadata"`
		NativeError map[string]string         `json:"nativeError"`
	}{metadata, map[string]string{"message": "private-native-message"}})
	if err != nil {
		return nil, err
	}
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "before"},
		{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "private-native-message", Data: data})},
		{Type: provider.PartTextDelta, ID: "text", Delta: "after"},
		{Type: provider.PartTextEnd, ID: "text"},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}, ProviderMetadata: metadata},
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleOrderedErrorEvidence(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &orderedErrorEvidenceModel{}, aisdk.WithModelMessages(provider.UserText("ordered evidence")))
	if err := aisdk.WriteUIMessageStream(w, result, aisdk.OnUIMessageStreamError(func(error) string { return "generation failed" })); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
