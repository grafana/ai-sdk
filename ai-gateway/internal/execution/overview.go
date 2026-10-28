package execution

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
)

// Overview explains one observed invocation without claiming stream completion.
// CanonicalModelID is empty when no catalog resolution selected the model.
type Overview struct {
	RequestedModelID string    `json:"requestedModelId"`
	CanonicalModelID string    `json:"canonicalModelId,omitempty"`
	Attempts         []Attempt `json:"attempts"`
}

// Attempt identifies an observed candidate and its selection outcome.
type Attempt struct {
	Provider         string                  `json:"provider"`
	ModelID          string                  `json:"modelId"`
	ProviderInstance string                  `json:"providerInstance,omitempty"`
	Outcome          fallback.AttemptOutcome `json:"outcome"`
	Error            *Failure                `json:"error,omitempty"`
}

// Failure contains optional candidate-local native error fields.
type Failure struct {
	Message    string          `json:"message,omitempty"`
	Type       string          `json:"type,omitempty"`
	Code       json.RawMessage `json:"code,omitempty"`
	StatusCode int             `json:"statusCode,omitempty"`
}

// Project converts one configured invocation's decisions into an optional
// overview, which requires a canonical identity. Configured descriptors supply
// identity when present. Native error bodies are bounded by the transport that
// read them. Mixed or unfinished observation is omitted rather than treated as
// complete.
func Project(requested, canonical string, decisions []fallback.Attempt, candidates []catalog.ConfiguredCandidate) *Overview {
	return projectGroups(requested, canonical, decisions, []modelGroup{{requested: requested, canonical: canonical, candidates: candidates}})
}

// projectGroups converts one invocation's decisions into an optional overview,
// attributing the decision at each position to the destination its model groups
// expect there. Configured groups require canonical, the requested model's
// canonical identity, and replace decision identity with the configured
// candidate's. Request-account groups have no canonical identity and must match
// the decision's native destination. Decisions that outnumber the expected
// destinations, or groups of both kinds, omit the overview.
func projectGroups(requested, canonical string, decisions []fallback.Attempt, groups []modelGroup) *Overview {
	slots, open, ok := groupSlots(groups)
	if !ok {
		return nil
	}
	if groups[0].accounts != nil {
		canonical = ""
	} else if canonical == "" || !utf8.ValidString(canonical) {
		return nil
	}
	return project(requested, canonical, decisions, func(i int, attempt *Attempt) bool {
		if open {
			return true
		}
		if i >= len(slots) {
			return false
		}
		switch slot := slots[i]; {
		case slot.candidate != nil:
			attempt.Provider, attempt.ModelID, attempt.ProviderInstance = slot.candidate.Provider, slot.candidate.ModelID, slot.candidate.ProviderInstance
			return true
		default:
			return attempt.Provider == slot.accounts.Provider && attempt.ModelID == slot.accounts.ModelID
		}
	})
}

// groupSlot is one expected attempt position of a selection's chain.
type groupSlot struct {
	group     int
	candidate *catalog.ConfiguredCandidate
	accounts  *RequestAccounts
}

// groupSlots flattens groups into their expected attempt positions. A single
// configured group without candidates is open: its positions take decision
// identity and are unbounded. Mixed configured and account groups, or an open
// group among others, are invalid.
func groupSlots(groups []modelGroup) (slots []groupSlot, open bool, ok bool) {
	if len(groups) == 0 {
		return nil, false, false
	}
	accounts := groups[0].accounts != nil
	for i, group := range groups {
		if (group.accounts != nil) != accounts {
			return nil, false, false
		}
		if accounts {
			for range group.accounts.Count {
				slots = append(slots, groupSlot{group: i, accounts: group.accounts})
			}
			continue
		}
		if len(group.candidates) == 0 {
			if len(groups) != 1 {
				return nil, false, false
			}
			return nil, true, true
		}
		for j := range group.candidates {
			slots = append(slots, groupSlot{group: i, candidate: &group.candidates[j]})
		}
	}
	return slots, false, true
}

// configuredCandidateCount returns the number of configured candidates across
// groups, or zero for request-account groups.
func configuredCandidateCount(groups []modelGroup) int {
	count := 0
	for _, group := range groups {
		if group.accounts != nil {
			return 0
		}
		count += len(group.candidates)
	}
	return count
}

// project validates decision order and outcomes; identify sets or checks each
// attempt's destination.
func project(requested, canonical string, decisions []fallback.Attempt, identify func(int, *Attempt) bool) *Overview {
	if requested == "" || !utf8.ValidString(requested) || len(decisions) == 0 {
		return nil
	}
	overview := &Overview{RequestedModelID: requested, CanonicalModelID: canonical, Attempts: make([]Attempt, 0, len(decisions))}
	for i, decision := range decisions {
		if decision.Index != i+1 {
			return nil
		}
		switch decision.Outcome {
		case fallback.AttemptFailed:
			if decision.WillFallback != (i < len(decisions)-1) {
				return nil
			}
		case fallback.AttemptSelected, fallback.AttemptCanceled:
			if i != len(decisions)-1 || decision.WillFallback {
				return nil
			}
		default:
			return nil
		}
		attempt := Attempt{Provider: decision.Provider, ModelID: decision.ModelID, Outcome: decision.Outcome}
		if !identify(i, &attempt) {
			return nil
		}
		if attempt.Provider == "" || attempt.ModelID == "" || !utf8.ValidString(attempt.Provider) || !utf8.ValidString(attempt.ModelID) || !utf8.ValidString(attempt.ProviderInstance) {
			return nil
		}
		if decision.Outcome != fallback.AttemptSelected {
			attempt.Error = summarize(decision.SourceErr)
		}
		overview.Attempts = append(overview.Attempts, attempt)
	}
	return overview
}
