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
	if canonical == "" || !utf8.ValidString(canonical) {
		return nil
	}
	return project(requested, canonical, decisions, func(i int, attempt *Attempt) bool {
		if len(candidates) == 0 {
			return true
		}
		if i >= len(candidates) {
			return false
		}
		candidate := candidates[i]
		attempt.Provider, attempt.ModelID, attempt.ProviderInstance = candidate.Provider, candidate.ModelID, candidate.ProviderInstance
		return true
	})
}

// projectAccounts converts one request-account invocation's decisions into an
// optional overview without canonical identity. Every decision must name the
// accounts' native destination, and decisions must not outnumber the accounts.
func projectAccounts(requested string, decisions []fallback.Attempt, accounts RequestAccounts) *Overview {
	if len(decisions) > accounts.Count {
		return nil
	}
	return project(requested, "", decisions, func(_ int, attempt *Attempt) bool {
		return attempt.Provider == accounts.Provider && attempt.ModelID == accounts.ModelID
	})
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
