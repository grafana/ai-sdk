package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionTools_MetadataOnlyExports(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, continuation := range []bool{false, true} {
			t.Run(map[bool]string{false: "unary", true: "stream"}[streaming]+map[bool]string{false: "/call", true: "/continuation"}[continuation], func(t *testing.T) {
				env := testkit.NewEnv(t, func(configuration *agento11y.Config) {
					configuration.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
					configuration.Hooks = agento11y.HooksConfig{Enabled: false}
				})
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				telemetry, err := NewTelemetry(logger)
				require.NoError(t, err)
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
				factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				reason := provider.FinishReason{Unified: provider.FinishReasonToolCalls}
				if continuation {
					reason.Unified = provider.FinishReasonStop
				}
				lower := &observabilityTestModel{
					generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "private-call", ToolName: "private-name", Input: json.RawMessage(`{"secret":"private-input"}`)}}, FinishReason: reason}, nil
					},
					stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						ch := make(chan provider.StreamPart, 5)
						ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "private-call", ToolName: "private-name"}
						ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "private-call", Delta: "private-delta"}
						ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "private-call"}
						ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "private-call", ToolName: "private-name", Input: `{"secret":"private-input"}`}
						ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &reason, Usage: &provider.Usage{}}
						close(ch)
						return &provider.StreamResult{Stream: ch}, nil
					},
				}
				model, err := factory("grafana/assistant", lower)
				require.NoError(t, err)
				options := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "private-name", InputSchema: json.RawMessage(`{"type":"object","description":"private-schema"}`)}}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "private-name"}}
				if continuation {
					options.Prompt = []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart("private-call", "private-name", json.RawMessage(`{"secret":"private-input"}`))), provider.NewToolMessage(provider.ToolResultPart("private-call", "private-name", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "private-result"}))}
				}
				if streaming {
					result, err := model.DoStream(context.Background(), options)
					require.NoError(t, err)
					for range result.Stream {
					}
				} else {
					_, err := model.DoGenerate(context.Background(), options)
					require.NoError(t, err)
				}
				env.Shutdown(t)
				generation := env.SingleGenerationJSON(t)
				encoded, err := json.Marshal(generation)
				require.NoError(t, err)
				all := string(encoded) + logs.String() + testMetrics(t, telemetry)
				for _, secret := range []string{"private-name", "private-call", "private-input", "private-schema", "private-result", "private-delta"} {
					assert.NotContains(t, all, secret)
				}
				assert.Equal(t, string(reason.Unified), generation["stop_reason"])
				assert.Equal(t, "grafana", testkit.StringValue(t, generation, "model", "provider"))
				assert.Equal(t, "grafana/assistant", testkit.StringValue(t, generation, "model", "name"))
			})
		}
	}
}
