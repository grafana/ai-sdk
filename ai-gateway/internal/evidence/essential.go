package evidence

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/fallback"
)

type attemptDisposition struct {
	State  DispositionState `json:"state"`
	Reason Reason           `json:"reason"`
	Count  int              `json:"count"`
}

func (s *State) essentialNamespaceLocked(gateway *Gateway) namespace {
	attempts := append([]Attempt(nil), s.attempts...)
	for i, attempt := range attempts {
		if attempt.NativeError != nil {
			value := *attempt.NativeError
			value.Details = nil
			attempts[i].NativeError = &value
		}
	}
	if gateway != nil && gateway.Classification != "" && gateway.Classification != ClassificationCanceled && gateway.Classification != ClassificationTimeout && s.selected == 0 && len(attempts) != 0 {
		last := &attempts[len(attempts)-1]
		if last.Selection == fallback.AttemptCanceled && last.WillFallback == nil {
			last.Selection = fallback.AttemptFailed
		}
	}
	facts := snapshot{RequestedModelID: s.requested, CanonicalModelID: s.canonical, SelectedAttempt: s.selected, Attempts: attempts}
	if s.selected != 0 {
		facts.Native = s.identity
	}
	if s.count > MaxAttempts {
		facts.Attempts = attemptDisposition{State: OverLimit, Reason: AttemptLimit, Count: s.count}
	}
	if gateway != nil {
		value := *gateway
		if s.completed {
			value.ReplayRisk = GenerationCompleted
		} else {
			value.ReplayRisk = GenerationMayHaveRun
		}
		facts.Gateway = &value
	}
	return namespace{Routing: routing{OriginalModelID: s.requested, CanonicalSlug: s.canonical, FinalProvider: s.selectedProvider}, Evidence: facts}
}

func (s *State) enforceEssentialLocked() {
	value := struct {
		Metadata     namespace    `json:"metadata"`
		CurrentError *NativeError `json:"nativeError,omitempty"`
	}{Metadata: s.essentialNamespaceLocked(nil)}
	if s.currentError != nil {
		current := *s.currentError
		current.Details = nil
		value.CurrentError = &current
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > EssentialBytes {
		s.invalid = true
		s.attempts = nil
		s.identity = nil
		s.currentError = nil
	}
}
