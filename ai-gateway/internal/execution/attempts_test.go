package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticResolver struct {
	resolved catalog.ResolvedModel
	err      error
}

func (r staticResolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return r.resolved, r.err
}

type generateModel struct {
	provider.LanguageModel
	modelID  string
	generate func(context.Context) (*provider.GenerateResult, error)
	stream   func(context.Context) (*provider.StreamResult, error)
	mu       sync.Mutex
	calls    int
}

func (m *generateModel) SpecificationVersion() string { return "v4" }
func (m *generateModel) Provider() string             { return "native" }
func (m *generateModel) ModelID() string              { return m.modelID }
func (m *generateModel) DoGenerate(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.generate != nil {
		return m.generate(ctx)
	}
	return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
}

func (m *generateModel) DoStream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.stream != nil {
		return m.stream(ctx)
	}
	parts := make(chan provider.StreamPart)
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

func (m *generateModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func selectConfigured(t *testing.T, resolved catalog.ResolvedModel) Selection {
	t.Helper()
	selected, err := CatalogSelector(staticResolver{resolved: resolved})(t.Context(), "alias", provider.CallOptions{}, nil)
	require.NoError(t, err)
	return selected
}

func TestAttemptCapture_ConfiguredSelection(t *testing.T) {
	resolved := catalog.ResolvedModel{
		ID:         "canonical",
		Model:      &generateModel{},
		Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "backend", ProviderInstance: "configured"}},
	}
	selected := selectConfigured(t, resolved)
	resolved.Candidates[0].ModelID = "mutated"
	attempts := newAttemptCapture("alias", selected, SummaryRedactor{})
	assert.Equal(t, "canonical", attempts.canonical)
	assert.Equal(t, "backend", attempts.expected[0].Destination.ModelID)
	encoded, err := json.Marshal(selected)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "configured")
	attempts.enter()
	view := attempts.view(nil)
	require.NotNil(t, view.Overview)
	assert.Equal(t, "alias", view.Overview.RequestedModelID)
	assert.Equal(t, "canonical", view.Overview.CanonicalModelID)
	assert.Equal(t, "configured", view.Overview.Attempts[0].ProviderInstance)
	assert.Equal(t, &Failure{Message: "configured-secret"}, view.Current(provider.NewAPICallError(provider.APICallErrorOptions{Message: "configured-secret"})))
}

