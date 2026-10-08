package execution

import (
	"context"
	"encoding/json"
	"fmt"
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
			got := Project("alias", "public", decisions, candidates, nil, 4096)
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
	got := Project("public", "public", decisions, nil, nil, 4096)
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
			assert.Nil(t, Project("public", "public", tc.decisions, nil, nil, 4096))
		})
	}
}

func TestProject_SelectedErrorPartsNotDuplicated(t *testing.T) {
	got := Project("public", "public", []fallback.Attempt{{Index: 1, Provider: "native", ModelID: "model", Outcome: fallback.AttemptSelected}}, nil, nil, 4096)
	require.NotNil(t, got)
	require.Len(t, got.Attempts, 1)
	assert.Nil(t, got.Attempts[0].Error)
}
