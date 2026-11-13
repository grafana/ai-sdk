package execution

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

// attemptCapture records one call's fallback attempts until its outcome is decided.
type attemptCapture struct {
	requested   string
	canonical   string
	attribution attribution
	expected    []ExpectedAttempt
	redactor    SummaryRedactor

	mu       sync.Mutex
	entered  bool
	sealed   bool
	attempts []fallback.Attempt
	served   string
	overview *Overview
}

// newAttemptCapture prepares the capture for selected. Every selection is
// captured, and only selections with attribution are projected into an
// overview.
func newAttemptCapture(requested string, selected Selection, redactor SummaryRedactor) *attemptCapture {
	return &attemptCapture{requested: requested, canonical: selected.ID, attribution: selected.attribution, expected: selected.expected, redactor: redactor}
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

// seal ends observation. The first seal returns the observed attempts; later
// seals return none, so the call's outcome is projected once. The attempts
// stay readable through observed.
func (a *attemptCapture) seal() (bool, []fallback.Attempt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sealed {
		return a.entered, nil
	}
	a.sealed = true
	return a.entered, a.attempts
}

// observed returns whether the model was entered and a copy of the attempts
// observed before sealing, without sealing.
func (a *attemptCapture) observed() (bool, []fallback.Attempt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.entered, slices.Clone(a.attempts)
}

// discard seals the capture without projecting it.
func (a *attemptCapture) discard() {
	a.seal()
}

// view seals the capture and projects it for an outcome that failed with err,
// or succeeded when err is nil or [ErrInvalidOutput]. An extension outcome
// after entry can only refuse output the selected model returned, so it also
// projects as success. Attempts observed afterwards are ignored.
func (a *attemptCapture) view(err error) View {
	entered, attempts := a.seal()
	overview := a.project(entered, attempts, err)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.overview = overview
	a.applyServed()
	return View{Overview: overview, redactor: a.redactor}
}

// current returns the attempts so far, projected for err like the published
// overview, including the model that served the selected output, without
// sealing the capture or publishing an overview.
func (a *attemptCapture) current(err error) *Overview {
	entered, attempts := a.observed()
	overview := a.project(entered, attempts, err)
	a.mu.Lock()
	defer a.mu.Unlock()
	markServed(overview, a.served)
	return overview
}

// project returns the overview of attempts for an outcome that failed with
// err, with failure summaries redacted, or nil when none can be published. A
// single expected configured candidate that was entered without an observed
// attempt gets one synthesized attempt.
func (a *attemptCapture) project(entered bool, attempts []fallback.Attempt, err error) (overview *Overview) {
	defer func() {
		if recover() != nil {
			overview = nil
		}
	}()
	if len(attempts) == 0 && a.attribution == attributionConfigured && len(a.expected) == 1 && entered {
		outcome, source := fallback.AttemptSelected, error(nil)
		if err != nil && !errors.Is(err, ErrInvalidOutput) && !isExtensionOutcome(err) {
			outcome, source = fallback.AttemptFailed, err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				outcome = fallback.AttemptCanceled
			}
		}
		attempts = []fallback.Attempt{{Index: 1, Outcome: outcome, SourceErr: source}}
	}
	overview = projectExpected(a.requested, a.canonical, attempts, a.attribution, a.expected)
	if overview != nil {
		for i := range overview.Attempts {
			attempt := &overview.Attempts[i]
			attempt.Error = a.redactor.Redact(attempt.Error)
		}
	}
	return overview
}

// serve records the model that served the selected output. It updates the
// published overview in place, so every later carrier of the view reports it,
// without changing the attempts.
func (a *attemptCapture) serve(model string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.served = model
	a.applyServed()
}

func (a *attemptCapture) applyServed() { markServed(a.overview, a.served) }

// markServed records served on overview's selected attempt, when there is one.
func markServed(overview *Overview, served string) {
	if overview == nil || served == "" {
		return
	}
	if last := &overview.Attempts[len(overview.Attempts)-1]; last.Outcome == fallback.AttemptSelected {
		last.ServedModelID = served
	}
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
