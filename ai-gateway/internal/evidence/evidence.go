package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const (
	MaxAttempts        = 16
	SourceBytes        = 1 << 20
	ComponentBytes     = 128 << 10
	EssentialBytes     = 32 << 10
	ErrorDetailBytes   = 24 << 10
	SuccessDetailBytes = 256 << 10
	ErrorBytes         = 64 << 10
)

type Completion string

const (
	Completed  Completion = "completed"
	Incomplete Completion = "incomplete"
)

type Phase string

const (
	Unary     Phase = "unary"
	Setup     Phase = "setup"
	Committed Phase = "committed"
)

type ReplayRisk string

const (
	GenerationMayHaveRun ReplayRisk = "generationMayHaveRun"
	GenerationCompleted  ReplayRisk = "generationCompleted"
)

type DispositionState string

const (
	Available   DispositionState = "available"
	Unavailable DispositionState = "unavailable"
	Redacted    DispositionState = "redacted"
	Malformed   DispositionState = "malformed"
	OverLimit   DispositionState = "over-limit"
)

type Reason string

const (
	ProducerDoesNotExpose Reason = "producerDoesNotExpose"
	CredentialSource      Reason = "credentialSource"
	InvalidJSON           Reason = "invalidJSON"
	SourceLimit           Reason = "sourceBytes"
	EncodedLimit          Reason = "encodedBytes"
	AggregateLimit        Reason = "aggregateBytes"
	AttemptLimit          Reason = "attemptCount"
)

type Component struct {
	State    DispositionState `json:"state"`
	Value    json.RawMessage  `json:"value,omitempty"`
	Redacted bool             `json:"redacted,omitempty"`
	Reason   Reason           `json:"reason,omitempty"`

	encodedBytes int
}

type NativeError struct {
	Message     string          `json:"message,omitempty"`
	Type        string          `json:"type,omitempty"`
	Code        json.RawMessage `json:"code,omitempty"`
	StatusCode  int             `json:"statusCode,omitempty"`
	IsRetryable bool            `json:"isRetryable"`
	Details     *Component      `json:"details,omitempty"`
}

type Attempt struct {
	Index            int                     `json:"index"`
	Provider         string                  `json:"provider"`
	ModelID          string                  `json:"modelId"`
	ProviderInstance string                  `json:"providerInstance,omitempty"`
	Selection        fallback.AttemptOutcome `json:"selection"`
	Completion       Completion              `json:"completion,omitempty"`
	WillFallback     *bool                   `json:"willFallback,omitempty"`
	NativeError      *NativeError            `json:"nativeError,omitempty"`
}

