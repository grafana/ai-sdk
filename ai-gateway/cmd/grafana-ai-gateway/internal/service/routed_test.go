package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// routedCall runs one configured unary call and returns its overview.
func routedCall(t *testing.T, selector execution.RequestSelector, modelID, gateway string, options provider.CallOptions) (*execution.Overview, error) {
	t.Helper()
	runtime, err := execution.New(execution.Config{Selector: selector, Limits: serviceTestLimits()})
	require.NoError(t, err)
	if gateway != "" {
		if options.ProviderOptions == nil {
			options.ProviderOptions = provider.ProviderOptions{}
		}
		options.ProviderOptions[execution.ControlNamespace] = provider.RawProviderOption{Key: execution.ControlNamespace, Raw: json.RawMessage(gateway)}
	}
	options.Prompt = []provider.Message{provider.UserText("route me")}
	call, err := runtime.Call(callerContext(t, gatewayauth.ConfiguredAccounts), modelID, options)
	if err != nil {
		return nil, err
	}
	_, err = call.Generate()
	return call.View(err).Overview, err
}

func hosts(requests []nativeRequest) []string {
	out := make([]string, len(requests))
	for i, request := range requests {
		out[i] = request.host
	}
	return out
}

func TestRoutedSelection_Configured(t *testing.T) {
	claude := execution.Attempt{Provider: "anthropic", ModelID: "claude", ProviderInstance: "anthropic-primary"}
	gpt := execution.Attempt{Provider: "openai", ModelID: "gpt", ProviderInstance: "openai-backup"}
	local := execution.Attempt{Provider: "my-vllm.chat", ModelID: "local", ProviderInstance: "local"}
	with := func(attempt execution.Attempt, outcome fallback.AttemptOutcome) execution.Attempt {
		attempt.Outcome = outcome
		return attempt
	}
	for _, tc := range []struct {
		name      string
		modelID   string
		gateway   string
		failing   []string
		hosts     []string
		attempts  []execution.Attempt
		canonical string
	}{
		{
			name: "fallback model after the requested route", modelID: "grafana/claude", gateway: `{"models":["grafana/local"]}`,
			failing: []string{translationAnthropicHost}, hosts: []string{translationAnthropicHost, translationCompatHost},
			attempts: []execution.Attempt{with(claude, fallback.AttemptFailed), with(local, fallback.AttemptSelected)}, canonical: "grafana/claude",
		},
		{
			name: "requested model succeeds", modelID: "grafana/claude", gateway: `{"models":["grafana/local"]}`,
			hosts: []string{translationAnthropicHost}, attempts: []execution.Attempt{with(claude, fallback.AttemptSelected)}, canonical: "grafana/claude",
		},
		{
			name: "order prefers a provider", modelID: "grafana/chain", gateway: `{"order":["openai"]}`,
			hosts: []string{translationOpenAIHost}, attempts: []execution.Attempt{with(gpt, fallback.AttemptSelected)}, canonical: "grafana/chain",
		},
		{
			name: "order keeps the rest", modelID: "grafana/chain", gateway: `{"order":["openai"]}`, failing: []string{translationOpenAIHost},
			hosts: []string{translationOpenAIHost, translationAnthropicHost}, attempts: []execution.Attempt{with(gpt, fallback.AttemptFailed), with(claude, fallback.AttemptSelected)}, canonical: "grafana/chain",
		},
		{
			name: "only filters", modelID: "grafana/chain", gateway: `{"only":["openai"]}`, failing: []string{translationOpenAIHost},
			hosts: []string{translationOpenAIHost}, attempts: []execution.Attempt{with(gpt, fallback.AttemptFailed)}, canonical: "grafana/chain",
		},
		{
			name: "only skips the requested route", modelID: "grafana/claude", gateway: `{"models":["grafana/local"],"only":["my-vllm.chat"]}`,
			hosts: []string{translationCompatHost}, attempts: []execution.Attempt{with(local, fallback.AttemptSelected)}, canonical: "grafana/claude",
		},
		{
			name: "instance name is not a slug", modelID: "grafana/chain", gateway: `{"models":["grafana/local"],"only":["openai-backup","my-vllm.chat"]}`,
			hosts: []string{translationCompatHost}, attempts: []execution.Attempt{with(local, fallback.AttemptSelected)}, canonical: "grafana/chain",
		},
		{
			name: "shared destination is attempted once", modelID: "grafana/chain", gateway: `{"models":["grafana/claude","grafana/chain","grafana/local"]}`,
			failing: []string{translationAnthropicHost, translationOpenAIHost}, hosts: []string{translationAnthropicHost, translationOpenAIHost, translationCompatHost},
			attempts: []execution.Attempt{with(claude, fallback.AttemptFailed), with(gpt, fallback.AttemptFailed), with(local, fallback.AttemptSelected)}, canonical: "grafana/chain",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failing := map[string]bool{}
			for _, host := range tc.failing {
				failing[host] = true
			}
			server := &nativeServer{failing: failing}
			overview, _ := routedCall(t, NewSelector(translationCatalog(t, server.client()), nil), tc.modelID, tc.gateway, provider.CallOptions{})
			assert.Equal(t, tc.hosts, hosts(server.requests()))
			require.NotNil(t, overview)
			assert.Equal(t, tc.modelID, overview.RequestedModelID)
			assert.Equal(t, tc.canonical, overview.CanonicalModelID)
			for i := range overview.Attempts {
				overview.Attempts[i].Error = nil
			}
			assert.Equal(t, tc.attempts, overview.Attempts)
			for _, request := range server.requests() {
				assert.NotContains(t, request.body, "grafana/local")
				assert.NotContains(t, request.body, `"only"`)
			}
		})
	}
}

