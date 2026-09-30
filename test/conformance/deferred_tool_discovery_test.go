package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/require"
)

type discoveryFixtureModel struct {
	uiFixtureModel
	Steps [][]struct {
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	requests []map[string]any
}

func (m *discoveryFixtureModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	step := len(m.requests)
	if step >= len(m.Steps) {
		return nil, fmt.Errorf("unexpected discovery step %d", step)
	}
	raw, err := json.Marshal(map[string]any{"tools": opts.Tools, "prompt": opts.Prompt, "toolChoice": opts.ToolChoice})
	if err != nil {
		return nil, err
	}
	var request map[string]any
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	m.requests = append(m.requests, request)
	stream := make(chan provider.StreamPart, len(m.Steps[step])+4)
	for _, call := range m.Steps[step] {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: call.ID, ToolName: call.Name, Input: string(call.Input)}
	}
	reason := provider.FinishReasonToolCalls
	if step == 2 {
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "sunny"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}
		reason = provider.FinishReasonStop
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func TestUIConformance_DeferredToolDiscovery(t *testing.T) {
	for _, mode := range []string{"direct", "nested"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join("ui", "deferred-tool-discovery", mode)
			model := &discoveryFixtureModel{}
			data, err := os.ReadFile(filepath.Join(dir, "scenario.json"))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, model))
			empty, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`))
			require.NoError(t, err)
			calls := 0
			tools := aisdk.ToolSet{
				"search": aisdk.ToolSearch(),
				"getWeather": {DeferLoading: true, Description: "Weather forecast", InputSchema: empty, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
					calls++
					return json.RawMessage(`"sunny"`), nil
				}},
				"unrelated": {DeferLoading: true, Description: "Send email", InputSchema: empty},
			}
			options := []aisdk.StreamOption{aisdk.WithModelMessages(provider.UserText("Find the weather.")), aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(3))}
			if mode == "nested" {
				inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"input":{"type":"object"}},"required":["name","input"],"additionalProperties":false}`))
				require.NoError(t, err)
				tools["code"] = aisdk.Tool{Description: "Stable code tool.", InputSchema: inputSchema, Caller: &aisdk.ToolCaller{Type: aisdk.ToolCallerLocal,
					Bind: func(registry aisdk.ToolSet) aisdk.Tool {
						return aisdk.Tool{Description: "Bound tool", InputSchema: inputSchema, Execute: func(ctx context.Context, input json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
							var call struct {
								Name  string
								Input json.RawMessage
							}
							if err := json.Unmarshal(input, &call); err != nil {
								return nil, err
							}
							tool, ok := registry[call.Name]
							if !ok {
								return nil, fmt.Errorf("missing nested tool %q", call.Name)
							}
							return tool.Execute(ctx, call.Input, opts)
						}}
					},
					PrepareModelMessage: func(registry aisdk.ToolSet) *string {
						message := "Catalog: "
						for i, name := range slices.Sorted(maps.Keys(registry)) {
							if i > 0 {
								message += ", "
							}
							message += name
						}
						return &message
					},
				}}
				options = append(options, aisdk.WithToolRoutes(aisdk.ToolRoutes{"search": {Callers: []string{"code"}}, "getWeather": {Callers: []string{"code"}}, "unrelated": {Callers: []string{"code"}}}))
			}
			result := aisdk.StreamText(t.Context(), model, options...)
			var chunks []map[string]any
			for chunk := range result.ToUIMessageStream(aisdk.WithUIMessageStreamGenerateID(func() string { return "message-1" })) {
				raw, err := json.Marshal(chunk)
				require.NoError(t, err)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(raw, &decoded))
				chunks = append(chunks, decoded)
			}
			require.NoError(t, result.Err())
			require.Equal(t, 1, calls)
			require.Equal(t, loadUIExpected(t, filepath.Join(dir, "expected-requests.jsonl")), model.requests)
			require.Equal(t, loadUIExpected(t, filepath.Join(dir, "expected.jsonl")), chunks)
		})
	}
}
