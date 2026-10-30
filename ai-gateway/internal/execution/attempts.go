package execution

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// attemptCapture records one call's fallback attempts until its outcome is decided.
type attemptCapture struct {
	requested  string
	canonical  string
	candidates []catalog.ConfiguredCandidate
	accounts   *RequestAccounts
	redactor   SummaryRedactor

	mu       sync.Mutex
	entered  bool
	sealed   bool
	attempts []fallback.Attempt
}

// newAttemptCapture prepares the capture for selected. Every selection is
// captured, and only configured and request-account selections are projected
// into an overview.
func newAttemptCapture(requested string, selected Selection, redactor SummaryRedactor) *attemptCapture {
	attempts := &attemptCapture{redactor: redactor}
	switch {
	case selected.configured != nil:
		attempts.requested = requested
		attempts.canonical = selected.ID
		attempts.candidates = slices.Clone(selected.configured.candidates)
	case selected.accounts != nil:
		attempts.requested = requested
		attempts.accounts = selected.accounts
	}
	return attempts
}

// withObserver returns ctx with the request-scoped fallback observer attached.
func (a *attemptCapture) withObserver(ctx context.Context) context.Context {
	return fallback.WithAttemptObserver(ctx, a.observe)
}

func (a *attemptCapture) observe(_ context.Context, attempt fallback.Attempt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.sealed {
		a.attempts = append(a.attempts, attempt)
	}
}

// enter records that the model was invoked.
func (a *attemptCapture) enter() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.sealed {
		a.entered = true
	}
}

func (a *attemptCapture) seal() (bool, []fallback.Attempt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sealed = true
	attempts := a.attempts
	a.attempts = nil
	return a.entered, attempts
}

// discard seals the capture without projecting it.
func (a *attemptCapture) discard() {
	a.seal()
}

// view seals the capture and projects it for an outcome that failed with err,
// or succeeded when err is nil or [ErrInvalidOutput]. Attempts observed
// afterwards are ignored.
func (a *attemptCapture) view(err error) (view View) {
	entered, attempts := a.seal()
	view = View{redactor: a.redactor}
	defer func() {
		if recover() != nil {
			view.Overview = nil
		}
	}()
	if len(attempts) == 0 && len(a.candidates) == 1 && entered {
		outcome, source := fallback.AttemptSelected, error(nil)
		if err != nil && !errors.Is(err, ErrInvalidOutput) {
			outcome, source = fallback.AttemptFailed, err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				outcome = fallback.AttemptCanceled
			}
		}
		attempts = []fallback.Attempt{{Index: 1, Outcome: outcome, SourceErr: source}}
	}
	if a.accounts != nil {
		view.Overview = projectAccounts(a.requested, attempts, *a.accounts)
	} else {
		view.Overview = Project(a.requested, a.canonical, attempts, a.candidates)
	}
	if view.Overview != nil {
		for i := range view.Overview.Attempts {
			attempt := &view.Overview.Attempts[i]
			attempt.Error = a.redactor.Redact(attempt.Error)
		}
	}
	return view
}

// View is the execution evidence an adapter may encode for one call.
type View struct {
	// Overview is the published attempt overview, or nil when none is published.
	Overview *Overview
	redactor SummaryRedactor
}

// Current summarizes an event-local native failure, with configured keys
// redacted.
func (view View) Current(err *provider.APICallError) *Failure {
	if err == nil {
		return nil
	}
	return view.redactor.Redact(Summary(err))
}

// Metadata returns the execution metadata for an error document.
func (view View) Metadata() provider.ProviderMetadata {
	return Metadata(view.Overview, nil)
}
