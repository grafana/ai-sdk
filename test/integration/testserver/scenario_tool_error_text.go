package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const (
	toolErrorTextFailure  = "weather backend timeout"
	toolErrorTextRedacted = "redacted"
)

// The tool-error-text scenario fails a tool in the first step. The second step
// answers with the error text it received as the tool result, so a client can
// compare the UI error text with what the model saw.
func init() {
	registerScenario("tool-error-text", handleToolErrorText)
}

type toolErrorTextModel struct{ calls int }

func (*toolErrorTextModel) SpecificationVersion() string               { return "v4" }
func (*toolErrorTextModel) Provider() string                           { return "test" }
func (*toolErrorTextModel) ModelID() string                            { return "test-tool-error-text" }
func (*toolErrorTextModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*toolErrorTextModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func (m *toolErrorTextModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	stream := make(chan provider.StreamPart, 5)
	if m.calls == 1 {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "weather-1", ToolName: "weather", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "missing-1", ToolName: "missing", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}}
	} else {
		received := map[string]string{}
		for _, message := range opts.Prompt {
			for _, part := range message.Content {
				if part.Type == provider.ContentPartTypeToolResult && part.Output != nil {
					received[part.ToolCallID] = part.Output.Text
				}
			}
		}
		encoded, err := json.Marshal(received)
		if err != nil {
			return nil, err
		}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: string(encoded)}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleToolErrorText(w http.ResponseWriter, r *http.Request) {
	result := aisdk.StreamText(r.Context(), &toolErrorTextModel{},
		aisdk.WithModelMessages(provider.UserText("weather")),
		aisdk.WithTools(aisdk.ToolSet{"weather": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			return nil, errors.New(toolErrorTextFailure)
		}}}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
	)
	var opts []aisdk.UIMessageStreamOption
	onError := aisdk.OnUIMessageStreamError(func(error) string { return toolErrorTextRedacted })
	switch r.URL.Query().Get("mode") {
	case "callback":
		opts = append(opts, onError)
	case "callback-unredacted":
		opts = append(opts, onError, aisdk.WithUIMessageStreamRedactToolErrors(false))
	}
	if err := aisdk.WriteUIMessageStream(w, result, opts...); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
