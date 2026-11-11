package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProject_OrderedAttempts(t *testing.T) {
	failure := provider.NewAPICallError(provider.APICallErrorOptions{Message: "overloaded", StatusCode: 503})
	for _, outcome := range []fallback.AttemptOutcome{fallback.AttemptSelected, fallback.AttemptFailed, fallback.AttemptCanceled} {
		t.Run(string(outcome), func(t *testing.T) {
			decisions := []fallback.Attempt{
				{Index: 1, Provider: "native-primary", ModelID: "first", Outcome: fallback.AttemptFailed, SourceErr: failure, Err: failure, WillFallback: true},
				{Index: 2, Provider: "native-secondary", ModelID: "second", Outcome: outcome},
			}
			if outcome == fallback.AttemptCanceled {
				decisions[1].Err = context.Canceled
				decisions[1].SourceErr = failure
			}
			candidates := []catalog.ConfiguredCandidate{
				{Provider: "primary", ModelID: "configured-first", ProviderInstance: "account-a"},
				{Provider: "secondary", ModelID: "configured-second", ProviderInstance: "account-b"},
				{Provider: "unrun", ModelID: "never"},
			}
			got := Project("alias", "public", decisions, candidates)
			require.NotNil(t, got)
			assert.Equal(t, "alias", got.RequestedModelID)
			assert.Equal(t, "public", got.CanonicalModelID)
			require.Len(t, got.Attempts, 2)
			assert.Equal(t, "primary", got.Attempts[0].Provider)
			assert.Equal(t, "configured-first", got.Attempts[0].ModelID)
			assert.Equal(t, "account-a", got.Attempts[0].ProviderInstance)
			require.NotNil(t, got.Attempts[0].Error)
			assert.Equal(t, "overloaded", got.Attempts[0].Error.Message)
			assert.Equal(t, outcome, got.Attempts[1].Outcome)
			if outcome == fallback.AttemptCanceled {
				require.NotNil(t, got.Attempts[1].Error)
				assert.Equal(t, 503, got.Attempts[1].Error.StatusCode)
			} else {
				assert.Nil(t, got.Attempts[1].Error)
			}
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			for _, absent := range []string{"selectedAttempt", "index", "routing", "completion", "replayRisk", "willFallback", "details", "unrun"} {
				assert.NotContains(t, string(encoded), absent)
			}
		})
	}
}

func TestProject_NoHistoryCap(t *testing.T) {
	var decisions []fallback.Attempt
	for i := range 32 {
		outcome := fallback.AttemptFailed
		if i == 31 {
			outcome = fallback.AttemptSelected
		}
		decisions = append(decisions, fallback.Attempt{Index: i + 1, Provider: "native", ModelID: fmt.Sprintf("model-%d", i), Outcome: outcome, WillFallback: i != 31})
	}
	got := Project("public", "public", decisions, nil)
	require.NotNil(t, got)
	require.Len(t, got.Attempts, 32)
	assert.Equal(t, fallback.AttemptSelected, got.Attempts[31].Outcome)
}

func TestProject_InvalidObservationOmitted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		decisions []fallback.Attempt
	}{
		{name: "empty"},
		{name: "unfinished advance", decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptFailed, WillFallback: true}}},
		{name: "index gap", decisions: []fallback.Attempt{{Index: 2, Outcome: fallback.AttemptSelected}}},
		{name: "mixed calls", decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptSelected}, {Index: 1, Outcome: fallback.AttemptSelected}}},
		{name: "selection before last", decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptSelected}, {Index: 2, Outcome: fallback.AttemptFailed}}},
		{name: "advance after rejected decision", decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptFailed}, {Index: 2, Outcome: fallback.AttemptSelected}}},
		{name: "invalid outcome", decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptOutcome("unknown")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.decisions {
				tc.decisions[i].Provider, tc.decisions[i].ModelID = "native", "model"
			}
			assert.Nil(t, Project("public", "public", tc.decisions, nil))
		})
	}
}