func TestAttemptCapture_View(t *testing.T) {
	failure := errors.New("candidate failure")
	for _, tc := range []struct {
		name       string
		candidates int
		entered    bool
		attempts   []fallback.Attempt
		err        error
		discard    bool
		outcomes   []fallback.AttemptOutcome
	}{
		{name: "not entered", candidates: 1},
		{name: "direct selected", candidates: 1, entered: true, outcomes: []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{name: "direct failed", candidates: 1, entered: true, err: failure, outcomes: []fallback.AttemptOutcome{fallback.AttemptFailed}},
		{name: "direct invalid output", candidates: 1, entered: true, err: fmt.Errorf("wrapped: %w", ErrInvalidOutput), outcomes: []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{name: "rejected direct canceled", candidates: 1, entered: true, err: context.Canceled, discard: true, outcomes: []fallback.AttemptOutcome{fallback.AttemptCanceled}},
		{name: "rejected before entry", candidates: 1, discard: true},
		{name: "fallback selected", candidates: 2, entered: true, attempts: []fallback.Attempt{
			{Index: 1, Outcome: fallback.AttemptFailed, SourceErr: failure, WillFallback: true},
			{Index: 2, Outcome: fallback.AttemptSelected},
		}, outcomes: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{name: "rejected fallback", candidates: 2, entered: true, attempts: []fallback.Attempt{
			{Index: 1, Outcome: fallback.AttemptCanceled, SourceErr: failure},
		}, err: context.Canceled, discard: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := catalog.ResolvedModel{ID: "canonical"}
			for i := range tc.candidates {
				resolved.Candidates = append(resolved.Candidates, catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
			}
			attempts := newAttemptCapture("alias", selectConfigured(t, resolved), SummaryRedactor{})
			resolved.Candidates[0].ModelID = "mutated"
			if tc.entered {
				attempts.enter()
			}
			for _, attempt := range tc.attempts {
				attempts.observe(t.Context(), attempt)
			}
			if tc.discard {
				attempts.discard()
			}
			view := attempts.view(tc.err)
			if len(tc.outcomes) == 0 {
				assert.Nil(t, view.Overview)
			} else {
				require.NotNil(t, view.Overview)
				require.Len(t, view.Overview.Attempts, len(tc.outcomes))
				for i, outcome := range tc.outcomes {
					assert.Equal(t, outcome, view.Overview.Attempts[i].Outcome)
					assert.Equal(t, fmt.Sprint(i), view.Overview.Attempts[i].ModelID)
				}
			}
			assert.Equal(t, &Failure{Message: "credential"}, view.Current(provider.NewAPICallError(provider.APICallErrorOptions{Message: "credential"})))
			before, err := json.Marshal(view.Overview)
			require.NoError(t, err)
			var late sync.WaitGroup
			for range 8 {
				late.Go(func() {
					attempts.enter()
					attempts.observe(t.Context(), fallback.Attempt{Index: 1, Outcome: fallback.AttemptFailed, SourceErr: errors.New("late failure")})
				})
			}
			late.Wait()
			after, err := json.Marshal(view.Overview)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			entered, captured := attempts.seal()
			assert.Equal(t, tc.entered, entered)
			assert.Empty(t, captured)
			assert.True(t, attempts.sealed)
		})
	}
}

func TestAttemptCapture_RejectRecordedFallbackOutcome(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := &generateModel{modelID: "A", generate: func(context.Context) (*provider.GenerateResult, error) {
		cancel()
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &generateModel{modelID: "B"}
	model, err := fallback.New(first, second)
	require.NoError(t, err)
	attempts := newAttemptCapture("alias", selectConfigured(t, catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "A"}, {Provider: "native", ModelID: "B"}}}), SummaryRedactor{})
	ctx := attempts.withObserver(parent)
	_, err = model.DoGenerate(ctx, provider.CallOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, attempts.attempts, 1)
	require.NotNil(t, attempts.attempts[0].SourceErr)
	attempts.discard()
	view := attempts.view(ctx.Err())
	assert.Nil(t, view.Overview)
	assert.Empty(t, attempts.attempts)
	assert.True(t, attempts.sealed)
	assert.Zero(t, second.callCount())
}

type projectionError struct {
	inspect func()
}

func (err projectionError) Error() string {
	err.inspect()
	return "candidate failure"
}

func TestAttemptCapture_ProjectionAfterSeal(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint(panics), func(t *testing.T) {
			attempts := newAttemptCapture("alias", selectConfigured(t, catalog.ResolvedModel{ID: "canonical", Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "model"}}}), SummaryRedactor{})
			attempts.enter()
			inspected := make(chan struct{})
			failure := projectionError{inspect: func() {
				attempts.observe(t.Context(), fallback.Attempt{Index: 2, Outcome: fallback.AttemptSelected})
				close(inspected)
				if panics {
					panic("projection failed")
				}
			}}
			views := make(chan View, 1)
			go func() { views <- attempts.view(failure) }()
			var view View
			select {
			case view = <-views:
			case <-time.After(time.Second):
				require.FailNow(t, "projection held the capture lock")
			}
			select {
			case <-inspected:
			default:
				require.FailNow(t, "native error was not inspected")
			}
			if panics {
				assert.Nil(t, view.Overview)
			} else {
				require.NotNil(t, view.Overview)
				require.Len(t, view.Overview.Attempts, 1)
				assert.Equal(t, "candidate failure", view.Overview.Attempts[0].Error.Message)
			}
			_, captured := attempts.seal()
			assert.Empty(t, captured)
		})
	}
}

func TestAccountSelection(t *testing.T) {
	expected := accountAttempts("native", "model", 2)
	selected := AccountSelection("native/model", &generateModel{modelID: "model"}, expected)
	expected[0].Destination.ModelID = "mutated"
	assert.True(t, validSelection(selected))
	assert.Equal(t, accountAttempts("native", "model", 2), selected.expected)
	attempts := newAttemptCapture("native/model", selected, SummaryRedactor{})
	assert.Equal(t, attributionAccounts, attempts.attribution)
	assert.Equal(t, accountAttempts("native", "model", 2), attempts.expected)
}

