package routing

import (
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// Candidate is one destination of a model group.
type Candidate struct {
	// Provider is the provider slug that only and order match.
	Provider string
	// Destination identifies the destination across groups, such as a provider
	// instance and backend model. A repeated destination is attempted once; an
	// empty destination is never treated as repeated.
	Destination string
}

// Group is one model of the chain and its candidates in configured order.
type Group struct {
	// Key identifies the model, such as its canonical ID. A repeated model is
	// skipped.
	Key        string
	Candidates []Candidate
}

// Placement is one composed attempt: the indexes of its group and of the
// candidate within that group.
type Placement struct {
	Group     int
	Candidate int
}

// Compose returns the attempt order of groups: each group's candidates, in
// group order, restricted to the only filter and then stably moved to the
// front by order. Repeated groups and destinations are skipped. It returns
// [execution.ErrNoRoutingCandidates] when no candidate remains and
// [execution.ErrRoutingAttemptBound] when more than bound remain.
func Compose(groups []Group, only *[]string, order []string, bound int) ([]Placement, error) {
	var placements []Placement
	seenGroups := make(map[string]bool, len(groups))
	seenDestinations := make(map[string]bool)
	for g, group := range groups {
		if seenGroups[group.Key] {
			continue
		}
		seenGroups[group.Key] = true
		indexes := make([]int, 0, len(group.Candidates))
		for c, candidate := range group.Candidates {
			if only == nil || slices.Contains(*only, candidate.Provider) {
				indexes = append(indexes, c)
			}
		}
		slices.SortStableFunc(indexes, func(a, b int) int {
			return preference(order, group.Candidates[a].Provider) - preference(order, group.Candidates[b].Provider)
		})
		for _, c := range indexes {
			destination := group.Candidates[c].Destination
			if destination != "" {
				if seenDestinations[destination] {
					continue
				}
				seenDestinations[destination] = true
			}
			placements = append(placements, Placement{Group: g, Candidate: c})
		}
	}
	if len(placements) == 0 {
		return nil, execution.ErrNoRoutingCandidates
	}
	if len(placements) > bound {
		return nil, execution.ErrRoutingAttemptBound
	}
	return placements, nil
}

// preference ranks a provider slug by its first position in order, after every
// listed slug when it is absent.
func preference(order []string, slug string) int {
	if i := slices.Index(order, slug); i >= 0 {
		return i
	}
	return len(order)
}
