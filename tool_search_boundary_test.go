package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolSearch_ProviderDrivenUIClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		toolType UserToolType
		unknown  bool
	}{
		{name: "static", toolType: UserToolFunction},
		{name: "provider", toolType: UserToolProvider},
		{name: "unknown dynamic", unknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			localCalls := 0
			tool := Tool{Type: tc.toolType, ID: "test.weather", DeferLoading: true,
				InputSchema:   testMustSchema(t, `{"type":"object","properties":{"secret":{"type":"string"}},"required":["secret"]}`),
				ValidateInput: func(json.RawMessage) error { localCalls++; return nil },
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					localCalls++
					return json.RawMessage(`"local"`), nil
				},
				NeedsApproval:    ApprovalRequired(),
				OnInputStart:     func(ToolExecutionOptions) { localCalls++ },
				OnInputDelta:     func(string, ToolExecutionOptions) { localCalls++ },
				OnInputAvailable: func(json.RawMessage, ToolExecutionOptions) { localCalls++ },
			}
			registry := ToolSet{"weather": tool}
			var dynamic *bool
			if tc.unknown {
				registry = ToolSet{"unrelated": {DeferLoading: true}}
				dynamic = new(true)
			}
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: discoveryParts(
					provider.StreamPart{Type: provider.PartToolInputStart, ID: "provider", ToolName: "weather", ProviderExecuted: true, Dynamic: new(true)},
					provider.StreamPart{Type: provider.PartToolInputDelta, ID: "provider", Delta: `{}`},
					provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "provider", ToolName: "weather", Input: `{}`, ProviderExecuted: true, Dynamic: new(true)},
					provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "provider", ToolName: "weather", Result: json.RawMessage(`"sunny"`), ProviderExecuted: true, Dynamic: dynamic},
				)}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(registry))
			seen := 0
			for chunk := range result.ToUIMessageStream() {
				switch chunk.Type {
				case ChunkToolInputStart, ChunkToolInputAvailable, ChunkToolOutputAvailable:
					seen++
					assert.Equal(t, dynamic, chunk.Dynamic)
					assert.True(t, chunk.ProviderExecuted)
				}
			}
			require.NoError(t, result.Err())
			assert.Equal(t, 3, seen)
			assert.Zero(t, localCalls)
			require.Len(t, result.ToolCalls(), 1)
			assert.Equal(t, new(true), result.ToolCalls()[0].Dynamic)
		})
	}
}

func TestToolSearch_InactiveDescriptions(t *testing.T) {
	for _, routes := range []ToolRoutes{nil, {}} {
		for _, active := range [][]string{{"active"}, {}} {
			t.Run(marshalJSON(t, routes)+"/"+marshalJSON(t, active), func(t *testing.T) {
				activeCalls, inactiveCalls, executions := 0, 0, 0
				registry := ToolSet{
					"active": {DescriptionFunc: func(ToolDescriptionOptions) string { activeCalls++; return "active description" }},
					"inactive": {DescriptionFunc: func(ToolDescriptionOptions) string { inactiveCalls++; return "inactive description" }, Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
						executions++
						return json.RawMessage(`"done"`), nil
					}},
				}
				model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
					assert.ElementsMatch(t, active, toolNames(opts.Tools))
					if len(active) > 0 {
						assert.Equal(t, "active description", opts.Tools[0].Description)
					}
					return &provider.StreamResult{Stream: toolCallStreamParts("inactive", `{}`)}, nil
				}}
				result := StreamText(t.Context(), model, WithTools(registry), WithToolRoutes(routes), WithActiveTools(active...))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				assert.Equal(t, len(active), activeCalls)
				assert.Zero(t, inactiveCalls)
				if routes == nil {
					assert.Equal(t, 1, executions)
				} else {
					assert.Zero(t, executions)
				}
			})
		}
	}
}