func TestRoutedSelection_ConfiguredRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, modelID, gateway string
		want                   error
	}{
		{name: "backend model ID as fallback", modelID: "grafana/claude", gateway: `{"models":["claude"]}`, want: execution.ErrUnknownFallbackModel},
		{name: "provider instance as fallback", modelID: "grafana/claude", gateway: `{"models":["openai-backup"]}`, want: execution.ErrUnknownFallbackModel},
		{name: "BYOK selector from a configured caller", modelID: "grafana/claude", gateway: `{"models":["openai/gpt"]}`, want: execution.ErrUnknownFallbackModel},
		{name: "unknown requested model precedes unknown fallback", modelID: "missing", gateway: `{"models":["also-missing"]}`, want: catalog.ErrUnknownModel},
		{name: "unknown fallback precedes no candidates", modelID: "grafana/claude", gateway: `{"models":["missing"],"only":[]}`, want: execution.ErrUnknownFallbackModel},
		{name: "only excludes every candidate", modelID: "grafana/chain", gateway: `{"only":["vertex"]}`, want: execution.ErrNoRoutingCandidates},
		{name: "empty only", modelID: "grafana/chain", gateway: `{"only":[]}`, want: execution.ErrNoRoutingCandidates},
		{name: "malformed controls", modelID: "grafana/chain", gateway: `{"order":"openai"}`, want: execution.ErrRoutingControls},
		{name: "credential control", modelID: "grafana/chain", gateway: `{"byok":{"openai":[{"apiKey":"dummy-secret"}]}}`, want: execution.ErrUnsupportedGatewayControl},
		{name: "provider timeouts", modelID: "grafana/chain", gateway: `{"providerTimeouts":{"byok":{"openai":1000}}}`, want: execution.ErrUnsupportedGatewayControl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &nativeServer{}
			overview, err := routedCall(t, NewSelector(translationCatalog(t, server.client()), nil), tc.modelID, tc.gateway, provider.CallOptions{})
			require.ErrorIs(t, err, tc.want)
			assert.Nil(t, overview)
			assert.Empty(t, server.requests())
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}

func TestRoutedSelection_AttemptBound(t *testing.T) {
	file := config.File{Providers: map[string]config.Provider{"local": {Type: "openai-compatible"}}, Models: map[string]config.Model{}}
	var fallbacks []string
	for i := range config.MaxProviderAttempts + 1 {
		id := fmt.Sprintf("route-%d", i)
		file.Models[id] = config.Model{Name: id, Primary: config.Primary{Provider: "local", Model: id}}
		if i > 0 {
			fallbacks = append(fallbacks, fmt.Sprintf("%q", id))
		}
	}
	providers := map[string]config.ResolvedProvider{"local": {Type: "openai-compatible", APIKey: "configured-local", BaseURL: "https://" + translationCompatHost + "/v1"}}
	for _, tc := range []struct {
		count int
		want  error
	}{{config.MaxProviderAttempts, nil}, {config.MaxProviderAttempts + 1, execution.ErrRoutingAttemptBound}} {
		t.Run(fmt.Sprint(tc.count), func(t *testing.T) {
			server := &nativeServer{failing: map[string]bool{translationCompatHost: true}}
			created, err := BuildCatalog(file, providers, server.client(), identityModelFactory)
			require.NoError(t, err)
			gateway := fmt.Sprintf(`{"models":[%s]}`, strings.Join(fallbacks[:tc.count-1], ","))
			overview, err := routedCall(t, NewSelector(created, nil), "route-0", gateway, provider.CallOptions{})
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				assert.Empty(t, server.requests())
				return
			}
			require.Error(t, err)
			assert.Len(t, server.requests(), tc.count)
			require.NotNil(t, overview)
			assert.Len(t, overview.Attempts, tc.count)
		})
	}
}

