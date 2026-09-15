package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

func init() {
	registerScenario("function-tool-input", handleFunctionToolInput)
	registerScenario("function-tool-invalid-input", handleFunctionToolInput)
}

type functionToolInputModel struct {
	simpleTextModel
	invalid bool
	step    int
}

func (m *functionToolInputModel) DoStream(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.step++
	var parts []provider.StreamPart
	if m.step == 1 {
		input := `{"service":"checkout"}`
		if m.invalid {
			input = `{"service":`
		}
		parts = []provider.StreamPart{
			{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence", Title: "Read evidence"},
			{Type: provider.PartToolInputDelta, ID: "call-1", Delta: ""},
			{Type: provider.PartToolInputDelta, ID: "call-1", Delta: input},
			{Type: provider.PartToolInputEnd, ID: "call-1"},
			{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: input},
			{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}},
		}
	} else {
		var output *provider.ToolResultOutput
		for _, message := range options.Prompt {
			for _, part := range message.Content {
				if part.Type == provider.ContentPartTypeToolResult && part.ToolCallID == "call-1" {
					output = part.Output
				}
			}
		}
		if output == nil {
			return nil, fmt.Errorf("missing replayed tool result")
		}
		text := "Checkout error rate is 4.2%."
		if m.invalid {
			if output.Type != provider.ToolOutputErrorText && output.Type != provider.ToolOutputErrorJSON {
				return nil, fmt.Errorf("invalid arguments did not produce a tool error")
			}
			text = "The tool arguments were invalid."
		} else if output.Type != provider.ToolOutputJSON || string(output.JSON) != `{"errorRate":4.2}` {
			return nil, fmt.Errorf("unexpected replayed tool output")
		}
		parts = []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "answer"},
			{Type: provider.PartTextDelta, ID: "answer", Delta: text},
			{Type: provider.PartTextEnd, ID: "answer"},
			{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
		}
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleFunctionToolInput(w http.ResponseWriter, r *http.Request) {
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"service":{"type":"string"}},"required":["service"]}`))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	model := &functionToolInputModel{invalid: r.PathValue("name") == "function-tool-invalid-input"}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("Read release evidence")),
		aisdk.WithTools(aisdk.ToolSet{"read_evidence": {
			InputSchema: inputSchema,
			Execute: func(_ context.Context, input json.RawMessage, _ aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				if string(input) != `{"service":"checkout"}` {
					return nil, fmt.Errorf("unexpected tool arguments")
				}
				return json.RawMessage(`{"errorRate":4.2}`), nil
			},
		}}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
