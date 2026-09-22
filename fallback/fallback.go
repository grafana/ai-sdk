package fallback

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

// ErrNoCandidates is returned by New when no candidates are provided.
var ErrNoCandidates = errors.New("fallback: at least one candidate is required")

// ErrInvalidResult indicates a candidate returned no usable result without an error.
var ErrInvalidResult = errors.New("fallback: invalid candidate result")

// ErrPrematureStreamEnd indicates a candidate closed before producing any part.
var ErrPrematureStreamEnd = errors.New("fallback: stream ended before first part")

// Never retain or format the recovered value: provider panics may contain secrets.
var errStreamSetupPanic = errors.New("fallback: candidate stream setup panicked")

const (
	cleanupTimeout    = 100 * time.Millisecond
	cleanupPartBudget = 256
)

// AttemptOutcome describes the selection decision for a physical invocation.
type AttemptOutcome string

const (
	// AttemptSelected means a unary result or the first stream part was accepted.
	AttemptSelected AttemptOutcome = "selected"
	// AttemptFailed means the invocation failed before selection.
	AttemptFailed AttemptOutcome = "failed"
	// AttemptCanceled means the request ended before selection.
	AttemptCanceled AttemptOutcome = "canceled"
)

// Model wraps an ordered list of LanguageModel candidates and itself
// implements LanguageModel. It tries candidates in order, falling back
// on errors according to the configured decider.
type Model struct {
	candidates []provider.LanguageModel
	decider    func(error) bool
	observer   AttemptObserver
}

// Attempt describes one candidate invocation made by a fallback model.
type Attempt struct {
	Index      int
	Provider   string
	ModelID    string
	StartedAt  time.Time
	FinishedAt time.Time
	Err        error
	Outcome    AttemptOutcome
	// WillFallback records eligibility and a remaining candidate while the context
	// is live at decision time. Later cancellation may prevent that invocation;
	// subsequent attempt records establish which candidates were actually invoked.
	WillFallback bool
}

// AttemptObserver observes a candidate's selection decision synchronously.
// It must return promptly. Observer panics are recovered.
type AttemptObserver func(context.Context, Attempt)

// New creates a FallbackModel from the given candidates.
// Returns ErrNoCandidates if no candidates are provided.
func New(candidates ...provider.LanguageModel) (*Model, error) {
	if len(candidates) == 0 {
		return nil, ErrNoCandidates
	}
	return &Model{
		candidates: append([]provider.LanguageModel(nil), candidates...),
		decider:    defaultDecider,
	}, nil
}

// WithDecider sets a custom fallback decider. The decider returns true
// if the error should trigger fallback to the next candidate.
func (m *Model) WithDecider(fn func(error) bool) *Model {
	m.decider = fn
	return m
}

// WithAttemptObserver registers a callback invoked after each candidate
// attempt. Index is one-based. For streams, an attempt finishes when the first
// part arrives, the stream closes, or the provider returns an error. FinishedAt
// is decision time, not stream completion. The callback must return promptly.
func (m *Model) WithAttemptObserver(fn AttemptObserver) *Model {
	m.observer = fn
	return m
}

func (m *Model) SpecificationVersion() string               { return m.candidates[0].SpecificationVersion() }
func (m *Model) Provider() string                           { return m.candidates[0].Provider() }
func (m *Model) ModelID() string                            { return m.candidates[0].ModelID() }
func (m *Model) SupportedURLs() map[string][]*regexp.Regexp { return m.candidates[0].SupportedURLs() }

func (m *Model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	var failures []failedAttempt
	for i, c := range m.candidates {
		if contextErr := ctx.Err(); contextErr != nil {
			if len(failures) == 0 {
				return nil, contextErr
			}
			return nil, errors.Join(failedAttemptsError(failures), contextErr)
		}
		startedAt := time.Now()
		result, err := c.DoGenerate(ctx, params)
		if err == nil && result == nil {
			err = ErrInvalidResult
		}
		outcome, next, err := m.decision(ctx, i, err)
		m.observeAttempt(ctx, i, c, startedAt, err, outcome, next)
		if err == nil {
			return result, nil
		}
		failures = append(failures, failedAttempt{candidate: c, err: err})
		if !next {
			return nil, failedAttemptsError(failures)
		}
	}
	return nil, failedAttemptsError(failures)
}

