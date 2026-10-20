package main

import (
	"context"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("citations-text-input", handleCitationsTextInput)
}

type citationsTextInputModel struct{ sourceDocumentModel }

func (*citationsTextInputModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "t"},
		{Type: provider.PartTextDelta, ID: "t", Delta: "answer"},
		{Type: provider.PartTextEnd, ID: "t", Citations: []provider.Citation{
			{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "url", URL: "https://example.com"}, StartIndex: new(0), EndIndex: new(6)},
			{Source: provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "", MediaType: "text/plain", Title: ""}, CitedText: new("")},
		}},
		{Type: provider.PartToolInputStart, ID: "c", ToolName: "write", InputEncoding: provider.ToolInputEncodingText},
		{Type: provider.PartToolInputDelta, ID: "c", Delta: "SELECT \""},
		{Type: provider.PartToolInputDelta, ID: "c", Delta: "value\"\n"},
		{Type: provider.PartToolInputEnd, ID: "c"},
		{Type: provider.PartToolCall, ToolCallID: "c", ToolName: "write", Input: `"SELECT \"value\"\n"`},
		{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}},
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleCitationsTextInput(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &citationsTextInputModel{}, aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithTools(aisdk.ToolSet{"write": {Type: aisdk.UserToolDynamic}}))
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream(aisdk.WithUIMessageStreamGenerateID(func() string { return "citation-message" }))); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
