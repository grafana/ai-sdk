package prometheus

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	aimiddleware "github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockModel struct {
	providerName string
	modelID      string
	generateFunc func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error)
	streamFunc   func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
}

func (m *mockModel) SpecificationVersion() string { return "v4" }
func (m *mockModel) Provider() string             { return m.providerName }
func (m *mockModel) ModelID() string              { return m.modelID }
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (m *mockModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	return m.generateFunc(ctx, opts)
}
func (m *mockModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	return m.streamFunc(ctx, opts)
}

type mockProvider struct{ model provider.LanguageModel }

func (p mockProvider) LanguageModel(string) (provider.LanguageModel, error) { return p.model, nil }

func TestMiddleware_APIAndRegistration(t *testing.T) {
	reg := promclient.NewRegistry()
	mw, err := Middleware(Options{
		Registerer:  reg,
		ConstLabels: promclient.Labels{"service": "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, mw.WrapGenerate)
	require.NotNil(t, mw.WrapStream)

	model := &mockModel{providerName: "test", modelID: "model"}
	model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}
	wrappedWithConstLabels := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
	_, err = wrappedWithConstLabels.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "service": "test"}, 1)

	_, err = Middleware(Options{Registerer: reg, ConstLabels: promclient.Labels{"service": "test"}})
	require.Error(t, err)

	assert.Panics(t, func() { MustMiddleware(Options{Registerer: reg, ConstLabels: promclient.Labels{"service": "test"}}) })

	wrapped, err := Wrap(model, Options{Registerer: promclient.NewRegistry()})
	require.NoError(t, err)
	assert.Implements(t, (*provider.LanguageModel)(nil), wrapped)

	dupReg := promclient.NewRegistry()
	_, err = Middleware(Options{Registerer: dupReg})
	require.NoError(t, err)
	assert.Panics(t, func() { MustWrap(model, Options{Registerer: dupReg}) })
}

func TestMiddleware_DefaultRegisterer(t *testing.T) {
	oldRegisterer := promclient.DefaultRegisterer
	reg := promclient.NewRegistry()
	promclient.DefaultRegisterer = reg
	t.Cleanup(func() { promclient.DefaultRegisterer = oldRegisterer })

	_, err := Middleware(Options{})
	require.NoError(t, err)
	_, err = Middleware(Options{})
	require.Error(t, err)
	count, err := testutil.GatherAndCount(reg, metricRequestsTotal)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestMiddleware_BucketOverridesAndDisabledStreamCollectors(t *testing.T) {
	t.Run("default duration buckets include long LLM calls", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)

		model := &mockModel{providerName: "test", modelID: "model"}
		model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		_, err = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		duration := findMetric(t, reg, metricRequestDurationSeconds, map[string]string{"operation": operationGenerate})
		assertHistogramBuckets(t, duration, []float64{60, 120, 300})
	})

	t.Run("duration and TTFT overrides with disabled chunk collectors", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{
			Registerer:                reg,
			DurationBuckets:           []float64{0.001},
			TimeToFirstOutputBuckets:  []float64{0.002},
			InterChunkDelayBuckets:    []float64{0.003},
			DisableStreamChunkMetrics: true,
		})
		require.NoError(t, err)

		model := &mockModel{providerName: "test", modelID: "model"}
		model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return streamResult(provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}, finishPart(provider.FinishReasonStop, usage(1, 1))), nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		_, err = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		stream, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		drain(stream.Stream)

		duration := findMetric(t, reg, metricRequestDurationSeconds, map[string]string{"operation": operationGenerate})
		assertHistogramBuckets(t, duration, []float64{0.001})
		ttft := findMetric(t, reg, metricTimeToFirstOutputSeconds, map[string]string{"operation": operationStream})
		assertHistogramBuckets(t, ttft, []float64{0.002})

		families, err := reg.Gather()
		require.NoError(t, err)
		assert.Nil(t, findFamily(families, metricStreamChunksTotal))
		assert.Nil(t, findFamily(families, metricInterChunkDelaySeconds))
		count, err := testutil.GatherAndCount(reg, metricStreamChunksTotal)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("inter chunk override", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg, InterChunkDelayBuckets: []float64{0.003}})
		require.NoError(t, err)
		model := &mockModel{providerName: "test", modelID: "model"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return streamResult(provider.StreamPart{Type: provider.PartTextDelta, Delta: "one"}, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call"}, finishPart(provider.FinishReasonStop, usage(1, 1))), nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		stream, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		drain(stream.Stream)

		interChunk := findMetric(t, reg, metricInterChunkDelaySeconds, map[string]string{"operation": operationStream, "chunk_type": string(provider.PartToolCall)})
		assertHistogramBuckets(t, interChunk, []float64{0.003})
	})
}

func TestRootDependencyIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("dependency graph check is not a short test")
	}
	rootGoMod, err := os.ReadFile("../../go.mod")
	require.NoError(t, err)
	assert.NotContains(t, string(rootGoMod), "github.com/prometheus/client_golang")

	cmd := exec.Command("go", "list", "-deps", "./...")
	cmd.Dir = "../.."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.NotContains(t, string(out), "github.com/prometheus/client_golang")
}

func TestGenerateMetrics(t *testing.T) {
	reg := promclient.NewRegistry()
	mw, err := Middleware(Options{Registerer: reg})
	require.NoError(t, err)

	expected := &provider.GenerateResult{
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        usage(7, 11),
		Response:     &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Provider: "anthropic", ModelID: "claude-3"}},
	}
	model := &mockModel{providerName: "grafana", modelID: "router"}
	model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return expected, nil }
	wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

	got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{Headers: map[string]string{"Authorization": "secret-header"}})
	require.NoError(t, err)
	require.Same(t, expected, got)

	assertCounter(t, reg, metricRequestsTotal, map[string]string{
		"operation": operationGenerate, "provider": "anthropic", "model": "claude-3", "status": statusSuccess,
		"error_type": errorTypeNone, "status_code": statusCodeNone, "finish_reason": string(provider.FinishReasonStop),
	}, 1)
	duration := assertHistogramCount(t, reg, metricRequestDurationSeconds, map[string]string{"operation": operationGenerate, "provider": "anthropic", "model": "claude-3", "status": statusSuccess})
	assertMetricMissingLabelNames(t, duration, "error_type", "status_code", "finish_reason")
	assertCounter(t, reg, metricTokensTotal, map[string]string{"operation": operationGenerate, "provider": "anthropic", "model": "claude-3", "token_type": tokenTypeInput}, 7)
	assertCounter(t, reg, metricTokensTotal, map[string]string{"operation": operationGenerate, "provider": "anthropic", "model": "claude-3", "token_type": tokenTypeOutput}, 11)
	assertInflightGauge(t, reg, map[string]string{"operation": operationGenerate, "provider": "grafana", "model": "router"})
	assertNoLabelContains(t, reg, "secret-header")
}

func TestGenerateErrorsIdentityAndNormalizers(t *testing.T) {
	t.Run("api call error", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "secret error message", URL: "https://secret.example", StatusCode: 429, ResponseBody: "secret body"})
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, apiErr }
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

		_, err = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.ErrorIs(t, err, apiErr)
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "provider": "openai", "model": "gpt", "status": statusError, "error_type": errorTypeAPICallError, "status_code": "429"}, 1)
		duration := assertHistogramCount(t, reg, metricRequestDurationSeconds, map[string]string{"operation": operationGenerate, "provider": "openai", "model": "gpt", "status": statusError})
		assertMetricMissingLabelNames(t, duration, "error_type", "status_code", "finish_reason")
		assertNoLabelContains(t, reg, "secret")
	})

	t.Run("context cancellation", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.generateFunc = func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, ctx.Err()
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = wrapped.DoGenerate(ctx, provider.CallOptions{})
		require.ErrorIs(t, err, context.Canceled)
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "status": statusCanceled, "error_type": errorTypeContextCanceled}, 1)
	})

	t.Run("context deadline", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.generateFunc = func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, ctx.Err()
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		_, err = wrapped.DoGenerate(ctx, provider.CallOptions{})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "status": statusCanceled, "error_type": errorTypeContextDeadlineExceeded}, 1)
	})

	t.Run("requested identity and normalizers", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{
			Registerer:     reg,
			IdentitySource: IdentityRequested,
			NormalizeProvider: func(string) string {
				return "bucket-provider"
			},
			NormalizeModel: func(providerName, modelID string) string {
				return providerName + ":bucket-model"
			},
		})
		require.NoError(t, err)
		model := &mockModel{providerName: "grafana", modelID: "router"}
		model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Provider: "anthropic", ModelID: "claude"}}}, nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		_, err = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "provider": "bucket-provider", "model": "bucket-provider:bucket-model"}, 1)
	})
}