func TestRoutedSelection_CandidateNativeOptions(t *testing.T) {
	server := &nativeServer{failing: map[string]bool{translationAnthropicHost: true, translationOpenAIHost: true}}
	options := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1024}}`)},
		"openai":    provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"user":"openai-only"}`)},
		"my-vllm":   provider.RawProviderOption{Key: "my-vllm", Raw: json.RawMessage(`{"compatMarker":"compatible-only"}`)},
	}}
	_, err := routedCall(t, NewSelector(translationCatalog(t, server.client()), nil), "grafana/chain", `{"models":["grafana/local"]}`, options)
	require.NoError(t, err)
	requests := server.requests()
	require.Equal(t, []string{translationAnthropicHost, translationOpenAIHost, translationCompatHost}, hosts(requests))
	assert.Contains(t, requests[0].body, `"budget_tokens":1024`)
	assert.NotContains(t, requests[0].body, "compatible-only")
	assert.NotContains(t, requests[0].body, "openai-only")
	assert.Contains(t, requests[1].body, `"user":"openai-only"`)
	assert.NotContains(t, requests[1].body, "budget_tokens")
	assert.NotContains(t, requests[1].body, "compatible-only")
	assert.Contains(t, requests[2].body, "compatible-only")
	assert.NotContains(t, requests[2].body, "budget_tokens")
	assert.NotContains(t, requests[2].body, "openai-only")
	for i, key := range []string{"configured-anthropic", "configured-openai", "configured-local"} {
		assert.Equal(t, key, requests[i].key)
	}
}

func TestRoutedSelection_SharesStartupCandidates(t *testing.T) {
	var constructed atomic.Int32
	file := config.File{
		Providers: map[string]config.Provider{"anthropic-primary": {Type: "anthropic"}},
		Models: map[string]config.Model{
			"first":  {Name: "First", Primary: config.Primary{Provider: "anthropic-primary", Model: "a"}},
			"second": {Name: "Second", Primary: config.Primary{Provider: "anthropic-primary", Model: "b"}},
		},
	}
	providers := map[string]config.ResolvedProvider{"anthropic-primary": {Type: "anthropic", APIKey: "configured-anthropic"}}
	byModel := map[string]*atomic.Int32{}
	created, err := buildCatalog(file, providers, http.DefaultClient, func(_ nativemodel.Config, modelID string, _ *http.Client) provider.LanguageModel {
		constructed.Add(1)
		count := &atomic.Int32{}
		byModel[modelID] = count
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			count.Add(1)
			if modelID == "a" {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "busy", StatusCode: http.StatusServiceUnavailable, IsRetryable: new(true)})
			}
			return &provider.GenerateResult{}, nil
		}}
	}, identityModelFactory)
	require.NoError(t, err)
	require.Equal(t, int32(2), constructed.Load())
	selector := NewSelector(created, nil)

	_, err = routedCall(t, selector, "second", "", provider.CallOptions{})
	require.NoError(t, err)
	_, err = routedCall(t, selector, "first", `{"models":["second"]}`, provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, int32(2), constructed.Load())
	assert.Equal(t, int32(1), byModel["a"].Load())
	assert.Equal(t, int32(2), byModel["b"].Load())
}

func routedFakeCatalog(t *testing.T, factory ModelFactory, sink *PhysicalAttemptSink, models map[string]*observabilityTestModel) *Catalog {
	t.Helper()
	file := config.File{Models: map[string]config.Model{
		"public": {Name: "Public", Aliases: []string{"alias"}, Primary: config.Primary{Provider: "primary-instance", Model: "backend-primary"}},
		"other":  {Name: "Other", Primary: config.Primary{Provider: "secondary-instance", Model: "backend-secondary"}},
	}}
	created, err := buildCatalog(file, fallbackProviders(), http.DefaultClient, func(_ nativemodel.Config, id string, _ *http.Client) provider.LanguageModel {
		return models[id]
	}, factory, sink)
	require.NoError(t, err)
	return created
}

