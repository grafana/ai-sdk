package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackRoute_ConcurrentMappedObservation(t *testing.T) {
	const count = 12
	var logs lockedBuffer
	var output physicalBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	factory, err := NewModelObservabilityFactory(telemetry, logger, nil, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, count*2, time.Second, time.Second)
	defer sink.Close()
	created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ nativemodel.Config, id string, _ *http.Client) provider.LanguageModel {
		invoke := func(ctx context.Context, opts provider.CallOptions) error {
			marker := opts.Headers["x-request"]
			assert.Equal(t, marker, observationFromContext(ctx).correlationID)
			assert.Equal(t, mappedFallbackFiles().Prompt, opts.Prompt)
			assert.Equal(t, provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(fmt.Sprintf(`{"request":%q}`, marker))}, opts.ProviderOptions["vendor"])
			if id == "backend-primary" {
				return hostileFallbackError(503)
			}
			return nil
		}
		return &observabilityTestModel{
			generate: func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				if err := invoke(ctx, opts); err != nil {
					return nil, err
				}
				return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			},
			stream: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				if err := invoke(ctx, opts); err != nil {
					return nil, err
				}
				return fallbackParts(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}), nil
			},
		}
	}, factory, sink)
	require.NoError(t, err)
	resolved, err := created.ResolveModel(t.Context(), "alias")
	require.NoError(t, err)
	var wg sync.WaitGroup
	for index := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			marker := fmt.Sprintf("request-%d", index)
			state := &telemetryState{}
			state.observation.Store(&requestObservation{correlationID: marker})
			ctx := context.WithValue(t.Context(), telemetryStateKey{}, state)
			opts := mappedFallbackFiles()
			opts.Headers = map[string]string{"x-request": marker}
			opts.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(fmt.Sprintf(`{"request":%q}`, marker))}}
			original, err := json.Marshal(opts)
			require.NoError(t, err)
			if index%2 == 0 {
				result, err := resolved.Model.DoStream(ctx, opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := resolved.Model.DoGenerate(ctx, opts)
				require.NoError(t, err)
			}
			after, err := json.Marshal(opts)
			require.NoError(t, err)
			assert.Equal(t, original, after)
		}()
	}
	wg.Wait()
	sink.Close()
	for _, mode := range []string{"generate", "stream"} {
		assert.Equal(t, count/2, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".start"))
		assert.Equal(t, count/2, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".finish"))
	}
	decisions := make(map[string][]physicalAttemptRecord)
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		decisions[record.CorrelationID] = append(decisions[record.CorrelationID], record)
	}
	require.Len(t, decisions, count)
	for index := range count {
		records := decisions[fmt.Sprintf("request-%d", index)]
		require.Len(t, records, 2)
		assert.Equal(t, 1, records[0].CandidateIndex)
		assert.True(t, records[0].WillFallback)
		assert.Equal(t, 2, records[1].CandidateIndex)
		assert.True(t, records[1].Winner)
	}
}

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
	catalog, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ nativemodel.Config, id string, _ *http.Client) provider.LanguageModel {
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