func TestStreamMetrics(t *testing.T) {
	reg := promclient.NewRegistry()
	mw, err := Middleware(Options{Registerer: reg})
	require.NoError(t, err)

	finish := provider.FinishReason{Unified: provider.FinishReasonLength}
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, Provider: "anthropic", ModelID: "claude-3", ResponseID: "secret-response-id"},
		{Type: provider.PartTextDelta, Delta: "secret-output"},
		{Type: provider.PartToolCall, ToolCallID: "secret-tool-call", ToolName: "secret-tool", Input: "secret-input"},
		{Type: provider.PartFinish, FinishReason: &finish, Usage: usagePtr(5, 8)},
	}
	model := &mockModel{providerName: "grafana", modelID: "router"}
	model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return streamResult(parts...), nil
	}
	wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	got := drain(result.Stream)
	assert.Equal(t, parts, got)

	assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "status": statusSuccess, "finish_reason": string(provider.FinishReasonLength)}, 1)
	assertHistogramCount(t, reg, metricRequestDurationSeconds, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "status": statusSuccess})
	assertHistogramCount(t, reg, metricTimeToFirstOutputSeconds, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "status": statusSuccess})
	assertCounter(t, reg, metricTokensTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "token_type": tokenTypeInput}, 5)
	assertCounter(t, reg, metricTokensTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "token_type": tokenTypeOutput}, 8)
	assertCounter(t, reg, metricStreamChunksTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "chunk_type": string(provider.PartResponseMeta)}, 1)
	assertCounter(t, reg, metricStreamChunksTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "chunk_type": string(provider.PartTextDelta)}, 1)
	assertCounter(t, reg, metricStreamChunksTotal, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "chunk_type": string(provider.PartFinish)}, 1)
	assertHistogramCount(t, reg, metricInterChunkDelaySeconds, map[string]string{"operation": operationStream, "provider": "anthropic", "model": "claude-3", "chunk_type": string(provider.PartToolCall)})
	assertInflightGauge(t, reg, map[string]string{"operation": operationStream, "provider": "grafana", "model": "router"})
	assertNoLabelContains(t, reg, "secret")
}

func TestStreamUsageAggregatesEveryPart(t *testing.T) {
	reg := promclient.NewRegistry()
	mw, err := Middleware(Options{Registerer: reg})
	require.NoError(t, err)

	inputTotal, inputNoCache, cacheRead, cacheWrite := 120, 80, 30, 10
	outputTotal, outputText, outputReasoning := 50, 30, 20
	provisionalInput, provisionalCacheRead, provisionalCacheWrite := 100, 20, 5
	provisionalOutput, provisionalText, provisionalReasoning := 45, 25, 15
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{
			Total: &inputTotal, NoCache: &inputNoCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite,
		}}},
		{Type: provider.PartTextDelta, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{
			Total: &outputTotal, Text: &outputText, Reasoning: &outputReasoning,
		}}},
		{Type: provider.PartFinish, Usage: &provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total: &provisionalInput, CacheRead: &provisionalCacheRead, CacheWrite: &provisionalCacheWrite,
			},
			OutputTokens: provider.OutputTokenUsage{
				Total: &provisionalOutput, Text: &provisionalText, Reasoning: &provisionalReasoning,
			},
		}},
	}
	model := &mockModel{providerName: "test", modelID: "model"}
	model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return streamResult(parts...), nil
	}
	wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	drain(result.Stream)

	want := map[string]int{
		tokenTypeInput:           inputTotal,
		tokenTypeInputNoCache:    inputNoCache,
		tokenTypeInputCacheRead:  cacheRead,
		tokenTypeInputCacheWrite: cacheWrite,
		tokenTypeOutput:          outputTotal,
		tokenTypeOutputText:      outputText,
		tokenTypeOutputReasoning: outputReasoning,
	}
	for tokenType, count := range want {
		assertCounter(t, reg, metricTokensTotal, map[string]string{
			"operation": operationStream, "provider": "test", "model": "model", "token_type": tokenType,
		}, float64(count))
	}
}