func TestAttemptCapture_AccountProjection(t *testing.T) {
	retryable := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "account overloaded", IsRetryable: new(true)})
	rejected := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "account rejected"})
	type expected struct {
		outcome fallback.AttemptOutcome
		message string
	}
	for _, tc := range []struct {
		name     string
		failures []error
		accounts []ExpectedAttempt
		cancel   bool
		observed []fallback.Attempt
		calls    []int
		want     []expected
	}{
		{name: "direct selected", failures: []error{nil}, calls: []int{1}, want: []expected{{outcome: fallback.AttemptSelected}}},
		{name: "direct failed", failures: []error{rejected}, calls: []int{1}, want: []expected{{fallback.AttemptFailed, "account rejected"}}},
		{name: "failed then selected", failures: []error{retryable, nil}, calls: []int{1, 1}, want: []expected{{fallback.AttemptFailed, "account overloaded"}, {outcome: fallback.AttemptSelected}}},
		{name: "noneligible", failures: []error{rejected, nil}, calls: []int{1, 0}, want: []expected{{fallback.AttemptFailed, "account rejected"}}},
		{name: "exhausted", failures: []error{retryable, retryable}, calls: []int{1, 1}, want: []expected{{fallback.AttemptFailed, "account overloaded"}, {fallback.AttemptFailed, "account overloaded"}}},
		{name: "canceled", failures: []error{retryable, nil}, cancel: true, calls: []int{1, 0}, want: []expected{{fallback.AttemptCanceled, "account overloaded"}}},
		{name: "mismatched provider", failures: []error{retryable, nil}, accounts: accountAttempts("other", "model", 2), calls: []int{1, 1}},
		{name: "mismatched model", failures: []error{retryable, nil}, accounts: accountAttempts("native", "other", 2), calls: []int{1, 1}},
		{name: "account overflow", failures: []error{retryable, nil}, accounts: accountAttempts("native", "model", 1), calls: []int{1, 1}},
		{name: "zero attempts", accounts: accountAttempts("native", "model", 1)},
		{name: "mixed calls", accounts: accountAttempts("native", "model", 2), observed: []fallback.Attempt{
			{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected},
			{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.accounts == nil {
				tc.accounts = accountAttempts("native", "model", len(tc.failures))
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var candidates []*generateModel
			for _, failure := range tc.failures {
				candidates = append(candidates, &generateModel{modelID: "model", generate: func(context.Context) (*provider.GenerateResult, error) {
					if tc.cancel {
						cancel()
					}
					if failure != nil {
						return nil, failure
					}
					return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				}})
			}
			attempts := newAttemptCapture("native/model", AccountSelection("native/model", &generateModel{modelID: "model"}, tc.accounts), SummaryRedactor{})
			attempts.enter()
			var err error
			if len(candidates) != 0 {
				models := make([]provider.LanguageModel, 0, len(candidates))
				for _, candidate := range candidates {
					models = append(models, candidate)
				}
				model, chainErr := fallback.New(models...)
				require.NoError(t, chainErr)
				_, err = model.DoGenerate(attempts.withObserver(ctx), provider.CallOptions{})
			}
			for _, attempt := range tc.observed {
				attempts.observe(ctx, attempt)
			}
			for i, calls := range tc.calls {
				assert.Equal(t, calls, candidates[i].callCount(), "candidate %d", i+1)
			}
			view := attempts.view(err)
			if len(tc.want) == 0 {
				assert.Nil(t, view.Overview)
				return
			}
			require.NotNil(t, view.Overview)
			assert.Equal(t, "native/model", view.Overview.RequestedModelID)
			assert.Empty(t, view.Overview.CanonicalModelID)
			require.Len(t, view.Overview.Attempts, len(tc.want))
			for i, want := range tc.want {
				attempt := view.Overview.Attempts[i]
				assert.Equal(t, Attempt{Provider: "native", ModelID: "model", Outcome: want.outcome, Error: attempt.Error}, attempt)
				if want.message == "" {
					assert.Nil(t, attempt.Error)
				} else {
					require.NotNil(t, attempt.Error)
					assert.Equal(t, want.message, attempt.Error.Message)
				}
			}
			encoded, err := json.Marshal(view.Overview)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "canonicalModelId")
			assert.NotContains(t, string(encoded), "providerInstance")
		})
	}
}

