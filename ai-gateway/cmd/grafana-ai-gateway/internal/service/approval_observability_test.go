package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/middleware"
	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	grafana "github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApproval_IndependentConsumerAndOperatorObservation(t *testing.T) {
	metadata := provider.ProviderMetadata{"future": json.RawMessage(`{"application-token":"ordinary-value","empty":{},"false":false}`)}
	content := []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "mcp.lookup", Input: json.RawMessage(`{}`), ProviderExecuted: true, ProviderMetadata: metadata}, {Type: provider.ContentToolApprovalRequest, ToolCallID: "call", ApprovalID: "approval", ProviderMetadata: metadata}}
	streamParts := []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "mcp.lookup", Input: `{}`, ProviderExecuted: true, ProviderMetadata: metadata}, {Type: provider.PartToolApprovalRequest, ToolCallID: "call", ApprovalID: "approval", ProviderMetadata: metadata}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}}
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "operator disabled", true: "operator enabled"}[enabled], func(t *testing.T) {
			env := testkit.NewEnv(t, func(c *agento11y.Config) {
				c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				c.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs lockedBuffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			lower := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{Content: content, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				parts := make(chan provider.StreamPart, len(streamParts))
				for _, p := range streamParts {
					parts <- p
				}
				close(parts)
				return &provider.StreamResult{Stream: parts}, nil
			}}
			var model provider.LanguageModel = lower
			if enabled {
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}
				factory, _, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				model, err = factory("grafana/approvals", model)
				require.NoError(t, err)
			}
			serverModel := model
			handler, err := providerv4.New(providerv4.Config{Limits: serviceTestLimits(), Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (providerv4.Selection, error) {
				return providerv4.Selection{ID: "grafana/approvals", Model: serverModel}, nil
			}})
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			defer server.Close()
			client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "test", BaseURL: server.URL})
			require.NoError(t, err)
			model, err = client.LanguageModel("approvals")
			require.NoError(t, err)
			var consumerLogs lockedBuffer
			model = logmiddleware.Wrap(model, logmiddleware.Options{Logger: slog.New(slog.NewJSONHandler(&consumerLogs, &slog.HandlerOptions{Level: slog.LevelDebug})), LogStreamParts: true, Capture: logmiddleware.CaptureOptions{Outputs: true, ToolInputs: true, ToolOutputs: true, ProviderMetadata: true}})
			var observedUnary []provider.GenerateContentPart
			var observedStream []provider.StreamPart
			model = middleware.WrapLanguageModel(model, middleware.Middleware{WrapGenerate: func(ctx context.Context, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
				r, err := p.DoGenerate(ctx)
				if err == nil {
					observedUnary = append(observedUnary, r.Content...)
				}
				return r, err
			}, WrapStream: func(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
				r, err := p.DoStream(ctx)
				if err != nil {
					return nil, err
				}
				input := r.Stream
				out := make(chan provider.StreamPart)
				go func() {
					defer close(out)
					for part := range input {
						observedStream = append(observedStream, part)
						select {
						case out <- part:
						case <-ctx.Done():
							return
						}
					}
				}()
				r.Stream = out
				return r, nil
			}})
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolApprovalResponsePart("prior", false, ""))}}
			generated, err := model.DoGenerate(context.Background(), options)
			require.NoError(t, err)
			assert.Equal(t, content, generated.Content)
			assert.Equal(t, content, observedUnary)
			streamed, err := model.DoStream(context.Background(), options)
			require.NoError(t, err)
			var returned []provider.StreamPart
			for p := range streamed.Stream {
				returned = append(returned, p)
			}
			assert.Contains(t, returned, streamParts[1])
			assert.Equal(t, returned, observedStream)
			assert.NotContains(t, consumerLogs.String(), "ordinary-value")
			assert.Contains(t, consumerLogs.String(), "[REDACTED]")
			assert.Contains(t, consumerLogs.String(), "approval")
			env.Shutdown(t)
			if enabled {
				assert.Equal(t, 2, env.RequestCount())
				capture := logs.String() + testMetrics(t, telemetry)
				assert.NotContains(t, capture, "ordinary-value")
			} else {
				assert.Zero(t, env.RequestCount())
			}
		})
	}
}
