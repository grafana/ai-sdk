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
	return &provider.GenerateResult{}, nil
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
	selected.configured.candidates[0].ModelID = "mutated-again"
	assert.Equal(t, "canonical", attempts.canonical)
	assert.Equal(t, "backend", attempts.candidates[0].ModelID)
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
	accounts := RequestAccounts{Provider: "native", ModelID: "model", Count: 2}
	selected := AccountSelection("native/model", &generateModel{modelID: "model"}, accounts)
	assert.True(t, validSelection(selected))
	assert.Nil(t, selected.configured)
	require.NotNil(t, selected.accounts)
	assert.Equal(t, RequestAccounts{Provider: "native", ModelID: "model", Count: 2}, *selected.accounts)
	attempts := newAttemptCapture("native/model", selected, SummaryRedactor{})
	assert.Equal(t, RequestAccounts{Provider: "native", ModelID: "model", Count: 2}, *attempts.accounts)
	assert.Empty(t, attempts.canonical)
	assert.Empty(t, attempts.candidates)
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
		accounts RequestAccounts
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
		{name: "mismatched provider", failures: []error{retryable, nil}, accounts: RequestAccounts{Provider: "other", ModelID: "model", Count: 2}, calls: []int{1, 1}},
		{name: "mismatched model", failures: []error{retryable, nil}, accounts: RequestAccounts{Provider: "native", ModelID: "other", Count: 2}, calls: []int{1, 1}},
		{name: "account overflow", failures: []error{retryable, nil}, accounts: RequestAccounts{Provider: "native", ModelID: "model", Count: 1}, calls: []int{1, 1}},
		{name: "zero attempts", accounts: RequestAccounts{Provider: "native", ModelID: "model", Count: 1}},
		{name: "mixed calls", accounts: RequestAccounts{Provider: "native", ModelID: "model", Count: 2}, observed: []fallback.Attempt{
			{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected},
			{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.accounts == (RequestAccounts{}) {
				tc.accounts = RequestAccounts{Provider: "native", ModelID: "model", Count: len(tc.failures)}
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
					return &provider.GenerateResult{}, nil
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
