package execution

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttemptCapture_Routing(t *testing.T) {
	overloaded := provider.NewAPICallError(provider.APICallErrorOptions{Message: "overloaded", StatusCode: 503})
	limited := provider.NewAPICallError(provider.APICallErrorOptions{Message: "limited", StatusCode: 429})
	failed := func(index int) fallback.Attempt {
		return fallback.Attempt{Index: index, Provider: "native", ModelID: "native", Outcome: fallback.AttemptFailed, SourceErr: overloaded, WillFallback: true}
	}
	configured := []ConfiguredModel{
		{RequestedID: "a", CanonicalID: "A", Candidates: []catalog.ConfiguredCandidate{{Provider: "openai", ModelID: "a1", ProviderInstance: "one"}}},
		{RequestedID: "b", CanonicalID: "B", Candidates: []catalog.ConfiguredCandidate{{Provider: "anthropic", ModelID: "b1", ProviderInstance: "two"}, {Provider: "openai", ModelID: "b2", ProviderInstance: "three"}}},
		{RequestedID: "c", CanonicalID: "C", Candidates: []catalog.ConfiguredCandidate{{Provider: "openai", ModelID: "c1", ProviderInstance: "four"}}},
	}
	for _, tc := range []struct {
		name      string
		selection Selection
		decisions []fallback.Attempt
		want      *Routing
	}{
		{
			name:      "configured fallback model serves the call",
			selection: ConfiguredSelection("A", &generateModel{}, configured),
			decisions: []fallback.Attempt{failed(1), {Index: 2, Outcome: fallback.AttemptSelected}},
			want: &Routing{
				OriginalModelID: "a", CanonicalSlug: "A", ResolvedProvider: "openai", FinalProvider: "anthropic",
				ModelAttemptCount: 2, TotalProviderAttemptCount: 2,
				ModelAttempts: []ModelAttempt{
					{ModelID: "a", CanonicalSlug: "A", ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai", StatusCode: 503}}},
					{ModelID: "b", CanonicalSlug: "B", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "anthropic", Success: true}}},
				},
			},
		},
		{
			name:      "unrouted call",
			selection: ConfiguredSelection("A", &generateModel{}, configured[:1]),
			decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptSelected}},
			want: &Routing{
				OriginalModelID: "a", CanonicalSlug: "A", ResolvedProvider: "openai", FinalProvider: "openai",
				ModelAttemptCount: 1, TotalProviderAttemptCount: 1,
				ModelAttempts: []ModelAttempt{{ModelID: "a", CanonicalSlug: "A", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai", Success: true}}}},
			},
		},
		{
			name:      "all attempts fail",
			selection: ConfiguredSelection("A", &generateModel{}, configured[:2]),
			decisions: []fallback.Attempt{failed(1), failed(2), {Index: 3, Provider: "native", ModelID: "native", Outcome: fallback.AttemptFailed, SourceErr: limited}},
			want: &Routing{
				OriginalModelID: "a", CanonicalSlug: "A", ResolvedProvider: "openai",
				ModelAttemptCount: 2, TotalProviderAttemptCount: 3,
				ModelAttempts: []ModelAttempt{
					{ModelID: "a", CanonicalSlug: "A", ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai", StatusCode: 503}}},
					{ModelID: "b", CanonicalSlug: "B", ProviderAttemptCount: 2, ProviderAttempts: []ProviderAttempt{{Provider: "anthropic", StatusCode: 503}, {Provider: "openai", StatusCode: 429}}},
				},
			},
		},
		{
			name:      "canceled last attempt",
			selection: ConfiguredSelection("A", &generateModel{}, configured[:1]),
			decisions: []fallback.Attempt{{Index: 1, Provider: "native", ModelID: "native", Outcome: fallback.AttemptCanceled}},
			want: &Routing{
				OriginalModelID: "a", CanonicalSlug: "A", ResolvedProvider: "openai",
				ModelAttemptCount: 1, TotalProviderAttemptCount: 1,
				ModelAttempts: []ModelAttempt{{ModelID: "a", CanonicalSlug: "A", ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai"}}}},
			},
		},
		{
			name:      "BYOK model attempts",
			selection: AccountSelection("anthropic/x", &generateModel{}, []RequestAccounts{{Provider: "anthropic", ModelID: "x", Count: 1}, {Provider: "openai", ModelID: "y", Count: 1}}),
			decisions: []fallback.Attempt{
				{Index: 1, Provider: "anthropic", ModelID: "x", Outcome: fallback.AttemptFailed, SourceErr: overloaded, WillFallback: true},
				{Index: 2, Provider: "openai", ModelID: "y", Outcome: fallback.AttemptSelected},
			},
			want: &Routing{
				OriginalModelID: "anthropic/x", ResolvedProvider: "anthropic", FinalProvider: "openai",
				ModelAttemptCount: 2, TotalProviderAttemptCount: 2,
				ModelAttempts: []ModelAttempt{
					{ModelID: "anthropic/x", ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "anthropic", StatusCode: 503}}},
					{ModelID: "openai/y", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai", Success: true}}},
				},
			},
		},
		{
			name:      "model never reached is not described",
			selection: ConfiguredSelection("A", &generateModel{}, configured),
			decisions: []fallback.Attempt{{Index: 1, Outcome: fallback.AttemptSelected}},
			want: &Routing{
				OriginalModelID: "a", CanonicalSlug: "A", ResolvedProvider: "openai", FinalProvider: "openai",
				ModelAttemptCount: 1, TotalProviderAttemptCount: 1,
				ModelAttempts: []ModelAttempt{{ModelID: "a", CanonicalSlug: "A", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "openai", Success: true}}}},
			},
		},
		{
			name:      "group mismatch publishes neither",
			selection: ConfiguredSelection("A", &generateModel{}, configured[:1]),
			decisions: []fallback.Attempt{failed(1), {Index: 2, Outcome: fallback.AttemptSelected}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := newAttemptCapture(tc.selection.groups[0].requested, tc.selection, SummaryRedactor{})
			attempts.enter()
			for _, decision := range tc.decisions {
				attempts.observe(t.Context(), decision)
			}
			view := attempts.view(nil)
			assert.Equal(t, tc.want, view.Routing)
			if tc.want == nil {
				assert.Nil(t, view.Overview)
				assert.Nil(t, view.Metadata())
				return
			}
			require.NotNil(t, view.Overview)
			assert.Len(t, view.Overview.Attempts, tc.want.TotalProviderAttemptCount)
			var placed namespace
			require.NoError(t, json.Unmarshal(view.Metadata()["gateway"], &placed))
			assert.Equal(t, tc.want, placed.Routing)
			assert.Equal(t, view.Overview, placed.Execution)
		})
	}
}

func TestAttemptCapture_RoutingForOpenGroup(t *testing.T) {
	attempts := newAttemptCapture("alias", ConfiguredSelection("public", &generateModel{}, []ConfiguredModel{{RequestedID: "alias", CanonicalID: "public"}}), SummaryRedactor{})
	attempts.observe(t.Context(), fallback.Attempt{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected})
	view := attempts.view(nil)
	require.NotNil(t, view.Routing)
	assert.Equal(t, []ModelAttempt{{ModelID: "alias", CanonicalSlug: "public", Success: true, ProviderAttemptCount: 1, ProviderAttempts: []ProviderAttempt{{Provider: "native", Success: true}}}}, view.Routing.ModelAttempts)
}

func TestMetadata_RoutingRequiresOverview(t *testing.T) {
	assert.Nil(t, Metadata(nil, &Routing{OriginalModelID: "a"}, nil))
	metadata := Metadata(&Overview{RequestedModelID: "a", Attempts: []Attempt{{Provider: "p", ModelID: "m", Outcome: fallback.AttemptSelected}}}, nil, nil)
	assert.NotContains(t, string(metadata["gateway"]), "routing")
}
