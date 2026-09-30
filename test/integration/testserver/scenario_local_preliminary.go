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
	registerScenario("local-preliminary", handleLocalPreliminary)
}

type localPreliminaryModel struct{}

func (localPreliminaryModel) SpecificationVersion() string               { return "v4" }
func (localPreliminaryModel) Provider() string                           { return "test" }
func (localPreliminaryModel) ModelID() string                            { return "test-local-preliminary" }
func (localPreliminaryModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (localPreliminaryModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (localPreliminaryModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 2)
	stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "local-1", ToolName: "lookup", Input: `{}`}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleLocalPreliminary(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), localPreliminaryModel{},
		aisdk.WithModelMessages(provider.UserText("Look up data.")),
		aisdk.WithTools(aisdk.ToolSet{"lookup": {ExecuteStream: func(_ context.Context, _ json.RawMessage, _ aisdk.ToolExecutionOptions, emit func(json.RawMessage) error) error {
			if err := emit(json.RawMessage(`{"status":"loading"}`)); err != nil {
				return err
			}
			return emit(json.RawMessage(`{"status":"done"}`))
		}}}),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
