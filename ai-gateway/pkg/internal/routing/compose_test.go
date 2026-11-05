package routing

import (
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompose(t *testing.T) {
	candidate := func(provider, destination string) Candidate {
		return Candidate{Provider: provider, Destination: destination}
	}
	empty := []string{}
	for _, tc := range []struct {
		name   string
		groups []Group
		only   []string
		filter bool
		order  []string
		want   []Placement
	}{
		{
			name: "requested model then fallback model",
			groups: []Group{
				{Key: "a", Candidates: []Candidate{candidate("openai", "a1"), candidate("anthropic", "a2")}},
				{Key: "b", Candidates: []Candidate{candidate("anthropic", "b1")}},
			},
			want: []Placement{{0, 0}, {0, 1}, {1, 0}},
		},
		{
			name:   "filter removes a provider",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1"), candidate("anthropic", "a2")}}},
			only:   []string{"anthropic"}, filter: true,
			want: []Placement{{0, 1}},
		},
		{
			name: "filter skips a whole model",
			groups: []Group{
				{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}},
				{Key: "b", Candidates: []Candidate{candidate("anthropic", "b1")}},
			},
			only: []string{"anthropic"}, filter: true,
			want: []Placement{{1, 0}},
		},
		{
			name:   "preference reorders without filtering",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1"), candidate("anthropic", "a2"), candidate("compatible-x", "a3")}}},
			order:  []string{"anthropic"},
			want:   []Placement{{0, 1}, {0, 0}, {0, 2}},
		},
		{
			name:   "filter then preference",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("anthropic", "a1"), candidate("compatible-x", "a2"), candidate("openai", "a3")}}},
			only:   []string{"anthropic", "openai"}, filter: true,
			order: []string{"openai"},
			want:  []Placement{{0, 2}, {0, 0}},
		},
		{
			name:   "preference follows order sequence and keeps ties stable",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("x", "1"), candidate("anthropic", "2"), candidate("openai", "3"), candidate("anthropic", "4")}}},
			order:  []string{"openai", "anthropic", "openai"},
			want:   []Placement{{0, 2}, {0, 1}, {0, 3}, {0, 0}},
		},
		{
			name:   "preference applies per model",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}}, {Key: "b", Candidates: []Candidate{candidate("openai", "b1"), candidate("anthropic", "b2")}}},
			order:  []string{"anthropic"},
			want:   []Placement{{0, 0}, {1, 1}, {1, 0}},
		},
		{
			name:   "unlisted slug has no effect",
			groups: []Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1"), candidate("anthropic", "a2")}}},
			only:   []string{"openai", "vertex"}, filter: true,
			order: []string{"bedrock"},
			want:  []Placement{{0, 0}},
		},
		{
			name: "repeated model is skipped",
			groups: []Group{
				{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}},
				{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}},
				{Key: "b", Candidates: []Candidate{candidate("openai", "b1")}},
			},
			want: []Placement{{0, 0}, {2, 0}},
		},
		{
			name: "shared destination is attempted once",
			groups: []Group{
				{Key: "a", Candidates: []Candidate{candidate("openai", "shared"), candidate("anthropic", "a2")}},
				{Key: "b", Candidates: []Candidate{candidate("openai", "shared"), candidate("openai", "b2")}},
			},
			want: []Placement{{0, 0}, {0, 1}, {1, 1}},
		},
		{
			name: "empty destinations are never repeated",
			groups: []Group{
				{Key: "anthropic/x", Candidates: []Candidate{candidate("anthropic", ""), candidate("anthropic", "")}},
				{Key: "anthropic/y", Candidates: []Candidate{candidate("anthropic", "")}},
			},
			want: []Placement{{0, 0}, {0, 1}, {1, 0}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var only *[]string
			if tc.filter {
				only = &tc.only
			}
			got, err := Compose(tc.groups, only, tc.order, 8)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	t.Run("filter excludes every candidate", func(t *testing.T) {
		only := []string{"vertex"}
		_, err := Compose([]Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}}}, &only, nil, 8)
		require.ErrorIs(t, err, execution.ErrNoRoutingCandidates)
	})
	t.Run("empty filter excludes every candidate", func(t *testing.T) {
		_, err := Compose([]Group{{Key: "a", Candidates: []Candidate{candidate("openai", "a1")}}}, &empty, nil, 8)
		require.ErrorIs(t, err, execution.ErrNoRoutingCandidates)
	})
	t.Run("chain at and above the bound", func(t *testing.T) {
		var group Group
		for i := range 9 {
			group.Candidates = append(group.Candidates, candidate("openai", string(rune('a'+i))))
		}
		got, err := Compose([]Group{{Key: "a", Candidates: group.Candidates[:8]}}, nil, nil, 8)
		require.NoError(t, err)
		assert.Len(t, got, 8)
		_, err = Compose([]Group{{Key: "a", Candidates: group.Candidates}}, nil, nil, 8)
		require.ErrorIs(t, err, execution.ErrRoutingAttemptBound)
	})
	t.Run("filtering precedes the bound", func(t *testing.T) {
		var candidates []Candidate
		for i := range 9 {
			provider := "openai"
			if i == 0 {
				provider = "anthropic"
			}
			candidates = append(candidates, candidate(provider, string(rune('a'+i))))
		}
		only := []string{"anthropic"}
		got, err := Compose([]Group{{Key: "a", Candidates: candidates}}, &only, nil, 8)
		require.NoError(t, err)
		assert.Equal(t, []Placement{{0, 0}}, got)
	})
}
