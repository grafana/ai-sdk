package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	modellogger "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_IndependentOperatorAndConsumerCapture(t *testing.T) {
	for _, fail := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			for _, observed := range []bool{false, true} {
				for _, capture := range []bool{false, true} {
					t.Run(fmt.Sprintf("failure=%t/stream=%t/operator=%t/consumer=%t", fail, streaming, observed, capture), func(t *testing.T) {
						env := testkit.NewEnv(t, func(c *agento11y.Config) {
							c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
							c.Hooks = agento11y.HooksConfig{Enabled: false}
						})
						var operatorLogs, consumerLogs lockedBuffer
						operatorLogger := slog.New(slog.NewJSONHandler(&operatorLogs, nil))
						telemetry, err := NewTelemetry(operatorLogger)
						require.NoError(t, err)
						const output = `{"value":"structured-output-marker"}`
						schema := json.RawMessage(`{"type":"object","properties":{"value":{"type":"string","description":"structured-schema-marker"}},"required":["value"],"additionalProperties":false}`)
						finish := provider.FinishReason{Unified: provider.FinishReasonStop}
						warnings := []provider.Warning{{Type: provider.WarnOther, Message: "structured-warning-marker"}}
						checkRequest := func(opts provider.CallOptions) {
							require.NotNil(t, opts.ResponseFormat)
							assert.Equal(t, provider.ResponseFormatJSON, opts.ResponseFormat.Type)
							assert.JSONEq(t, string(schema), string(opts.ResponseFormat.Schema))
						}
						lower := &observabilityTestModel{
							generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
								checkRequest(opts)
								if fail {
									return nil, errors.New("structured-failure-marker")
								}
								return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: output}}, FinishReason: finish, Warnings: warnings}, nil
							},
							stream: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
								checkRequest(opts)
								if fail {
									return nil, errors.New("structured-failure-marker")
								}
								parts := []provider.StreamPart{
									{Type: provider.PartStreamStart, Warnings: warnings},
									{Type: provider.PartTextStart, ID: "structured-text"},
									{Type: provider.PartTextDelta, ID: "structured-text", Delta: output},
									{Type: provider.PartTextEnd, ID: "structured-text"},
									{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}},
								}
								ch := make(chan provider.StreamPart, len(parts))
								for _, part := range parts {
									ch <- part
								}
								close(ch)
								return &provider.StreamResult{Stream: ch}, nil
							},
						}
						var model provider.LanguageModel = lower
						if observed {
							factory, _, err := NewModelObservabilityFactories(telemetry, operatorLogger, &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}, 10*time.Millisecond)
							require.NoError(t, err)
							model, err = factory("public", model)
							require.NoError(t, err)
						}
						models, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "public"}, Model: model}})
						require.NoError(t, err)
						handler, err := providerv4.New(providerv4.Config{Selector: providerv4.CatalogSelector(models), Limits: serviceTestLimits()})
						require.NoError(t, err)
						server := httptest.NewServer(handler)
						defer server.Close()
						client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "consumer-credential-marker", BaseURL: server.URL, HTTPClient: server.Client()})
						require.NoError(t, err)
						remote, err := client.LanguageModel("public")
						require.NoError(t, err)
						consumer := modellogger.Wrap(remote, modellogger.Options{
							Logger:         slog.New(slog.NewJSONHandler(&consumerLogs, nil)),
							Capture:        modellogger.CaptureOptions{Inputs: capture, Outputs: capture, RequestBody: capture},
							LogStreamParts: true, PartLevel: slog.LevelInfo,
						})
						opts := provider.CallOptions{Prompt: []provider.Message{}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema}}
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						if streaming {
							result, err := consumer.DoStream(ctx, opts)
							if fail {
								require.Error(t, err)
								assert.NotContains(t, err.Error(), "structured-failure-marker")
							} else {
								require.NoError(t, err)
								var text strings.Builder
								finishes := 0
								starts := 0
								for part := range result.Stream {
									require.NotEqual(t, provider.PartError, part.Type, "%+v", part)
									if part.Type == provider.PartStreamStart {
										starts++
										assert.Equal(t, warnings, part.Warnings)
									}
									if part.Type == provider.PartTextDelta {
										text.WriteString(part.Delta)
									}
									if part.Type == provider.PartFinish {
										finishes++
									}
								}
								assert.Equal(t, output, text.String())
								assert.Equal(t, 1, finishes)
								assert.Equal(t, 1, starts)
							}
						} else {
							result, err := consumer.DoGenerate(ctx, opts)
							if fail {
								require.Error(t, err)
								assert.NotContains(t, err.Error(), "structured-failure-marker")
							} else {
								require.NoError(t, err)
								require.Len(t, result.Content, 1)
								assert.Equal(t, output, result.Content[0].Text)
								assert.Equal(t, finish, result.FinishReason)
								assert.Equal(t, warnings, result.Warnings)
							}
						}
						env.Shutdown(t)
						operatorCapture := operatorLogs.String() + testMetrics(t, telemetry)
						if observed {
							generation, err := json.Marshal(env.SingleGenerationJSON(t))
							require.NoError(t, err)
							operatorCapture += string(generation)
						} else {
							assert.Zero(t, env.RequestCount())
						}
						for _, marker := range []string{"structured-output-marker", "structured-schema-marker", "consumer-credential-marker", "structured-warning-marker", "structured-failure-marker"} {
							assert.NotContains(t, operatorCapture, marker)
						}
						assert.NotContains(t, consumerLogs.String(), "consumer-credential-marker")
						if capture && !fail {
							assert.Contains(t, consumerLogs.String(), "structured-output-marker")
							assert.Contains(t, consumerLogs.String(), "structured-schema-marker")
						} else {
							assert.NotContains(t, consumerLogs.String(), "structured-output-marker")
							assert.NotContains(t, consumerLogs.String(), "structured-schema-marker")
						}
					})
				}
			}
		}
	}
}