type Identity struct {
	ID        string `json:"id,omitempty"`
	ModelID   string `json:"modelId,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type native struct {
	ResponseIdentity Identity `json:"responseIdentity"`
}

type Classification string

const (
	ClassificationInvalidRequest   Classification = "invalid_request"
	ClassificationModelNotFound    Classification = "model_not_found"
	ClassificationRateLimit        Classification = "rate_limit_exceeded"
	ClassificationOverloaded       Classification = "overloaded"
	ClassificationFailedDependency Classification = "failed_dependency"
	ClassificationUpstream         Classification = "upstream_error"
	ClassificationTimeout          Classification = "timeout"
	ClassificationCanceled         Classification = "canceled"
	ClassificationInternal         Classification = "internal_error"
)

type Gateway struct {
	HTTPStatusCode int            `json:"httpStatusCode"`
	Classification Classification `json:"classification"`
	Phase          Phase          `json:"phase"`
	IsRetryable    bool           `json:"isRetryable"`
	ReplayRisk     ReplayRisk     `json:"replayRisk,omitempty"`
}

type snapshot struct {
	RequestedModelID string   `json:"requestedModelId"`
	CanonicalModelID string   `json:"canonicalModelId"`
	SelectedAttempt  int      `json:"selectedAttempt,omitempty"`
	Attempts         any      `json:"attempts"`
	Native           *native  `json:"native,omitempty"`
	Gateway          *Gateway `json:"gateway,omitempty"`
}

type routing struct {
	OriginalModelID string `json:"originalModelId"`
	CanonicalSlug   string `json:"canonicalSlug"`
	FinalProvider   string `json:"finalProvider,omitempty"`
}

type namespace struct {
	Routing        routing         `json:"routing"`
	Evidence       snapshot        `json:"evidence"`
	NativeMetadata json.RawMessage `json:"nativeMetadata,omitempty"`
}

type contextKey struct{}

type State struct {
	mu                   sync.Mutex
	requested, canonical string
	attempts             []Attempt
	count, selected      int
	selectedProvider     string
	currentProvider      string
	identity             *native
	currentError         *NativeError
	protectedSources     []string
	completed            bool
	sealed, invalid      bool
}

func New(ctx context.Context, requested, canonical string) (context.Context, *State) {
	s := &State{requested: requested, canonical: canonical, attempts: make([]Attempt, 0, MaxAttempts)}
	if len(requested)+len(canonical) > EssentialBytes {
		s.invalid = true
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
	return s.count != 0
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
	s.count++
	if s.invalid {
		return s.count
	}
	if len(candidate.Provider)+len(candidate.ModelID)+len(candidate.ProviderInstance) > EssentialBytes || !utf8.ValidString(candidate.Provider) || !utf8.ValidString(candidate.ModelID) || !utf8.ValidString(candidate.ProviderInstance) {
		s.invalid = true
		return s.count
	}
	s.currentProvider = candidate.Provider
	if s.count > MaxAttempts {
		s.attempts = nil
		return s.count
	}
	s.attempts = append(s.attempts, Attempt{Index: s.count, Provider: candidate.Provider, ModelID: candidate.ModelID, ProviderInstance: candidate.ProviderInstance, Selection: fallback.AttemptCanceled})
	s.enforceEssentialLocked()
	return s.count
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
		s.completed = true
	}
	if s.invalid {
		return
	}
	if s.count > MaxAttempts {
		if !streaming && result != nil && err == nil {
			if !ordered {
				s.selectLocked(index)
			}
			if result.Response != nil {
				s.setIdentityLocked(result.Response.ID, result.Response.ModelID, result.Response.Timestamp)
			}
		}
		return
	}
	if index > len(s.attempts) {
		return
	}
	a := &s.attempts[index-1]
	if err != nil {
		a.Selection = fallback.AttemptFailed
		value, invalid := projectError(err, s.protectedSources...)
		s.retainErrorLocked(a, value)
		s.invalid = s.invalid || invalid
	} else if !streaming && result != nil {
		a.Completion = Completed
		if !ordered {
			s.selectLocked(index)
		}
		if result.Response != nil {
			s.setIdentityLocked(result.Response.ID, result.Response.ModelID, result.Response.Timestamp)
		}
	} else if !streaming {
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
	if index <= len(s.attempts) {
		s.attempts[index-1].Selection = fallback.AttemptCanceled
	}
	if s.selected == index {
		s.selected = 0
		s.selectedProvider = ""
	}
}

func ObserveDecision(ctx context.Context, decision fallback.Attempt) {
	s := FromContext(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed {
		return
	}
	if decision.Outcome == fallback.AttemptSelected {
		s.selectLocked(decision.Index)
	}
	if decision.Index > len(s.attempts) {
		return
	}
	a := &s.attempts[decision.Index-1]
	a.Selection = decision.Outcome
	will := decision.WillFallback
	a.WillFallback = &will
}

func (s *State) selectLocked(index int) {
	s.selected = index
	if index == s.count {
		s.selectedProvider = s.currentProvider
	}
	if index <= len(s.attempts) {
		s.attempts[index-1].Selection = fallback.AttemptSelected
		s.selectedProvider = s.attempts[index-1].Provider
		s.completed = s.attempts[index-1].Completion == Completed
	}
}

func (s *State) ObservePart(part provider.StreamPart) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sealed || s.count == 0 {
		return
	}
	if s.selected == 0 {
		s.selectLocked(s.count)
	}
	if part.Type == provider.PartResponseMeta {
		s.setIdentityLocked(part.ResponseID, part.ModelID, part.Timestamp)
	}
	if part.Type == provider.PartFinish && part.FinishReason != nil {
		s.completed = true
	}
	if s.selected > len(s.attempts) {
		if part.Type == provider.PartError && !s.invalid {
			value, invalid := projectError(part.APICallError, s.protectedSources...)
			if value != nil {
				value.Details = nil
			}
			s.currentError = value
			s.invalid = invalid
			s.enforceEssentialLocked()
		}
		return
	}
	a := &s.attempts[s.selected-1]
	if part.Type == provider.PartError && !s.invalid {
		value, invalid := projectError(part.APICallError, s.protectedSources...)
		s.currentError = value
		s.retainErrorLocked(a, value)
		s.invalid = s.invalid || invalid
	}
	if part.Type == provider.PartFinish && part.FinishReason != nil {
		a.Completion = Completed
		s.completed = true
	}
}

func (s *State) setIdentityLocked(id, model string, timestamp time.Time) {
	if len(id)+len(model) > EssentialBytes || !utf8.ValidString(id) || !utf8.ValidString(model) {
		s.invalid = true
		return
	}
	identity := Identity{ID: id, ModelID: model}
	if !timestamp.IsZero() {
		identity.Timestamp = timestamp.UTC().Format(time.RFC3339Nano)
	}
	if id != "" || model != "" || identity.Timestamp != "" {
		s.identity = &native{ResponseIdentity: identity}
		s.enforceEssentialLocked()
	}
}

func (s *State) Metadata(original provider.ProviderMetadata, gateway *Gateway, minimal bool) (provider.ProviderMetadata, error) {
	if s == nil {
		return original, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return original, nil
	}
	if s.invalid {
		return nil, errors.New("gateway evidence: essential values exceed allocation or are malformed")
	}
	value := s.essentialNamespaceLocked(gateway)
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > EssentialBytes {
		return nil, errors.New("gateway evidence: essential encoded allocation exceeded")
	}
	remaining := SuccessDetailBytes
	if gateway != nil {
		remaining = ErrorDetailBytes
	}
	if attempts, ok := value.Evidence.Attempts.([]Attempt); ok {
		for i := range attempts {
			if attempts[i].NativeError == nil {
				continue
			}
			details := s.attempts[i].NativeError.Details
			if details != nil {
				size, err := details.encodedSize()
				if err != nil {
					return nil, err
				}
				if (minimal && details.State == Available) || size > remaining {
					details = &Component{State: OverLimit, Reason: AggregateLimit}
				} else {
					remaining -= size
				}
			}
			attempts[i].NativeError.Details = details
		}
	}
	value.NativeMetadata = original["gateway"]
	encoded, err = json.Marshal(value)
	if err != nil {
		return nil, err
	}
	metadata := make(provider.ProviderMetadata, len(original)+1)
	for key, raw := range original {
		metadata[key] = raw
	}
	metadata["gateway"] = encoded
	return metadata, nil
}

func (s *State) CurrentError() *NativeError {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentError == nil {
		return nil
	}
	copy := *s.currentError
	copy.Details = nil
	return &copy
}
