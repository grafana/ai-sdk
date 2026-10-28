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
	requested string
	canonical string
	groups    []modelGroup
	redactor  SummaryRedactor

	mu       sync.Mutex
	entered  bool
	sealed   bool
	attempts []fallback.Attempt
}

// newAttemptCapture prepares the capture for selected. Every selection is
// captured, and only selections with model groups are projected into an
// overview.
func newAttemptCapture(requested string, selected Selection, redactor SummaryRedactor) *attemptCapture {
	attempts := &attemptCapture{redactor: redactor}
	if len(selected.groups) != 0 {
		attempts.requested = requested
		attempts.groups = cloneGroups(selected.groups)
		if selected.groups[0].accounts == nil {
			attempts.canonical = selected.ID
		}
	}
	return attempts
}

func cloneGroups(groups []modelGroup) []modelGroup {
	cloned := make([]modelGroup, len(groups))
	for i, group := range groups {
		cloned[i] = group
		cloned[i].candidates = slices.Clone(group.candidates)
		if group.accounts != nil {
			accounts := *group.accounts
			cloned[i].accounts = &accounts
		}
	}
	return cloned
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
// or succeeded when err is nil. Attempts observed afterwards are ignored.
func (a *attemptCapture) view(err error) (view View) {
	entered, attempts := a.seal()
	view = View{redactor: a.redactor}
	defer func() {
		if recover() != nil {
			view.Overview = nil
		}
	}()
	if len(attempts) == 0 && configuredCandidateCount(a.groups) == 1 && entered {
		outcome := fallback.AttemptSelected
		if err != nil {
			outcome = fallback.AttemptFailed
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				outcome = fallback.AttemptCanceled
			}
		}
		attempts = []fallback.Attempt{{Index: 1, Outcome: outcome, SourceErr: err}}
	}
	view.Overview = projectGroups(a.requested, a.canonical, attempts, a.groups)
	if view.Overview != nil {
		for i := range view.Overview.Attempts {
			attempt := &view.Overview.Attempts[i]
			attempt.Error = a.redactor.Redact(attempt.Error)
		}
		view.Routing = projectRouting(view.Overview, a.groups)
	}
	return view
}

// View is the execution evidence an adapter may encode for one call.
type View struct {
	// Overview is the published attempt overview, or nil when none is published.
	Overview *Overview
	// Routing groups the overview's attempts by model, or is nil when they
	// cannot be assigned to the call's models.
	Routing  *Routing
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
	return view.Enrich(nil)
}

// Enrich returns original with the view's execution and routing metadata, or
// nil when the view publishes no overview.
func (view View) Enrich(original provider.ProviderMetadata) provider.ProviderMetadata {
	return Metadata(view.Overview, view.Routing, original)
}
