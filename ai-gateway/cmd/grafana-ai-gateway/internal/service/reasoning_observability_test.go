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

func TestReasoningMetadataOnlyPrivacy(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			env := testkit.NewEnv(t, func(config *agento11y.Config) {
				config.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
				config.Hooks = agento11y.HooksConfig{Enabled: false}
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			factory, err := NewModelObservabilityFactory(telemetry, logger, &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}, 10*time.Millisecond)
			require.NoError(t, err)
			metadata := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"private-signature","redactedData":"private-redacted"}`), "openai": json.RawMessage(`{"itemId":"private-id","reasoningEncryptedContent":"private-encrypted"}`)}
			finish := provider.FinishReason{Unified: provider.FinishReasonStop}
			data := provider.URLDataContent("https://private-file.example/reasoning")
			lower := &observabilityTestModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentReasoning, Text: "private-thought", ProviderMetadata: metadata}, {Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &data, ProviderMetadata: metadata}}, FinishReason: finish}, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					ch := make(chan provider.StreamPart, 5)
					for _, part := range []provider.StreamPart{{Type: provider.PartReasoningStart, ID: "private-id"}, {Type: provider.PartReasoningDelta, ID: "private-id", Delta: "private-thought"}, {Type: provider.PartReasoningEnd, ID: "private-id", ProviderMetadata: metadata}, {Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: data.URL}, ProviderMetadata: metadata}, {Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}}} {
						ch <- part
					}
					close(ch)
					return &provider.StreamResult{Stream: ch}, nil
				},
			}
			model, err := factory("grafana/assistant", lower)
			require.NoError(t, err)
			if streaming {
				result, err := model.DoStream(context.Background(), provider.CallOptions{})
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
				require.NoError(t, err)
			}
			env.Shutdown(t)
			encoded, err := json.Marshal(env.SingleGenerationJSON(t))
			require.NoError(t, err)
			all := string(encoded) + logs.String() + testMetrics(t, telemetry)
			for _, secret := range []string{"private-thought", "private-signature", "private-redacted", "private-id", "private-encrypted", "private-file.example"} {
				assert.NotContains(t, all, secret)
			}
		})
	}
}
