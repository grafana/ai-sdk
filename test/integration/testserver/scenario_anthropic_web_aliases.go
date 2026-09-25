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
	registerScenario("anthropic-web-aliases", handleAnthropicWebAliases)
}

type anthropicWebAliasesModel struct{}

func (*anthropicWebAliasesModel) SpecificationVersion() string               { return "v4" }
func (*anthropicWebAliasesModel) Provider() string                           { return "test" }
func (*anthropicWebAliasesModel) ModelID() string                            { return "anthropic-web-aliases" }
func (*anthropicWebAliasesModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*anthropicWebAliasesModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*anthropicWebAliasesModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "search-1", ToolName: "search_latest", Input: `{"query":"Go"}`, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "search-1", ToolName: "search_latest", Result: json.RawMessage(`[{"type":"web_search_result","url":"https://example.com"}]`), ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "fetch-1", ToolName: "fetch_latest", Input: `{"url":"https://example.com"}`, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "fetch-1", ToolName: "fetch_latest", Result: json.RawMessage(`{"type":"web_fetch_result","url":"https://example.com"}`), ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleAnthropicWebAliases(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &anthropicWebAliasesModel{}, aisdk.WithModelMessages(provider.UserText("search and fetch")))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
