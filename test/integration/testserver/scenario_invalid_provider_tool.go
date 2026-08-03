package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("invalid-provider-tool", handleInvalidProviderTool)
}

type invalidProviderToolModel struct{}

func (*invalidProviderToolModel) SpecificationVersion() string               { return "v4" }
func (*invalidProviderToolModel) Provider() string                           { return "test" }
func (*invalidProviderToolModel) ModelID() string                            { return "test-invalid-provider-tool" }
func (*invalidProviderToolModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*invalidProviderToolModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (*invalidProviderToolModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 3)
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "search-1", ToolName: "web_search", Input: `{}`, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "search-1", ToolName: "web_search", Result: json.RawMessage(`{"type":"web_search_tool_result_error","errorCode":"invalid_tool_input"}`), IsError: true, ProviderExecuted: true}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleInvalidProviderTool(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &invalidProviderToolModel{},
		aisdk.WithModelMessages(provider.UserText("search")),
		aisdk.WithTools(aisdk.ToolSet{
			"web_search": {
				ValidateInput: func(json.RawMessage) error {
					return fmt.Errorf("query is required")
				},
			},
		}),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
