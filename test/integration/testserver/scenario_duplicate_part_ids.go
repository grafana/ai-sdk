package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("duplicate-part-ids", handleDuplicatePartIDs)
}

type duplicatePartIDsModel struct {
	callCount int
}

func (m *duplicatePartIDsModel) SpecificationVersion() string               { return "v4" }
func (m *duplicatePartIDsModel) Provider() string                           { return "test" }
func (m *duplicatePartIDsModel) ModelID() string                            { return "test-duplicate-part-ids" }
func (m *duplicatePartIDsModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *duplicatePartIDsModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *duplicatePartIDsModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.callCount++
	stream := make(chan provider.StreamPart, 10)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "0"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: []string{"First answer.", "Second answer."}[m.callCount-1]}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "0"}
	stream <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "0"}
	stream <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "0", Delta: []string{"First thought.", "Second thought."}[m.callCount-1]}
	stream <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "0"}
	if m.callCount == 1 {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "continue-1", ToolName: "continue", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}}
	} else {
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleDuplicatePartIDs(w http.ResponseWriter, r *http.Request) {
	generated := 0
	result := aisdk.StreamText(r.Context(), &duplicatePartIDsModel{},
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithTools(aisdk.ToolSet{"continue": {
			Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"ok":true}`), nil
			},
		}}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
		aisdk.WithGenerateID(func() string {
			id := "generated-" + strconv.Itoa(generated)
			generated++
			return id
		}),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