func (m *Model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	var failures []failedAttempt
	for i, c := range m.candidates {
		if ctx.Err() != nil {
			return nil, errors.Join(failedAttemptsError(failures), ctx.Err())
		}
		startedAt := time.Now()
		candidateCtx, cancel := context.WithCancel(ctx)
		result, err := startStream(candidateCtx, c, params)
		var first provider.StreamPart
		if err == nil && (result == nil || result.Stream == nil) {
			err = ErrInvalidResult
		}
		if err == nil {
			select {
			case part, ok := <-result.Stream:
				if !ok {
					err = ErrPrematureStreamEnd
				} else {
					first = part
				}
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		outcome, next, err := m.decision(ctx, i, err)
		if err != nil {
			cancel()
			if result != nil && result.Stream != nil {
				go drainStream(result.Stream)
			}
			m.observeAttempt(ctx, i, c, startedAt, err, outcome, next)
			failures = append(failures, failedAttempt{candidate: c, err: err})
			if !next {
				return nil, failedAttemptsError(failures)
			}
			continue
		}
		m.observeAttempt(ctx, i, c, startedAt, nil, outcome, false)
		ch := make(chan provider.StreamPart, 64)
		go func() {
			defer close(ch)
			defer cancel()
			part := first
			for {
				if candidateCtx.Err() != nil {
					drainStream(result.Stream)
					return
				}
				select {
				case ch <- part:
				case <-candidateCtx.Done():
					drainStream(result.Stream)
					return
				}
				select {
				case nextPart, ok := <-result.Stream:
					if !ok {
						return
					}
					part = nextPart
				case <-candidateCtx.Done():
					drainStream(result.Stream)
					return
				}
			}
		}()
		return &provider.StreamResult{
			Stream:   ch,
			Request:  result.Request,
			Response: result.Response,
		}, nil
	}
	return nil, failedAttemptsError(failures)
}

func startStream(ctx context.Context, candidate provider.LanguageModel, params provider.CallOptions) (*provider.StreamResult, error) {
	type setup struct {
		result *provider.StreamResult
		err    error
	}
	ready := make(chan setup)
	go func() {
		// Recovery must belong to this worker, not its caller's goroutine. Feed
		// failures through the same ownership transfer and cancellation path.
		result, err := func() (result *provider.StreamResult, err error) {
			defer func() {
				if recover() != nil {
					result, err = nil, errStreamSetupPanic
				}
			}()
			return candidate.DoStream(ctx, params)
		}()
		select {
		case ready <- setup{result, err}:
		case <-ctx.Done():
			if result != nil && result.Stream != nil {
				drainStream(result.Stream)
			}
		}
	}()
	select {
	case value := <-ready:
		return value.result, value.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func drainStream(stream <-chan provider.StreamPart) {
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()
	for range cleanupPartBudget {
		select {
		case _, ok := <-stream:
			if !ok {
				return
			}
		case <-timer.C:
			return
		}
	}
}

func (m *Model) decision(ctx context.Context, index int, err error) (AttemptOutcome, bool, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return AttemptCanceled, false, contextErr
	}
	if err == nil {
		return AttemptSelected, false, nil
	}
	eligible := m.decider(err)
	if contextErr := ctx.Err(); contextErr != nil {
		return AttemptCanceled, false, contextErr
	}
	return AttemptFailed, eligible && index+1 < len(m.candidates), err
}

type failedAttempt struct {
	candidate provider.LanguageModel
	err       error
}

func (m *Model) observeAttempt(ctx context.Context, index int, candidate provider.LanguageModel, startedAt time.Time, err error, outcome AttemptOutcome, next bool) {
	if m.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	m.observer(ctx, Attempt{
		Index:        index + 1,
		Provider:     candidate.Provider(),
		ModelID:      candidate.ModelID(),
		StartedAt:    startedAt,
		FinishedAt:   time.Now(),
		Err:          err,
		Outcome:      outcome,
		WillFallback: next,
	})
}

func failedAttemptsError(failures []failedAttempt) error {
	if len(failures) == 0 {
		return nil
	}
	if len(failures) == 1 {
		return failures[0].err
	}

	errs := make([]error, 0, len(failures))
	for i := len(failures) - 1; i >= 0; i-- {
		failure := failures[i]
		errs = append(errs, fmt.Errorf("fallback: provider %q model %q: %w", failure.candidate.Provider(), failure.candidate.ModelID(), failure.err))
	}
	return errors.Join(errs...)
}

// defaultDecider returns true (try next candidate) for retryable API errors
// and unknown errors. Returns false (stop) for non-retryable API errors. It also
// returns false for context-window/context-length failures, because the next
// candidate would fail identically.
func defaultDecider(err error) bool {
	var apiErr *provider.APICallError
	if errors.As(err, &apiErr) {
		if isContextWindowError(apiErr) {
			return false
		}
		return apiErr.IsRetryable
	}
	return true
}

// contextLengthSignal matches the common ways providers phrase a context-window
// overflow. This heuristic is intentionally confined to the fallback decider --
// upstream has no context-window error category, and we do not expose one.
var contextLengthSignal = regexp.MustCompile(`(?i)context (length|window)|maximum context|too many tokens|prompt is too long`)

// isContextWindowError reports whether apiErr represents a context-window/
// context-length overflow. Anthropic surfaces this as an invalid_request_error
// at HTTP 400 with a context-length message rather than a dedicated type, so we
// match on status plus the structured Data/Message signal.
func isContextWindowError(apiErr *provider.APICallError) bool {
	if apiErr.StatusCode != 400 {
		return false
	}
	if contextLengthSignal.MatchString(apiErr.Message) {
		return true
	}
	if len(apiErr.Data) > 0 && contextLengthSignal.Match(apiErr.Data) {
		return true
	}
	return false
}
