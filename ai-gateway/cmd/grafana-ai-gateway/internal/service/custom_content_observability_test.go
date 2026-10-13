package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const customContentRequestBody = `{"prompt":[{"role":"user","content":[{"type":"text","text":"private-prompt"}]},{"role":"assistant","content":[{"type":"custom","kind":"openai.compaction","providerOptions":{"openai":{"itemId":"private-metadata-item","encryptedContent":"private-metadata-encrypted"}}}]},{"role":"user","content":[{"type":"text","text":"private-prompt"}]}]}`

func TestCustomContentTelemetry_MetadataOnlyKeepsOptionsAndMetadataPrivate(t *testing.T) {
	customMetadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"private-metadata-out","encryptedContent":"private-metadata-encrypted-out"}`)}
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			h := newFallbackAcceptance(t,
				&observabilityTestModel{
					generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
						require.Len(t, options.Prompt, 3)
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "openai.compaction", ProviderMetadata: customMetadata}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
					},
					stream: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
						require.Len(t, options.Prompt, 3)
						return fallbackParts(
							provider.StreamPart{Type: provider.PartCustom, Kind: "openai.compaction", ProviderMetadata: customMetadata},
							provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
						), nil
					},
				},
				&observabilityTestModel{},
			)
			response := httptest.NewRecorder()

			h.handler.ServeHTTP(response, h.request(context.Background(), streaming, customContentRequestBody))

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "private-metadata-out", "the caller keeps custom metadata")
			assert.Contains(t, response.Body.String(), "private-metadata-encrypted-out")
			mode := map[bool]string{false: "generate", true: "stream"}[streaming]
			h.verify(t, mode, response.Body.String(), []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}

func TestCustomContentTelemetry_Disposition(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t, func(c *agento11y.Config) {
				c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				c.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
			factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
			require.NoError(t, err)
			metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"metadata-private"}`)}
			lower := &observabilityTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "kind-private.custom", ProviderMetadata: metadata}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 3)
					parts <- provider.StreamPart{Type: provider.PartStreamStart}
					parts <- provider.StreamPart{Type: provider.PartCustom, Kind: "kind-private.custom", ProviderMetadata: metadata}
					parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			model, err := factory("grafana/assistant", lower)
			require.NoError(t, err)
			history := provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(func() provider.ContentPart {
				part := provider.CustomPart("kind-private.history")
				part.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"options-private"}`)}}
				return part
			}())}}
			if streaming {
				result, err := model.DoStream(context.Background(), history)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), history)
				require.NoError(t, err)
			}
			require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
			env.Shutdown(t)
			generation, err := json.Marshal(env.SingleGenerationJSON(t))
			require.NoError(t, err)
			metrics := testMetrics(t, telemetry)

			for _, private := range []string{"metadata-private", "options-private"} {
				assert.NotContains(t, string(generation)+logs.String()+metrics, private)
			}
			assert.NotContains(t, string(generation), "kind-private", "Agent Observability omits custom parts")
			assert.NotContains(t, logs.String(), "kind-private", "operator logs carry no custom kinds under metadata-only capture")
			if streaming {
				assert.Contains(t, metrics, "aisdk_model_time_to_first_output_seconds_count", "Prometheus counts custom as first output")
			}
		})
	}
}
