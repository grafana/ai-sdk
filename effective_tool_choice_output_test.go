package aisdk_test

import (
	"context"
	"encoding/json"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_EffectiveToolChoiceOutput(t *testing.T) {
	for _, mode := range []string{"stream", "agent stream"} {
		for _, choice := range []provider.ToolChoice{{Type: provider.ToolChoiceRequired}, {Type: provider.ToolChoiceTool, ToolName: "lookup"}} {
			for _, tc := range []struct {
				name      string
				text      string
				wantError bool
			}{
				{"valid JSON", `{"value":42}`, false},
				{"malformed JSON", `not JSON`, true},
				{"empty JSON", "", true},
			} {
				t.Run(mode+"/"+string(choice.Type)+"/"+tc.name, func(t *testing.T) {
					providerCalls, executions, errorCalls := 0, 0, 0
					model := &testModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						providerCalls++
						parts := []provider.StreamPart{
							{Type: provider.PartTextStart, ID: "t1"},
							{Type: provider.PartTextDelta, ID: "t1", Delta: tc.text},
							{Type: provider.PartTextEnd, ID: "t1"},
						}
						finish := provider.FinishReason{Unified: provider.FinishReasonStop}
						if choice.Type == provider.ToolChoiceTool {
							parts = append(parts, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`})
							finish.Unified = provider.FinishReasonToolCalls
						}
						parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish})
						stream := make(chan provider.StreamPart, len(parts))
						for _, part := range parts {
							stream <- part
						}
						close(stream)
						return &provider.StreamResult{Stream: stream}, nil
					}}
					opts := []aisdk.StreamOption{
						aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(output.JSON()),
						aisdk.WithToolChoice(choice), aisdk.WithStopWhen(aisdk.StepCountIs(3)),
						aisdk.WithTools(aisdk.ToolSet{"lookup": {}, "other": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
							executions++
							return json.RawMessage(`"unexpected"`), nil
						}}}),
						aisdk.OnError(func(error) { errorCalls++ }),
					}
					var result *aisdk.StreamTextResult
					if mode == "agent stream" {
						result = aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(opts...)).Stream(t.Context())
					} else {
						result = aisdk.StreamText(t.Context(), model, opts...)
					}
					streamErrors, stepFinishes, finishes := 0, 0, 0
					for event := range result.FullStream() {
						switch part := event.(type) {
						case aisdk.StreamError:
							streamErrors++
							assert.ErrorContains(t, part.Error, "tool choice")
						case aisdk.StreamFinishStep:
							stepFinishes++
							assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
						case aisdk.StreamFinish:
							finishes++
							assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
						}
					}
					require.ErrorContains(t, result.Err(), "tool choice")
					assert.NotErrorIs(t, result.Err(), aisdk.ErrNoObjectGenerated)
					assert.Equal(t, tc.text, result.Text())
					if tc.wantError {
						assert.Nil(t, result.OutputValue())
						require.ErrorIs(t, result.OutputError(), aisdk.ErrNoObjectGenerated)
					} else {
						require.NoError(t, result.OutputError())
						assert.Equal(t, map[string]any{"value": float64(42)}, result.OutputValue())
					}
					assert.Zero(t, executions)
					assert.Equal(t, 1, providerCalls)
					assert.Equal(t, 1, errorCalls)
					assert.Equal(t, 1, streamErrors)
					assert.Equal(t, 1, stepFinishes)
					assert.Equal(t, 1, finishes)
				})
			}
		}
	}
}
