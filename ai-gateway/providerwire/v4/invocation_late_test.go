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

func TestInvocation_CanceledLateFallbackObservation(t *testing.T) {
	calls := make(chan *invocation, 1)
	release := make(chan struct{})
	observed := make(chan struct{})
	first := &recordingModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
		calls <- invocationFromContext(ctx)
		<-release
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "unowned late native failure", StatusCode: 401})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	switch model := h.resolver.resolved.Model.(type) {
	case *fallback.Model:
		model.WithAttemptObserver(func(context.Context, fallback.Attempt) { close(observed) })
	default:
		require.FailNow(t, "expected configured fallback")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	responses := make(chan string, 1)
	go func() { responses <- h.serve(validRequest(`{"prompt":[]}`).WithContext(ctx)).Body.String() }()
	var call *invocation
	select {
	case call = <-calls:
	case <-time.After(time.Second):
		require.FailNow(t, "candidate did not enter")
	}
	cancel()
	select {
	case body := <-responses:
		assert.NotContains(t, body, "unowned late native failure")
	case <-time.After(time.Second):
		require.FailNow(t, "cancellation did not return")
	}
	close(release)
	select {
	case <-observed:
	case <-time.After(time.Second):
		require.FailNow(t, "fallback observation did not arrive")
	}
	call.mu.Lock()
	defer call.mu.Unlock()
	assert.True(t, call.sealed)
	assert.Empty(t, call.decisions)
	assert.Nil(t, call.overview)
	assert.Zero(t, second.callCount())
}