func fallbackMarker(raw string) provider.ProviderMetadata {
	return provider.ProviderMetadata{"anthropic": json.RawMessage(raw)}
}

func TestServedModel(t *testing.T) {
	marker := func(to string) provider.ProviderMetadata {
		return fallbackMarker(`{"type":"fallback","from":{"model":"claude-a"},"to":{"model":"` + to + `"}}`)
	}
	resolved := func(model provider.LanguageModel) catalog.ResolvedModel {
		return catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "anthropic", ModelID: "backend", ProviderInstance: "configured"}}}
	}
	configuredCall := func(t *testing.T, model provider.LanguageModel) *Call {
		t.Helper()
		call, err := newRuntime(t, CatalogSelector(staticResolver{resolved: resolved(model)}), testLimits()).Call(t.Context(), "alias", provider.CallOptions{})
		require.NoError(t, err)
		t.Cleanup(call.Close)
		return call
	}
	custom := func(metadata provider.ProviderMetadata) provider.GenerateContentPart {
		return provider.GenerateContentPart{Type: provider.ContentCustom, Kind: "anthropic.fallback", ProviderMetadata: metadata}
	}
	generated := func(content ...provider.GenerateContentPart) *generateModel {
		return &generateModel{modelID: "backend", generate: func(context.Context) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{Content: content, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}}
	}
	for _, tc := range []struct {
		name    string
		content []provider.GenerateContentPart
		served  string
	}{
		{"latest valid marker", []provider.GenerateContentPart{custom(marker("claude-b")), custom(marker("claude-c"))}, "claude-c"},
		{"no marker", []provider.GenerateContentPart{{Type: provider.ContentText, Text: "answer"}}, ""},
		{"empty target", []provider.GenerateContentPart{custom(marker(""))}, ""},
		{"invalid UTF-8 target", []provider.GenerateContentPart{custom(fallbackMarker("{\"type\":\"fallback\",\"to\":{\"model\":\"claude-\xff\"}}"))}, ""},
		{"missing target", []provider.GenerateContentPart{custom(fallbackMarker(`{"type":"fallback","from":{"model":"claude-a"}}`))}, ""},
		{"other kind", []provider.GenerateContentPart{{Type: provider.ContentCustom, Kind: "anthropic.container_upload", ProviderMetadata: marker("claude-b")}}, ""},
		{"invalid marker keeps the earlier valid one", []provider.GenerateContentPart{custom(marker("claude-b")), custom(marker(""))}, "claude-b"},
	} {
		t.Run("unary/"+tc.name, func(t *testing.T) {
			call := configuredCall(t, generated(tc.content...))
			_, err := call.Generate()
			require.NoError(t, err)
			view := call.View(nil)
			require.NotNil(t, view.Overview)
			require.Len(t, view.Overview.Attempts, 1)
			attempt := view.Overview.Attempts[0]
			assert.Equal(t, tc.served, attempt.ServedModelID)
			assert.Equal(t, "backend", attempt.ModelID)
			encoded, err := json.Marshal(view.Overview)
			require.NoError(t, err)
			if tc.served == "" {
				assert.NotContains(t, string(encoded), "servedModelId")
			}
		})
	}

	t.Run("no marker keeps the overview byte-identical", func(t *testing.T) {
		call := configuredCall(t, generated(provider.GenerateContentPart{Type: provider.ContentText, Text: "answer"}))
		_, err := call.Generate()
		require.NoError(t, err)
		encoded, err := json.Marshal(call.View(nil).Overview)
		require.NoError(t, err)
		assert.Equal(t, `{"requestedModelId":"alias","canonicalModelId":"canonical","attempts":[{"provider":"anthropic","modelId":"backend","providerInstance":"configured","outcome":"selected"}]}`, string(encoded))
	})

	t.Run("routing summary is unchanged by a marker", func(t *testing.T) {
		plain := configuredCall(t, generated())
		_, err := plain.Generate()
		require.NoError(t, err)
		hop := configuredCall(t, generated(custom(marker("claude-b"))))
		_, err = hop.Generate()
		require.NoError(t, err)
		routing := func(view View) json.RawMessage {
			var gateway struct {
				Execution json.RawMessage `json:"execution"`
				Routing   json.RawMessage `json:"routing"`
			}
			require.NoError(t, json.Unmarshal(view.Metadata()["gateway"], &gateway))
			return gateway.Routing
		}
		plainView, hopView := plain.View(nil), hop.View(nil)
		assert.JSONEq(t, string(routing(plainView)), string(routing(hopView)))
		assert.Contains(t, string(hopView.Metadata()["gateway"]), `"servedModelId":"claude-b"`)
	})

	t.Run("stream marker after the first part", func(t *testing.T) {
		parts := closedStream(
			provider.StreamPart{Type: provider.PartResponseMeta},
			provider.StreamPart{Type: provider.PartCustom, Kind: "anthropic.fallback", ProviderMetadata: marker("claude-b")},
			validFinish(),
		)
		call := configuredCall(t, &generateModel{modelID: "backend", stream: func(context.Context) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}})
		stream, err := call.Stream()
		require.NoError(t, err)
		defer stream.Close()
		_, wait := stream.Next()
		require.Equal(t, WaitPart, wait)
		view := call.View(wait.Err())
		require.NotNil(t, view.Overview)
		before := append([]Attempt(nil), view.Overview.Attempts...)
		assert.Empty(t, view.Overview.Attempts[0].ServedModelID)
		_, wait = drainParts(t, stream)
		assert.Equal(t, WaitFinished, wait)
		require.Len(t, view.Overview.Attempts, len(before))
		assert.Equal(t, "claude-b", view.Overview.Attempts[0].ServedModelID)
		view.Overview.Attempts[0].ServedModelID = ""
		assert.Equal(t, before, view.Overview.Attempts)
	})

	t.Run("stream marker as the first part", func(t *testing.T) {
		parts := closedStream(provider.StreamPart{Type: provider.PartCustom, Kind: "anthropic.fallback", ProviderMetadata: marker("claude-b")}, validFinish())
		call := configuredCall(t, &generateModel{modelID: "backend", stream: func(context.Context) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}})
		stream, err := call.Stream()
		require.NoError(t, err)
		defer stream.Close()
		_, wait := stream.Next()
		require.Equal(t, WaitPart, wait)
		view := call.View(wait.Err())
		require.NotNil(t, view.Overview)
		assert.Equal(t, "claude-b", view.Overview.Attempts[0].ServedModelID)
	})

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"failed attempt", errors.New("candidate failure")},
		{"canceled attempt", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := newAttemptCapture("alias", selectConfigured(t, resolved(&generateModel{})), SummaryRedactor{})
			attempts.enter()
			attempts.serve("claude-b")
			view := attempts.view(tc.err)
			require.NotNil(t, view.Overview)
			assert.NotEqual(t, fallback.AttemptSelected, view.Overview.Attempts[0].Outcome)
			assert.Empty(t, view.Overview.Attempts[0].ServedModelID)
		})
	}

	t.Run("only the selected attempt of a fallback chain", func(t *testing.T) {
		selected := selectConfigured(t, catalog.ResolvedModel{ID: "canonical", Model: &generateModel{}, Candidates: []catalog.ConfiguredCandidate{
			{Provider: "anthropic", ModelID: "first", ProviderInstance: "a"},
			{Provider: "anthropic", ModelID: "second", ProviderInstance: "b"},
		}})
		attempts := newAttemptCapture("alias", selected, SummaryRedactor{})
		attempts.enter()
		attempts.observe(t.Context(), fallback.Attempt{Index: 1, Provider: "anthropic", ModelID: "first", Outcome: fallback.AttemptFailed, WillFallback: true, SourceErr: errors.New("busy")})
		attempts.observe(t.Context(), fallback.Attempt{Index: 2, Provider: "anthropic", ModelID: "second", Outcome: fallback.AttemptSelected})
		view := attempts.view(nil)
		attempts.serve("claude-b")
		require.NotNil(t, view.Overview)
		require.Len(t, view.Overview.Attempts, 2)
		assert.Empty(t, view.Overview.Attempts[0].ServedModelID)
		assert.Equal(t, "claude-b", view.Overview.Attempts[1].ServedModelID)
	})
}
