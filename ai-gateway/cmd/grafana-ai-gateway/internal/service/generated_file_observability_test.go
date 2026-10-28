package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/middleware"
	clientlogger "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFileCaptureIndependence(t *testing.T) {
	for _, serverCapture := range []bool{false, true} {
		for _, consumerCapture := range []bool{false, true} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("server=%t/consumer=%t/stream=%t", serverCapture, consumerCapture, streaming), func(t *testing.T) {
					env := testkit.NewEnv(t, func(c *agento11y.Config) {
						if !serverCapture {
							c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
						}
						c.Hooks = agento11y.HooksConfig{Enabled: false}
					})
					var serverLogs lockedBuffer
					log := slog.New(slog.NewJSONHandler(&serverLogs, nil))
					telemetry, err := NewTelemetry(log)
					require.NoError(t, err)
					factory, _, err := NewModelObservabilityFactories(telemetry, log, &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry}, 10*time.Millisecond)
					require.NoError(t, err)
					original := "opaque:Original?token=application"
					data := provider.URLDataContent("https://example.test/ordinary-media.png")
					data.OriginalURL = &original
					metadata := provider.ProviderMetadata{"app": json.RawMessage(`{"token":"ordinary-application-value","label":"ordinary-metadata","nested":[null,false]}`)}
					finish := provider.FinishReason{Unified: provider.FinishReasonStop}
					lower := &observabilityTestModel{
						generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
							return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data, ProviderMetadata: metadata}}, FinishReason: finish}, nil
						},
						stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
							return fallbackParts(provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: data.URL, OriginalURL: &original}, ProviderMetadata: metadata}, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}}), nil
						},
					}
					var operatorLogs lockedBuffer
					operatorModel := middleware.WrapLanguageModel(lower, clientlogger.Middleware(clientlogger.Options{Logger: slog.New(slog.NewJSONHandler(&operatorLogs, &slog.HandlerOptions{Level: slog.LevelDebug})), LogStreamParts: true, Capture: clientlogger.CaptureOptions{Outputs: serverCapture, Files: serverCapture, ProviderMetadata: serverCapture, MaxJSONBytes: 16384}}))
					observed, err := factory("files", operatorModel)
					require.NoError(t, err)
					handler, err := newWireHandler(execution.Config{Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
						return execution.Selection{ID: "files", Model: observed}, nil
					}, Limits: serviceTestLimits()})
					require.NoError(t, err)
					server := httptest.NewServer(handler)
					defer server.Close()
					client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{BaseURL: server.URL, AccessToken: "synthetic"})
					require.NoError(t, err)
					model, err := client.LanguageModel("files")
					require.NoError(t, err)
					var consumerLogs lockedBuffer
					wrapped := middleware.WrapLanguageModel(model, clientlogger.Middleware(clientlogger.Options{Logger: slog.New(slog.NewJSONHandler(&consumerLogs, &slog.HandlerOptions{Level: slog.LevelDebug})), LogStreamParts: true, Capture: clientlogger.CaptureOptions{Outputs: consumerCapture, Files: consumerCapture, ProviderMetadata: consumerCapture, ResponseBody: consumerCapture, MaxJSONBytes: 16384}}))
					if streaming {
						result, err := wrapped.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{}})
						require.NoError(t, err)
						found := false
						for part := range result.Stream {
							require.NotEqual(t, provider.PartError, part.Type)
							if part.Type == provider.PartFile {
								found = true
								require.Equal(t, data.URL, part.Data.URL)
								require.Equal(t, &original, part.Data.OriginalURL)
								require.Equal(t, metadata, part.ProviderMetadata)
							}
						}
						require.True(t, found)
					} else {
						result, err := wrapped.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{}})
						require.NoError(t, err)
						require.Len(t, result.Content, 1)
						require.Equal(t, data, *result.Content[0].Data)
						require.Equal(t, metadata, result.Content[0].ProviderMetadata)
					}
					require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
					env.Shutdown(t)
					captured, err := json.Marshal(env.SingleGenerationJSON(t))
					require.NoError(t, err)
					require.NotContains(t, string(captured), "ordinary-media.png")
					require.Equal(t, serverCapture, strings.Contains(operatorLogs.String(), "ordinary-media.png"), operatorLogs.String())
					require.Equal(t, serverCapture, strings.Contains(operatorLogs.String(), "ordinary-metadata"), operatorLogs.String())
					require.NotContains(t, serverLogs.String(), "ordinary-media.png")
					require.Equal(t, consumerCapture, strings.Contains(consumerLogs.String(), "ordinary-media.png"), consumerLogs.String())
					require.Equal(t, consumerCapture, strings.Contains(consumerLogs.String(), "ordinary-metadata"), consumerLogs.String())
				})
			}
		}
	}
}
