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
	registerScenario("web-search-status", handleWebSearchStatus)
}

type webSearchStatusModel struct{}

func (webSearchStatusModel) SpecificationVersion() string               { return "v4" }
func (webSearchStatusModel) Provider() string                           { return "test" }
func (webSearchStatusModel) ModelID() string                            { return "test-web-search-status" }
func (webSearchStatusModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (webSearchStatusModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func (webSearchStatusModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 5)
	stream <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "ws_1", ToolName: "lookup", ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "ws_1"}
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "ws_1", ToolName: "lookup", Input: "{}", ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "ws_1", ToolName: "lookup", Result: json.RawMessage(`{"status":"searching"}`), IsError: true}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleWebSearchStatus(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), webSearchStatusModel{},
		aisdk.WithModelMessages(provider.UserText("search")),
		aisdk.WithTools(aisdk.ToolSet{"lookup": {Type: aisdk.UserToolProvider, ID: "openai.web_search"}}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
