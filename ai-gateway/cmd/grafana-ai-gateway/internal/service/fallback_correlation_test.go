package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackRoute_OneLogicalGenerationJoinsPhysicalDecisions(t *testing.T) {
	env := testkit.NewEnv(t, func(cfg *agento11y.Config) {
		cfg.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
		cfg.Hooks = agento11y.HooksConfig{Enabled: false}
	})
	var logs lockedBuffer
	var output physicalBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
	factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, 8, time.Second, time.Second)
	defer sink.Close()
	catalog, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			if id == "backend-primary" {
				return nil, errors.New("private-upstream-body")
			}
			return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "private-output"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}}
	}, factory, sink)
	require.NoError(t, err)
	model, err := catalog.ResolveModel(context.Background(), "alias")
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "joined-correlation"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	_, err = model.Model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("private-prompt")}})
	require.NoError(t, err)
	sink.Close()
	env.Shutdown(t)
	generation := env.SingleGenerationJSON(t)
	metadata, ok := generation["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "joined-correlation", metadata["gateway.correlation_id"])
	assert.Equal(t, "public", testkit.StringValue(t, generation, "model", "name"))
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	for index, line := range lines {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		assert.Equal(t, index+1, record.CandidateIndex)
		assert.Equal(t, "joined-correlation", record.CorrelationID)
		assert.Equal(t, index == 1, record.Winner)
		assert.Equal(t, index == 0, record.WillFallback)
	}
	encoded, err := json.Marshal(generation)
	require.NoError(t, err)
	logical := string(encoded) + logs.String() + testMetrics(t, telemetry)
	for _, hidden := range []string{"backend-primary", "backend-secondary", "primary-instance", "secondary-instance", "private-upstream-body", "private-prompt", "private-output"} {
		assert.NotContains(t, logical, hidden)
	}
}
