package execution

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
)

// Overview explains one observed invocation without claiming stream completion.
type Overview struct {
	RequestedModelID string    `json:"requestedModelId"`
	CanonicalModelID string    `json:"canonicalModelId"`
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

// Failure contains only optional, protected candidate-local error fields.
type Failure struct {
	Message    string          `json:"message,omitempty"`
	Type       string          `json:"type,omitempty"`
	Code       json.RawMessage `json:"code,omitempty"`
	StatusCode int             `json:"statusCode,omitempty"`
}

// Project converts one invocation's decisions into an optional overview.
// Configured descriptors supply identity when present. sourceBytes is the
// caller's existing source/read bound, not a separate diagnostic allocation.
// Mixed or unfinished observation is omitted rather than treated as complete.
func Project(requested, canonical string, decisions []fallback.Attempt, candidates []catalog.ConfiguredCandidate, protectedSources []string, sourceBytes int64) *Overview {
	if requested == "" || canonical == "" || !utf8.ValidString(requested) || !utf8.ValidString(canonical) || len(decisions) == 0 {
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
		if len(candidates) != 0 {
			if i >= len(candidates) {
				return nil
			}
			candidate := candidates[i]
			attempt.Provider, attempt.ModelID, attempt.ProviderInstance = candidate.Provider, candidate.ModelID, candidate.ProviderInstance
		}
		if attempt.Provider == "" || attempt.ModelID == "" || !utf8.ValidString(attempt.Provider) || !utf8.ValidString(attempt.ModelID) || !utf8.ValidString(attempt.ProviderInstance) {
			return nil
		}
		if decision.Outcome != fallback.AttemptSelected {
			attempt.Error = summarize(decision.SourceErr, protectedSources, sourceBytes)
		}
		overview.Attempts = append(overview.Attempts, attempt)
	}
	return overview
}