func TestStreamErrorPaths(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		upstream := make(chan provider.StreamPart)
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		ctx, cancel := context.WithCancel(context.Background())
		result, err := wrapped.DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)
		cancel()

		producerDone := make(chan struct{})
		go func() {
			defer close(producerDone)
			upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "after-cancel"}
			close(upstream)
		}()

		drain(result.Stream)
		select {
		case <-producerDone:
		case <-time.After(time.Second):
			t.Fatal("upstream producer was not drained after cancellation")
		}
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "status": statusCanceled, "error_type": errorTypeContextCanceled}, 1)
		assertInflightGauge(t, reg, map[string]string{"operation": operationStream, "provider": "openai", "model": "gpt"})
	})

	t.Run("cancellation preserves usage from a received part", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		upstream := make(chan provider.StreamPart)
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
		ctx, cancel := context.WithCancel(context.Background())
		result, err := wrapped.DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)

		inputTokens := 9
		usageReceived := make(chan struct{})
		go func() {
			for range streamBufferSize {
				upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}
			}
			upstream <- provider.StreamPart{Type: provider.PartResponseMeta, Usage: &provider.Usage{
				InputTokens: provider.InputTokenUsage{Total: &inputTokens},
			}}
			close(usageReceived)
			close(upstream)
		}()

		select {
		case <-usageReceived:
		case <-time.After(time.Second):
			t.Fatal("middleware did not receive usage part")
		}
		cancel()

		require.Eventually(t, func() bool {
			families, gatherErr := reg.Gather()
			if gatherErr != nil {
				return false
			}
			family := findFamily(families, metricTokensTotal)
			if family == nil {
				return false
			}
			for _, metric := range family.GetMetric() {
				if metricHasLabels(metric, map[string]string{"operation": operationStream, "token_type": tokenTypeInput}) {
					return metric.GetCounter().GetValue() == float64(inputTokens)
				}
			}
			return false
		}, time.Second, 10*time.Millisecond)
		drain(result.Stream)
	})

	t.Run("nil stream result", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, nil }
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Nil(t, result)
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "provider": "openai", "model": "gpt", "status": statusSuccess}, 1)
		assertInflightGauge(t, reg, map[string]string{"operation": operationStream, "provider": "openai", "model": "gpt"})
	})

	t.Run("initial stream error", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		sentinel := errors.New("secret open error")
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, sentinel }
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

		_, err = wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.ErrorIs(t, err, sentinel)
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "provider": "openai", "model": "gpt", "status": statusError, "error_type": errorTypeOther}, 1)
		assertNoLabelContains(t, reg, "secret")
	})

	t.Run("stream part error", func(t *testing.T) {
		reg := promclient.NewRegistry()
		mw, err := Middleware(Options{Registerer: reg})
		require.NoError(t, err)
		apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "secret rate limit", StatusCode: 429})
		parts := []provider.StreamPart{{Type: provider.PartTextDelta, Delta: "hello"}, {Type: provider.PartError, APICallError: apiErr}}
		model := &mockModel{providerName: "openai", modelID: "gpt"}
		model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return streamResult(parts...), nil
		}
		wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})

		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, parts, drain(result.Stream))
		assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "status": statusError, "error_type": errorTypeAPICallError, "status_code": "429"}, 1)
	})
}

