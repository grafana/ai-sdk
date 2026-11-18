package customertelemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestDelivery_OTLPBackgroundRevocation(t *testing.T) {
	r := &otlpTestReceiver{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.serve(t, w, req) }))
	defer server.Close()
	var rotated, disabled atomic.Bool
	a, b := testStack(t, "A"), testStack(t, "B")
	resolver := &cacheTestResolver{
		destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			version := "v1"
			if s == a && rotated.Load() {
				version = "v2"
			}
			return NewDestinations(s, DestinationInput{Namespace: s.Namespace(), Revision: version, ValidUntil: now.Add(time.Minute), Signals: map[Signal]Destination{
				SignalTraces: {Endpoint: server.URL + "/v1/traces", IngestionIdentity: s.Namespace() + "-trace-tenant", Credential: CredentialRef{Name: "test-reference", Version: version}},
			}}, now)
		},
		policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			preference := PreferenceAbsent
			if s == a && disabled.Load() {
				preference = PreferenceDisabled
			}
			return cachePolicy(s, now, time.Minute, preference)
		},
	}
	po := poolTestOptions()
	po.BuildTimeout = 3 * time.Second
	p, err := NewPool(otlpTestFactory(t, server, 8, 8, false), po)
	require.NoError(t, err)
	d, err := NewDelivery(SignalTraces, p)
	require.NoError(t, err)
	co := cacheTestOptions()
	co.RefreshInterval, co.RetryInterval, co.MaintenanceInterval = 20*time.Millisecond, 10*time.Millisecond, 5*time.Millisecond
	c, err := NewController(resolver, resolver, co, d)
	require.NoError(t, err)
	defer func() { assert.NoError(t, c.Close(context.Background())) }()
	acquire := func(s Stack) *Lease[*sdktrace.TracerProvider] {
		l, _, acquireErr := d.Acquire(context.Background(), s)
		require.NoError(t, acquireErr)
		return l
	}
	old, other := acquire(a), acquire(b)
	defer old.Release()
	defer other.Release()
	otlpTestSpan(old.Value(), "authorized-before-revocation")
	otlpTestFlush(t, old.Value())
	rotated.Store(true)
	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		binding := p.current[poolKey{a, SignalTraces}]
		return binding != nil && binding.target.Destination.Credential.Version == "v2"
	}, 3*time.Second, time.Millisecond)
	current := acquire(a)
	defer current.Release()
	otlpTestSpan(old.Value(), "retiring-queued")
	otlpTestSpan(current.Value(), "current-queued")
	otlpTestSpan(other.Value(), "other-stack")
	disabled.Store(true)
	require.Eventually(t, func() bool {
		return !old.binding.gate.valid() && !current.binding.gate.valid()
	}, 3*time.Second, time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.Error(t, old.Value().ForceFlush(ctx))
	assert.Error(t, current.Value().ForceFlush(ctx))
	otlpTestFlush(t, other.Value())
	requests := r.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, otlpTestRequest{user: "A-trace-tenant", password: "dummy-v1", path: "/v1/traces", spans: []string{"authorized-before-revocation"}}, requests[0])
	assert.Equal(t, otlpTestRequest{user: "B-trace-tenant", password: "dummy-v1", path: "/v1/traces", spans: []string{"other-stack"}}, requests[1])
}
