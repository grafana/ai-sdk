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
	registerScenario("reused-part-ids", handleReusedPartIDs)
}

type reusedPartIDsModel struct {
	calls int
}

func (*reusedPartIDsModel) SpecificationVersion() string               { return "v4" }
func (*reusedPartIDsModel) Provider() string                           { return "test" }
func (*reusedPartIDsModel) ModelID() string                            { return "reused-part-ids" }
func (*reusedPartIDsModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*reusedPartIDsModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *reusedPartIDsModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	stream := make(chan provider.StreamPart, 8)
	text := "first"
	if m.calls == 2 {
		text = "second"
	}
	for _, part := range []provider.StreamPart{
		{Type: provider.PartReasoningStart, ID: "0"},
		{Type: provider.PartReasoningDelta, ID: "0", Delta: text + " thought"},
		{Type: provider.PartReasoningEnd, ID: "0"},
		{Type: provider.PartTextStart, ID: "0"},
		{Type: provider.PartTextDelta, ID: "0", Delta: text + " answer"},
		{Type: provider.PartTextEnd, ID: "0"},
	} {
		stream <- part
	}
	finish := provider.FinishReasonStop
	if m.calls == 1 {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "next-1", ToolName: "next", Input: `{}`}
		finish = provider.FinishReasonToolCalls
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: finish}, Usage: &provider.Usage{}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleReusedPartIDs(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &reusedPartIDsModel{},
		aisdk.WithModelMessages(provider.UserText("Continue after the tool.")),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
		aisdk.WithGenerateID(func() string { return "generated" }),
		aisdk.WithTools(aisdk.ToolSet{"next": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`"ok"`), nil
		}}}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
