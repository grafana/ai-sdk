package evidence

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const maxAttempts = 16

type Facts struct {
	RequestedModelID string
	CanonicalModelID string
	AttemptCount     int
	SelectedAttempt  int
	SelectedProvider string
	Attempts         []Attempt
	Identity         *Identity
	CurrentError     *NativeError
	Completed        bool

	err error
}

type contextKey struct{}

type State struct {
	mu               sync.Mutex
	facts            Facts
	protectedSources []string
	currentProvider  string
	detailBytes      int
	sealed           bool
}

func New(ctx context.Context, requested, canonical string) (context.Context, *State) {
	s := &State{facts: Facts{RequestedModelID: requested, CanonicalModelID: canonical}}
	if !validFacts(requested, canonical) {
		s.facts.err = errors.New("gateway evidence: invalid route identity")
	}
	return context.WithValue(ctx, contextKey{}, s), s
}

func FromContext(ctx context.Context) *State { s, _ := ctx.Value(contextKey{}).(*State); return s }

func (s *State) Seal() { s.mu.Lock(); s.sealed = true; s.mu.Unlock() }

func (s *State) HasAttempts() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.facts.AttemptCount != 0
}

func (s *State) Snapshot() Facts {
	if s == nil {
		return Facts{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts := s.facts
	facts.Attempts = append([]Attempt(nil), facts.Attempts...)
	for i := range facts.Attempts {
		facts.Attempts[i].NativeError = copyError(facts.Attempts[i].NativeError)
		if facts.Attempts[i].WillFallback != nil {
			will := *facts.Attempts[i].WillFallback
			facts.Attempts[i].WillFallback = &will
		}
	}
	if facts.Identity != nil {
		identity := *facts.Identity
		facts.Identity = &identity
	}
	facts.CurrentError = copyError(facts.CurrentError)
	return facts
}

func (s *State) CurrentError() *NativeError {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyError(s.facts.CurrentError)
}

func (s *State) begin(candidate catalog.ConfiguredCandidate, sources ...string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed {
		return 0
	}
	s.protectedSources = sources
	s.currentProvider = candidate.Provider
	s.facts.AttemptCount++
	index := s.facts.AttemptCount
	if index > maxAttempts {
		s.facts.Attempts = nil
		s.detailBytes = 0
		return index
	}
	if !validFacts(candidate.Provider, candidate.ModelID, candidate.ProviderInstance) {
		s.facts.err = errors.New("gateway evidence: invalid candidate identity")
		return index
	}
	s.facts.Attempts = append(s.facts.Attempts, Attempt{Index: index, Provider: candidate.Provider, ModelID: candidate.ModelID, ProviderInstance: candidate.ProviderInstance, Selection: fallback.AttemptCanceled})
	return index
}

func (s *State) returned(index int, result *provider.GenerateResult, err error, streaming bool, ordered bool) {
	if s == nil || index == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed {
		return
	}
	if !streaming && result != nil && err == nil {
		s.facts.Completed = true
		if !ordered {
			s.selectLocked(index)
		}
		if result.Response != nil {
			s.setIdentityLocked(result.Response.ID, result.Response.ModelID, result.Response.Timestamp)
		}
	}
	if index > len(s.facts.Attempts) {
		return
	}
	a := &s.facts.Attempts[index-1]
	switch {
	case err != nil:
		a.Selection = fallback.AttemptFailed
		s.retainErrorLocked(a, err)
	case !streaming && result != nil:
		a.Completion = Completed
	case !streaming:
		a.Selection = fallback.AttemptFailed
	}
}

func (s *State) cancel(index int) {
	if s == nil || index == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed {
		return
	}
	if index <= len(s.facts.Attempts) {
		s.facts.Attempts[index-1].Selection = fallback.AttemptCanceled
	}
	if s.facts.SelectedAttempt == index {
		s.facts.SelectedAttempt = 0
		s.facts.SelectedProvider = ""
	}
}

func ObserveDecision(ctx context.Context, decision fallback.Attempt) {
	s := FromContext(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed || decision.Index < 1 || decision.Index > s.facts.AttemptCount {
		return
	}
	if decision.Outcome == fallback.AttemptSelected {
		s.selectLocked(decision.Index)
	}
	if decision.Index > len(s.facts.Attempts) {
		return
	}
	a := &s.facts.Attempts[decision.Index-1]
	a.Selection = decision.Outcome
	will := decision.WillFallback
	a.WillFallback = &will
}

func (s *State) selectLocked(index int) {
	s.facts.SelectedAttempt = index
	if index == s.facts.AttemptCount {
		s.facts.SelectedProvider = s.currentProvider
	}
	if index <= len(s.facts.Attempts) {
		s.facts.Attempts[index-1].Selection = fallback.AttemptSelected
		s.facts.SelectedProvider = s.facts.Attempts[index-1].Provider
	}
}

func (s *State) ObservePart(part provider.StreamPart) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed || s.facts.AttemptCount == 0 {
		return
	}
	if s.facts.SelectedAttempt == 0 {
		s.selectLocked(s.facts.AttemptCount)
	}
	switch part.Type {
	case provider.PartResponseMeta:
		s.setIdentityLocked(part.ResponseID, part.ModelID, part.Timestamp)
	case provider.PartFinish:
		if part.FinishReason != nil {
			s.facts.Completed = true
			if s.facts.SelectedAttempt <= len(s.facts.Attempts) {
				s.facts.Attempts[s.facts.SelectedAttempt-1].Completion = Completed
			}
		}
	case provider.PartError:
		value, err := normalizeError(part.APICallError, s.protectedSources)
		if err != nil {
			s.facts.err = err
		}
		s.facts.CurrentError = summary(value)
		if s.facts.SelectedAttempt <= len(s.facts.Attempts) {
			s.retainLocked(&s.facts.Attempts[s.facts.SelectedAttempt-1], value)
		}
	}
}

func (s *State) setIdentityLocked(id, model string, timestamp time.Time) {
	if !validFacts(id, model) {
		s.facts.err = errors.New("gateway evidence: invalid native identity")
		return
	}
	identity := Identity{ID: id, ModelID: model}
	if !timestamp.IsZero() {
		identity.Timestamp = timestamp.UTC().Format(time.RFC3339Nano)
	}
	if id != "" || model != "" || identity.Timestamp != "" {
		s.facts.Identity = &identity
	}
}

func (s *State) retainErrorLocked(attempt *Attempt, err error) {
	value, normalizationErr := normalizeError(err, s.protectedSources)
	if normalizationErr != nil {
		s.facts.err = normalizationErr
	}
	s.retainLocked(attempt, value)
}

func (s *State) retainLocked(attempt *Attempt, value *NativeError) {
	if value == nil {
		return
	}
	if attempt.NativeError != nil {
		s.detailBytes -= len(attempt.NativeError.Details)
	}
	if s.detailBytes+len(value.Details) > maxSuccessDetailBytes {
		value.Details = disposition(OverLimit, AggregateLimit)
	}
	s.detailBytes += len(value.Details)
	attempt.NativeError = value
}

func validFacts(values ...string) bool {
	size := 0
	for _, value := range values {
		size += len(value)
		if size > maxEssentialBytes || !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func copyError(value *NativeError) *NativeError {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Code = bytes.Clone(value.Code)
	copy.Details = bytes.Clone(value.Details)
	return &copy
}

func summary(value *NativeError) *NativeError {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Code = bytes.Clone(value.Code)
	copy.Details = nil
	return &copy
}
