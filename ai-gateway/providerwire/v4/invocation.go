package v4

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

type invocationKey struct{}

type invocation struct {
	mu          sync.Mutex
	requested   string
	canonical   string
	candidates  []catalog.ConfiguredCandidate
	sources     []string
	sourceBytes int64
	decisions   []fallback.Attempt
	entered     bool
	sealed      bool
	overview    *execution.Overview
}

func newInvocation(ctx context.Context, requested string, resolved catalog.ResolvedModel, headers http.Header, options provider.CallOptions, sourceBytes int64) (context.Context, *invocation) {
	value := &invocation{requested: requested, canonical: resolved.ID, candidates: append([]catalog.ConfiguredCandidate(nil), resolved.Candidates...), sources: append(append(append([]string(nil), resolved.ProtectedSources...), execution.HeaderSources(headers)...), requestProtectedSources(options)...), sourceBytes: sourceBytes}
	ctx = context.WithValue(ctx, invocationKey{}, value)
	return fallback.WithAttemptObserver(ctx, value.observe), value
}

func invocationFromContext(ctx context.Context) *invocation {
	value, _ := ctx.Value(invocationKey{}).(*invocation)
	return value
}

func (value *invocation) observe(_ context.Context, decision fallback.Attempt) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if !value.sealed {
		value.decisions = append(value.decisions, decision)
	}
}

func (value *invocation) enter() {
	if value == nil {
		return
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if !value.sealed {
		value.entered = true
	}
}

func (value *invocation) finish(err error) *execution.Overview {
	if value == nil {
		return nil
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	return value.finishLocked(err)
}

func (value *invocation) reject(err error) {
	if value == nil {
		return
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	value.decisions = nil
	value.finishLocked(err)
}

func (value *invocation) finishLocked(err error) *execution.Overview {
	if value.sealed {
		return value.overview
	}
	value.sealed = true
	defer func() {
		value.decisions = nil
		if recover() != nil {
			value.overview = nil
		}
	}()
	if len(value.decisions) == 0 && len(value.candidates) == 1 && value.entered {
		outcome := fallback.AttemptSelected
		if err != nil {
			outcome = fallback.AttemptFailed
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				outcome = fallback.AttemptCanceled
			}
		}
		value.decisions = []fallback.Attempt{{Index: 1, Outcome: outcome, SourceErr: err}}
	}
	value.overview = execution.Project(value.requested, value.canonical, value.decisions, value.candidates, value.sources, value.sourceBytes)
	value.decisions = nil
	return value.overview
}

func (value *invocation) current(err *provider.APICallError) *execution.Failure {
	if value == nil || err == nil {
		return nil
	}
	return execution.Summary(err, value.sources, value.sourceBytes)
}

func (value safeError) withCurrentInvocation(call *invocation, err *provider.APICallError) safeError {
	value.invocation = call
	value.nativeError = err
	return value
}

func errorForStreamWait(result streamWaitResult) error {
	switch result {
	case streamWaitCanceled:
		return context.Canceled
	case streamWaitTotalTimeout, streamWaitIdleTimeout:
		return context.DeadlineExceeded
	default:
		return errModelInternal
	}
}

func (value safeError) withInvocation(call *invocation, err error) safeError {
	value.invocation = call
	if call != nil {
		call.finish(err)
	}
	return value
}