func TestRegistryIntegration(t *testing.T) {
	reg := promclient.NewRegistry()
	mw, err := Middleware(Options{Registerer: reg})
	require.NoError(t, err)
	model := &mockModel{providerName: "openai", modelID: "gpt"}
	model.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}
	models := registry.NewProviderRegistry(map[string]registry.Provider{"mock": mockProvider{model: model}}, registry.WithLanguageModelMiddleware(mw))
	resolved, err := models.LanguageModel("mock:gpt")
	require.NoError(t, err)

	_, err = resolved.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationGenerate, "provider": "openai", "model": "gpt", "status": statusSuccess}, 1)
}

func streamResult(parts ...provider.StreamPart) *provider.StreamResult {
	ch := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		ch <- part
	}
	close(ch)
	return &provider.StreamResult{Stream: ch, Request: &provider.RequestMetadata{}, Response: &provider.ResponseHeaders{}}
}

func finishPart(reason provider.UnifiedFinishReason, usage provider.Usage) provider.StreamPart {
	finishReason := provider.FinishReason{Unified: reason}
	return provider.StreamPart{Type: provider.PartFinish, FinishReason: &finishReason, Usage: &usage}
}

func drain(ch <-chan provider.StreamPart) []provider.StreamPart {
	parts := []provider.StreamPart{}
	for part := range ch {
		parts = append(parts, part)
	}
	return parts
}

func usage(input, output int) provider.Usage {
	return provider.Usage{InputTokens: provider.InputTokenUsage{Total: &input}, OutputTokens: provider.OutputTokenUsage{Total: &output}}
}

func usagePtr(input, output int) *provider.Usage {
	usage := usage(input, output)
	return &usage
}

func assertCounter(t *testing.T, reg *promclient.Registry, name string, labels map[string]string, expected float64) {
	t.Helper()
	metric := findMetric(t, reg, name, labels)
	require.NotNil(t, metric.GetCounter(), "metric %s is not a counter", name)
	assert.Equal(t, expected, metric.GetCounter().GetValue())
}

func assertInflightGauge(t *testing.T, reg *promclient.Registry, labels map[string]string) {
	t.Helper()
	metric := findMetric(t, reg, metricInflightRequests, labels)
	require.NotNil(t, metric.GetGauge(), "metric %s is not a gauge", metricInflightRequests)
	assert.Equal(t, float64(0), metric.GetGauge().GetValue())
}

func assertHistogramCount(t *testing.T, reg *promclient.Registry, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	metric := findMetric(t, reg, name, labels)
	require.NotNil(t, metric.GetHistogram(), "metric %s is not a histogram", name)
	assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
	return metric
}

func assertMetricMissingLabelNames(t *testing.T, metric *dto.Metric, names ...string) {
	t.Helper()
	got := map[string]struct{}{}
	for _, label := range metric.GetLabel() {
		got[label.GetName()] = struct{}{}
	}
	for _, name := range names {
		assert.NotContains(t, got, name)
	}
}

func assertHistogramBuckets(t *testing.T, metric *dto.Metric, want []float64) {
	t.Helper()
	require.NotNil(t, metric.GetHistogram())
	got := make([]float64, 0, len(metric.GetHistogram().GetBucket()))
	for _, bucket := range metric.GetHistogram().GetBucket() {
		got = append(got, bucket.GetUpperBound())
	}
	for _, bucket := range want {
		assert.Contains(t, got, bucket)
	}
}

func findMetric(t *testing.T, reg *promclient.Registry, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	family := findFamily(families, name)
	require.NotNil(t, family, "metric family %s not found", name)
	for _, metric := range family.GetMetric() {
		if metricHasLabels(metric, labels) {
			return metric
		}
	}
	require.Failf(t, "metric not found", "family %s missing labels %v", name, labels)
	return nil
}

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	for name, want := range labels {
		found := false
		for _, label := range metric.GetLabel() {
			if label.GetName() == name && label.GetValue() == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func assertNoLabelContains(t *testing.T, reg *promclient.Registry, forbidden string) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.NotContains(t, label.GetValue(), forbidden)
			}
		}
	}
}
