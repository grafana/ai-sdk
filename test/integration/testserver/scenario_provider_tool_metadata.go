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
	registerScenario("provider-tool-metadata", handleProviderToolMetadata)
}

type providerToolMetadataModel struct{}

func (*providerToolMetadataModel) SpecificationVersion() string               { return "v4" }
func (*providerToolMetadataModel) Provider() string                           { return "test" }
func (*providerToolMetadataModel) ModelID() string                            { return "test-provider-tool-metadata" }
func (*providerToolMetadataModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*providerToolMetadataModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*providerToolMetadataModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	preliminary := true
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "image-1", ToolName: "image_generation", Input: `{}`, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "image-1", ToolName: "image_generation", Result: json.RawMessage(`{"stage":"preview"}`), Preliminary: &preliminary, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "image-1", ToolName: "image_generation", Result: json.RawMessage(`{"stage":"final"}`), ProviderExecuted: true}
	stream <- provider.StreamPart{
		Type: provider.PartCustom,
		Kind: "openai.compaction",
		ProviderMetadata: provider.ProviderMetadata{
			"openai": json.RawMessage(`{"itemId":"cmp-1","encryptedContent":"encrypted"}`),
		},
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleProviderToolMetadata(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &providerToolMetadataModel{},
		aisdk.WithModelMessages(provider.UserText("create image")),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
