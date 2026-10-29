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

	// fallbackModel and fallbackCanonical name the models entry, as sent, and
	// its canonical ID, whose chain the attempt belongs to. Both are empty for an
	// attempt of the requested model.
	fallbackModel     string
	fallbackCanonical string
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
	expected := make([]ExpectedAttempt, len(candidates))
	for i, candidate := range candidates {
		expected[i] = ExpectedAttempt{ModelID: requested, CanonicalID: canonical, Destination: candidate}
	}
	return projectExpected(requested, canonical, decisions, attributionConfigured, expected)
}

// projectExpected converts one invocation's decisions into an optional
// overview, attributing the decision at each position to the attempt expected
// there. Configured attribution requires canonical, the requested model's
// canonical identity, and replaces decision identity with the expected
// destination; without expected attempts, decisions keep their identity.
// Account attribution has no canonical identity and requires each decision to
// match its expected destination. Decisions that outnumber the expected
// attempts omit the overview.
func projectExpected(requested, canonical string, decisions []fallback.Attempt, kind attribution, expected []ExpectedAttempt) *Overview {
	switch kind {
	case attributionConfigured:
		if canonical == "" || !utf8.ValidString(canonical) {
			return nil
		}
	case attributionAccounts:
		canonical = ""
	default:
		return nil
	}
	return project(requested, canonical, decisions, func(i int, attempt *Attempt) bool {
		if len(expected) == 0 {
			return kind == attributionConfigured
		}
		if i >= len(expected) {
			return false
		}
		want := expected[i]
		if kind == attributionAccounts {
			if attempt.Provider != want.Destination.Provider || attempt.ModelID != want.Destination.ModelID {
				return false
			}
		} else {
			attempt.Provider, attempt.ModelID, attempt.ProviderInstance = want.Destination.Provider, want.Destination.ModelID, want.Destination.ProviderInstance
		}
		if want.ModelID != requested {
			attempt.fallbackModel, attempt.fallbackCanonical = want.ModelID, want.CanonicalID
		}
		return true
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
