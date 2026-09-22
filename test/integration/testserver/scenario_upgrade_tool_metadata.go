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
	registerScenario("upgrade-tool-metadata", handleUpgradeToolMetadata)
}

type upgradeToolMetadataModel struct{}

func (*upgradeToolMetadataModel) SpecificationVersion() string               { return "v4" }
func (*upgradeToolMetadataModel) Provider() string                           { return "test" }
func (*upgradeToolMetadataModel) ModelID() string                            { return "upgrade-tool-metadata" }
func (*upgradeToolMetadataModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*upgradeToolMetadataModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*upgradeToolMetadataModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 9)
	metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"parallelToolCall":{"itemId":"fc_parallel","toolCallId":"call_parallel","toolName":"parallel","input":"wrapper-input","index":0,"count":1}}`)}
	stream <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "call_parallel_0", ToolName: "weather"}
	stream <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call_parallel_0", Delta: `{"location":"SF"}`}
	stream <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call_parallel_0"}
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_parallel_0", ToolName: "weather", Input: `{"location":"SF"}`, ProviderMetadata: metadata}
	caller := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":"program_1"}}`)}
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "search_1", ToolName: "web_search", Input: `{}`, ProviderExecuted: true, ProviderMetadata: caller}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "search_1", ToolName: "web_search", Result: json.RawMessage(`[]`), ProviderExecuted: true, ProviderMetadata: caller}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleUpgradeToolMetadata(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &upgradeToolMetadataModel{},
		aisdk.WithModelMessages(provider.UserText("show tool metadata")),
		aisdk.WithTools(aisdk.ToolSet{"weather": {}}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