func routedGateway(raw string) provider.CallOptions {
	return provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("route me")},
		ProviderOptions: provider.ProviderOptions{execution.ControlNamespace: provider.RawProviderOption{Key: execution.ControlNamespace, Raw: json.RawMessage(raw)}},
	}
}

func TestRoutedSelection_OneLogicalObservation(t *testing.T) {
	var logs, output lockedBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	factory, _, err := NewModelObservabilityFactories(telemetry, logger, nil, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(hclogLogger(&output), telemetry, 8)
	created := routedFakeCatalog(t, factory, sink, map[string]*observabilityTestModel{
		"backend-primary": {generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, hostileFallbackError(503)
		}},
		"backend-secondary": {generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}},
	})
	runtime, err := execution.New(execution.Config{Selector: NewSelector(created, nil), Limits: serviceTestLimits()})
	require.NoError(t, err)
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "routed-request"})
	ctx := context.WithValue(callerContext(t, gatewayauth.ConfiguredAccounts), telemetryStateKey{}, state)
	call, err := runtime.Call(ctx, "alias", routedGateway(`{"models":["other"]}`))
	require.NoError(t, err)
	_, err = call.Generate()
	require.NoError(t, err)
	overview := call.View(nil).Overview
	sink.Close(context.Background())

	require.NotNil(t, overview)
	assert.Equal(t, "public", overview.CanonicalModelID)
	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.generate.start"))
	assert.Equal(t, 1, countModelLogEvent(t, logs.String(), "aisdk.model.generate.finish"))
	assert.NotContains(t, logs.String(), "backend-secondary")
	var records []physicalAttemptRecord
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	require.Len(t, records, 2)
	assert.Equal(t, physicalAttemptRecord{CorrelationID: "routed-request", CandidateIndex: 1, ProviderInstance: "primary-instance", BackendModelID: "backend-primary", Outcome: fallback.AttemptFailed, WillFallback: true}, withoutTimes(records[0]))
	assert.Equal(t, physicalAttemptRecord{CorrelationID: "routed-request", CandidateIndex: 2, ProviderInstance: "secondary-instance", BackendModelID: "backend-secondary", Outcome: fallback.AttemptSelected, Winner: true}, withoutTimes(records[1]))
}

func withoutTimes(record physicalAttemptRecord) physicalAttemptRecord {
	record.StartedAt, record.DecidedAt = time.Time{}, time.Time{}
	return record
}

func TestRoutedSelection_CommitmentAndCancellation(t *testing.T) {
	t.Run("committed stream error", func(t *testing.T) {
		var secondary atomic.Int32
		created := routedFakeCatalog(t, identityModelFactory, nil, map[string]*observabilityTestModel{
			"backend-primary": {stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return fallbackParts(
					provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)},
					provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}, Usage: &provider.Usage{}},
				), nil
			}},
			"backend-secondary": {stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				secondary.Add(1)
				return fallbackParts(), nil
			}},
		})
		selected, err := NewSelector(created, nil)(callerContext(t, gatewayauth.ConfiguredAccounts), "public", provider.CallOptions{}, json.RawMessage(`{"models":["other"]}`))
		require.NoError(t, err)
		result, err := selected.Model.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("route me")}})
		require.NoError(t, err)
		var parts []provider.StreamPartType
		for part := range result.Stream {
			parts = append(parts, part.Type)
		}
		assert.Equal(t, []provider.StreamPartType{provider.PartError, provider.PartFinish}, parts)
		assert.Zero(t, secondary.Load())
	})
	t.Run("cancellation stops the chain", func(t *testing.T) {
		var secondary atomic.Int32
		started := make(chan struct{})
		created := routedFakeCatalog(t, identityModelFactory, nil, map[string]*observabilityTestModel{
			"backend-primary": {generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}},
			"backend-secondary": {generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				secondary.Add(1)
				return &provider.GenerateResult{}, nil
			}},
		})
		selected, err := NewSelector(created, nil)(callerContext(t, gatewayauth.ConfiguredAccounts), "public", provider.CallOptions{}, json.RawMessage(`{"models":["other"]}`))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			<-started
			cancel()
		}()
		_, err = selected.Model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("route me")}})
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, secondary.Load())
	})
}