func TestProject_ConfiguredRequiresCanonical(t *testing.T) {
	decisions := []fallback.Attempt{{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}
	assert.Nil(t, Project("alias", "", decisions, nil))
	assert.Nil(t, Project("alias", "\xff", decisions, nil))
	attempts := newAttemptCapture("alias", selectConfigured(t, catalog.ResolvedModel{Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "model"}}}), SummaryRedactor{})
	attempts.enter()
	assert.Nil(t, attempts.view(nil).Overview)
	got := Project("alias", "public", decisions, nil)
	require.NotNil(t, got)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"canonicalModelId":"public"`)
}

func TestProject_SelectedErrorPartsNotDuplicated(t *testing.T) {
	got := Project("public", "public", []fallback.Attempt{{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}, nil)
	require.NotNil(t, got)
	require.Len(t, got.Attempts, 1)
	assert.Nil(t, got.Attempts[0].Error)
}

func TestProjectExpected_FallbackModels(t *testing.T) {
	failure := provider.NewAPICallError(provider.APICallErrorOptions{Message: "overloaded", StatusCode: 503})
	failed := func(index int, providerName, modelID string) fallback.Attempt {
		return fallback.Attempt{Index: index, Provider: providerName, ModelID: modelID, Outcome: fallback.AttemptFailed, SourceErr: failure, WillFallback: true}
	}
	selected := func(index int, providerName, modelID string) fallback.Attempt {
		return fallback.Attempt{Index: index, Provider: providerName, ModelID: modelID, Outcome: fallback.AttemptSelected}
	}
	configured := slices.Concat(
		configuredAttempts("alias", "first", catalog.ConfiguredCandidate{Provider: "openai", ModelID: "a1", ProviderInstance: "one"}, catalog.ConfiguredCandidate{Provider: "anthropic", ModelID: "a2", ProviderInstance: "two"}),
		configuredAttempts("second", "second", catalog.ConfiguredCandidate{Provider: "anthropic", ModelID: "b1", ProviderInstance: "three"}),
	)
	accounts := slices.Concat(accountAttempts("anthropic", "x", 2), accountAttempts("openai", "y", 1))
	for _, tc := range []struct {
		name      string
		kind      attribution
		expected  []ExpectedAttempt
		decisions []fallback.Attempt
		want      []Attempt
		canonical string
	}{
		{
			name:      "configured second model selected",
			kind:      attributionConfigured,
			expected:  configured,
			decisions: []fallback.Attempt{failed(1, "native", "ignored"), failed(2, "native", "ignored"), selected(3, "native", "ignored")},
			canonical: "first",
			want: []Attempt{
				{Provider: "openai", ModelID: "a1", ProviderInstance: "one", Outcome: fallback.AttemptFailed},
				{Provider: "anthropic", ModelID: "a2", ProviderInstance: "two", Outcome: fallback.AttemptFailed},
				{Provider: "anthropic", ModelID: "b1", ProviderInstance: "three", Outcome: fallback.AttemptSelected},
			},
		},
		{
			name:      "requested model filtered out keeps its canonical identity",
			kind:      attributionConfigured,
			expected:  configured[2:],
			decisions: []fallback.Attempt{selected(1, "native", "ignored")},
			canonical: "first",
			want:      []Attempt{{Provider: "anthropic", ModelID: "b1", ProviderInstance: "three", Outcome: fallback.AttemptSelected}},
		},
		{
			name:      "configured attempts outnumber candidates",
			kind:      attributionConfigured,
			expected:  configured,
			canonical: "first",
			decisions: []fallback.Attempt{failed(1, "n", "m"), failed(2, "n", "m"), failed(3, "n", "m"), selected(4, "n", "m")},
		},
		{
			name:      "configured selection without canonical identity",
			kind:      attributionConfigured,
			expected:  configured,
			decisions: []fallback.Attempt{selected(1, "n", "m")},
		},
		{
			name:      "accounts second model selected",
			kind:      attributionAccounts,
			expected:  accounts,
			decisions: []fallback.Attempt{failed(1, "anthropic", "x"), failed(2, "anthropic", "x"), selected(3, "openai", "y")},
			want: []Attempt{
				{Provider: "anthropic", ModelID: "x", Outcome: fallback.AttemptFailed},
				{Provider: "anthropic", ModelID: "x", Outcome: fallback.AttemptFailed},
				{Provider: "openai", ModelID: "y", Outcome: fallback.AttemptSelected},
			},
		},
		{
			name:      "account attempt at another model's position",
			kind:      attributionAccounts,
			expected:  accounts,
			decisions: []fallback.Attempt{failed(1, "anthropic", "x"), selected(2, "openai", "y")},
		},
		{
			name:      "account attempts outnumber accounts",
			kind:      attributionAccounts,
			expected:  accounts,
			decisions: []fallback.Attempt{failed(1, "anthropic", "x"), failed(2, "anthropic", "x"), failed(3, "openai", "y"), selected(4, "openai", "y")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := projectExpected("alias", tc.canonical, tc.decisions, tc.kind, tc.expected)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, "alias", got.RequestedModelID)
			assert.Equal(t, tc.canonical, got.CanonicalModelID)
			require.Len(t, got.Attempts, len(tc.want))
			for i, want := range tc.want {
				attempt := got.Attempts[i]
				if want.Outcome == fallback.AttemptFailed {
					require.NotNil(t, attempt.Error)
					assert.Equal(t, "overloaded", attempt.Error.Message)
				}
				attempt.Error, attempt.fallbackModel, attempt.fallbackCanonical = nil, "", ""
				assert.Equal(t, want, attempt)
			}
		})
	}
}

func TestAttemptCapture_ConcurrentModelGroups(t *testing.T) {
	retryable := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "overloaded", IsRetryable: new(true)})
	const calls = 32
	results := make([]*Overview, calls)
	done := make(chan struct{})
	for i := range calls {
		go func() {
			defer func() { done <- struct{}{} }()
			failFirst := i%2 == 0
			first := &generateModel{modelID: "a1", generate: func(context.Context) (*provider.GenerateResult, error) {
				if failFirst {
					return nil, retryable
				}
				return &provider.GenerateResult{}, nil
			}}
			chain, err := fallback.New(first, &generateModel{modelID: "b1"})
			if err != nil {
				return
			}
			selected := ConfiguredSelection("first", chain, slices.Concat(
				configuredAttempts("alias", "first", catalog.ConfiguredCandidate{Provider: "openai", ModelID: "a1", ProviderInstance: "one"}),
				configuredAttempts("second", "second", catalog.ConfiguredCandidate{Provider: "anthropic", ModelID: "b1", ProviderInstance: "two"}),
			))
			attempts := newAttemptCapture("alias", selected, SummaryRedactor{})
			attempts.enter()
			_, err = chain.DoGenerate(attempts.withObserver(t.Context()), provider.CallOptions{})
			results[i] = attempts.view(err).Overview
		}()
	}
	for range calls {
		<-done
	}
	for i, overview := range results {
		require.NotNil(t, overview, "call %d", i)
		if i%2 == 0 {
			require.Len(t, overview.Attempts, 2, "call %d", i)
			assert.Equal(t, "b1", overview.Attempts[1].ModelID)
			assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)
		} else {
			require.Len(t, overview.Attempts, 1, "call %d", i)
			assert.Equal(t, "a1", overview.Attempts[0].ModelID)
		}
	}
}

func TestConfiguredSelection_Identity(t *testing.T) {
	expected := configuredAttempts("alias", "first", catalog.ConfiguredCandidate{Provider: "openai", ModelID: "a1", ProviderInstance: "one"})
	selected := ConfiguredSelection("first", &generateModel{modelID: "a1"}, expected)
	expected[0].Destination.ModelID = "mutated"
	assert.Equal(t, "first", selected.ID)
	assert.True(t, validSelection(selected))
	assert.Equal(t, "a1", selected.expected[0].Destination.ModelID)
	assert.False(t, validSelection(ConfiguredSelection("", &generateModel{}, nil)))
}
