package v4

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

type attemptCapture struct {
	mu       sync.Mutex
	entered  bool
	sealed   bool
	attempts []fallback.Attempt
}

func (capture *attemptCapture) observe(_ context.Context, attempt fallback.Attempt) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.sealed {
		capture.attempts = append(capture.attempts, attempt)
	}
}

func (capture *attemptCapture) enter() {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.sealed {
		capture.entered = true
	}
}

func (capture *attemptCapture) seal() (bool, []fallback.Attempt) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.sealed = true
	attempts := capture.attempts
	capture.attempts = nil
	return capture.entered, attempts
}

func (capture *attemptCapture) discard() {
	capture.seal()
}

type executionRequest struct {
	requested   string
	canonical   string
	candidates  []catalog.ConfiguredCandidate
	sourceBytes int64
}

func newExecutionRequest(requested string, selected Selection, sourceBytes int64) executionRequest {
	request := executionRequest{sourceBytes: sourceBytes}
	if selected.configured != nil {
		request.requested = requested
		request.canonical = selected.ID
		request.candidates = slices.Clone(selected.configured.candidates)
	}
	return request
}

func (request executionRequest) snapshot(capture *attemptCapture, err error) (view executionView) {
	entered, attempts := capture.seal()
	view = executionView{sourceBytes: request.sourceBytes}
	defer func() {
		if recover() != nil {
			view.overview = nil
		}
	}()
	if len(attempts) == 0 && len(request.candidates) == 1 && entered {
		outcome := fallback.AttemptSelected
		if err != nil {
			outcome = fallback.AttemptFailed
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				outcome = fallback.AttemptCanceled
			}
		}
		attempts = []fallback.Attempt{{Index: 1, Outcome: outcome, SourceErr: err}}
	}
	view.overview = execution.Project(request.requested, request.canonical, attempts, request.candidates, request.sourceBytes)
	return view
}

type executionView struct {
	overview    *execution.Overview
	sourceBytes int64
}

func (view executionView) current(err *provider.APICallError) *execution.Failure {
	if err == nil || view.sourceBytes == 0 {
		return nil
	}
	return execution.Summary(err, view.sourceBytes)
}

func (view executionView) metadata() provider.ProviderMetadata {
	return execution.Metadata(view.overview, nil)
}
