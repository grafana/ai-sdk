package v4

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocation_RejectedUnaryOutcomeBeforePublication(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	observations := make(chan fallback.Attempt, 1)
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		close(entered)
		<-release
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	switch model := h.resolver.resolved.Model.(type) {
	case *fallback.Model:
		model.WithAttemptObserver(func(_ context.Context, attempt fallback.Attempt) { observations <- attempt })
	default:
		require.FailNow(t, "expected configured fallback")
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx, call := newInvocation(parent, "alias", h.resolver.resolved, nil, provider.CallOptions{}, h.handler.limits.UnaryResponseBytes)
	results := make(chan error, 1)
	go func() {
		_, err := h.handler.invokeModel(ctx, h.resolver.resolved.Model, provider.CallOptions{})
		results <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "candidate did not enter")
	}
	cancel()
	var err error
	select {
	case err = <-results:
	case <-time.After(time.Second):
		require.FailNow(t, "cancellation did not return")
	}
	require.ErrorIs(t, err, context.Canceled)
	close(release)
	var attempt fallback.Attempt
	select {
	case attempt = <-observations:
	case <-time.After(time.Second):
		require.FailNow(t, "fallback observation did not arrive")
	}
	assert.Equal(t, fallback.AttemptCanceled, attempt.Outcome)
	require.NotNil(t, attempt.SourceErr)
	assert.Contains(t, attempt.SourceErr.Error(), "handler-rejected native failure")
	assert.Nil(t, call.finish(err))
	call.mu.Lock()
	defer call.mu.Unlock()
	assert.Empty(t, call.decisions)
	assert.Zero(t, second.callCount())
}

func TestInvocation_RejectRecordedFallbackOutcome(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		cancel()
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	ctx, call := newInvocation(parent, "alias", h.resolver.resolved, nil, provider.CallOptions{}, h.handler.limits.UnaryResponseBytes)
	_, err := h.resolver.resolved.Model.DoGenerate(ctx, provider.CallOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, call.decisions, 1)
	require.NotNil(t, call.decisions[0].SourceErr)
	call.reject(ctx.Err())
	assert.Nil(t, call.finish(ctx.Err()))
	assert.Empty(t, call.decisions)
	assert.True(t, call.sealed)
	assert.Zero(t, second.callCount())
}
