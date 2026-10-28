package customertelemetry

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cacheTestResolver struct {
	destination      func(context.Context, Stack, time.Time) (Destinations, error)
	policy           func(context.Context, Stack, time.Time) (Policy, error)
	destinationCalls atomic.Int64
	policyCalls      atomic.Int64
}

type cacheRequestKey struct{}

func (r *cacheTestResolver) ResolveDestinations(ctx context.Context, stack Stack, now time.Time) (Destinations, error) {
	r.destinationCalls.Add(1)
	if r.destination != nil {
		return r.destination(ctx, stack, now)
	}
	return cacheDestinations(stack, now, time.Minute)
}

func (r *cacheTestResolver) ResolvePolicy(ctx context.Context, stack Stack, now time.Time) (Policy, error) {
	r.policyCalls.Add(1)
	if r.policy != nil {
		return r.policy(ctx, stack, now)
	}
	return cachePolicy(stack, now, time.Minute, PreferenceAbsent)
}

var (
	_ DestinationResolver = (*cacheTestResolver)(nil)
	_ PolicyResolver      = (*cacheTestResolver)(nil)
)

func cacheDestinations(stack Stack, now time.Time, lifetime time.Duration) (Destinations, error) {
	in := destinationInput(stack.Namespace())
	in.ValidUntil = now.Add(lifetime)
	return NewDestinations(stack, in, now)
}

func cachePolicy(stack Stack, now time.Time, lifetime time.Duration, preference PreferenceState) (Policy, error) {
	in := policyInput(stack.Namespace())
	in.ValidUntil, in.Preference = now.Add(lifetime), preference
	return NewPolicy(stack, in, now)
}

func cacheTestOptions() CacheOptions {
	return CacheOptions{MaxEntries: 2, LookupTimeout: time.Second, InferenceHeadroom: time.Second,
		RefreshInterval: 10 * time.Second, RetryInterval: 2 * time.Second, IdleRetention: time.Minute, MaintenanceInterval: time.Second}
}

func newTestCache(t *testing.T, resolver *cacheTestResolver, options CacheOptions) *Cache {
	t.Helper()
	c, err := NewCache(resolver, resolver, options)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		assert.NoError(t, c.Close(ctx))
	})
	return c
}

func assertCacheRecording(t *testing.T, stack Stack, result CacheResult, allowed bool) {
	t.Helper()
	d, ok := RecordingDestination(stack, result.Destinations, result.Policy, SignalMetrics, time.Now())
	require.Equal(t, allowed, ok)
	if allowed {
		assert.Equal(t, stack.Namespace()+"-metrics", d.IngestionIdentity)
	}
}

func TestCache_Admission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*CacheOptions)
	}{
		{name: "capacity", mutate: func(o *CacheOptions) { o.MaxEntries = 0 }},
		{name: "timeout", mutate: func(o *CacheOptions) { o.LookupTimeout = 0 }},
		{name: "headroom", mutate: func(o *CacheOptions) { o.InferenceHeadroom = -1 }},
		{name: "refresh", mutate: func(o *CacheOptions) { o.RefreshInterval = 0 }},
		{name: "retry", mutate: func(o *CacheOptions) { o.RetryInterval = 0 }},
		{name: "retry exceeds refresh", mutate: func(o *CacheOptions) { o.RetryInterval = time.Hour }},
		{name: "retention", mutate: func(o *CacheOptions) { o.IdleRetention = 0 }},
		{name: "cadence", mutate: func(o *CacheOptions) { o.MaintenanceInterval = 0 }},
		{name: "cadence exceeds retry", mutate: func(o *CacheOptions) { o.MaintenanceInterval = time.Hour }},
		{name: "cadence exceeds retention", mutate: func(o *CacheOptions) { o.IdleRetention = time.Nanosecond }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := cacheTestOptions()
			tc.mutate(&o)
			r := &cacheTestResolver{}
			c, err := NewCache(r, r, o)
			assert.ErrorIs(t, err, ErrCacheOptions)
			assert.Nil(t, c)
		})
	}
	t.Run("nil resolvers", func(t *testing.T) {
		r := &cacheTestResolver{}
		_, err := NewCache(nil, r, cacheTestOptions())
		assert.ErrorIs(t, err, ErrCacheOptions)
		_, err = NewCache(r, nil, cacheTestOptions())
		assert.ErrorIs(t, err, ErrCacheOptions)
	})
	t.Run("no unauthorized or startup work", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := &cacheTestResolver{}
			c := newTestCache(t, r, cacheTestOptions())
			synctest.Wait()
			assert.Zero(t, r.destinationCalls.Load())
			assert.Zero(t, r.policyCalls.Load())
			assert.Equal(t, LookupInvalid, c.Lookup(context.Background(), Stack{}).DestinationOutcome)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			assert.Equal(t, LookupCanceled, c.Lookup(ctx, testStack(t, "A")).PolicyOutcome)
			assert.Zero(t, r.destinationCalls.Load())
			assert.Zero(t, r.policyCalls.Load())
		})
	})
}

func TestCache_ColdWarmAndIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &cacheTestResolver{}
		c := newTestCache(t, r, cacheTestOptions())
		a, b := testStack(t, "A"), testStack(t, "B")
		for _, stack := range []Stack{a, a, b, b, a} {
			result := c.Lookup(context.Background(), stack)
			assert.Equal(t, LookupReady, result.DestinationOutcome)
			assert.Equal(t, LookupReady, result.PolicyOutcome)
			assertCacheRecording(t, stack, result, true)
		}
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		assert.EqualValues(t, 2, r.policyCalls.Load())
	})
}

func TestCache_DeduplicationAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		r := &cacheTestResolver{}
		r.destination = func(ctx context.Context, s Stack, now time.Time) (Destinations, error) {
			assert.Nil(t, ctx.Value(cacheRequestKey{}))
			if s.Namespace() == "A" {
				select {
				case <-release:
				case <-ctx.Done():
					return Destinations{}, ctx.Err()
				}
			}
			return cacheDestinations(s, now, time.Minute)
		}
		c := newTestCache(t, r, cacheTestOptions())
		a, b := testStack(t, "A"), testStack(t, "B")
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), cacheRequestKey{}, "private"))
		first, second := make(chan CacheResult, 1), make(chan CacheResult, 1)
		go func() { first <- c.Lookup(ctx, a) }()
		synctest.Wait()
		go func() { second <- c.Lookup(context.Background(), a) }()
		synctest.Wait()
		assert.EqualValues(t, 1, r.destinationCalls.Load())
		assert.EqualValues(t, 1, r.policyCalls.Load())
		cancel()
		assert.Equal(t, LookupCanceled, (<-first).DestinationOutcome)
		assertCacheRecording(t, b, c.Lookup(context.Background(), b), true)
		close(release)
		assertCacheRecording(t, a, <-second, true)
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		assert.EqualValues(t, 2, r.policyCalls.Load())
	})
}

func TestCache_RequestBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &cacheTestResolver{}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		deadline, _ := ctx.Deadline()
		assert.Equal(t, LookupBudget, c.Lookup(ctx, s).DestinationOutcome)
		assert.Zero(t, r.destinationCalls.Load())
		assert.Zero(t, r.policyCalls.Load())
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		assertCacheRecording(t, s, c.Lookup(ctx, s), true)
		actual, _ := ctx.Deadline()
		assert.Equal(t, deadline, actual)
	})
}

func TestCache_ResultValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		foreign bool
		expire  bool
		err     error
		want    LookupOutcome
	}{
		{name: "foreign", foreign: true, want: LookupInvalid},
		{name: "expires during lookup", expire: true, want: LookupInvalid},
		{name: "denied", err: fmt.Errorf("private endpoint: %w", ErrResolutionDenied), want: LookupDenied},
		{name: "invalid", err: ErrDestination, want: LookupInvalid},
		{name: "policy unavailable", err: ErrPolicyUnavailable, want: LookupFailed},
		{name: "unknown failure", err: errors.New("secret raw error"), want: LookupFailed},
		{name: "upstream deadline", err: context.DeadlineExceeded, want: LookupTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := &cacheTestResolver{}
				other := testStack(t, "B")
				r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
					if tc.err != nil {
						return Destinations{}, tc.err
					}
					if tc.foreign {
						s = other
					}
					life := time.Minute
					if tc.expire {
						life = time.Millisecond
					}
					d, err := cacheDestinations(s, now, life)
					if tc.expire {
						time.Sleep(2 * time.Millisecond)
					}
					return d, err
				}
				c := newTestCache(t, r, cacheTestOptions())
				s := testStack(t, "A")
				result := c.Lookup(context.Background(), s)
				assert.Equal(t, tc.want, result.DestinationOutcome)
				assertCacheRecording(t, s, result, false)
				assert.Equal(t, LookupReady, result.PolicyOutcome)
				for range 5 {
					c.Lookup(context.Background(), s)
				}
				assert.EqualValues(t, 1, r.destinationCalls.Load())
			})
		})
	}
}

func TestCache_TimeoutAndLateResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		r := &cacheTestResolver{}
		r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			<-release
			return cacheDestinations(s, now, time.Minute)
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		start := time.Now()
		result := c.Lookup(context.Background(), s)
		assert.Equal(t, time.Second, time.Since(start))
		assert.Equal(t, LookupTimeout, result.DestinationOutcome)
		assertCacheRecording(t, s, result, false)
		close(release)
		synctest.Wait()
		assert.Equal(t, LookupTimeout, c.Lookup(context.Background(), s).DestinationOutcome)
		assert.EqualValues(t, 1, r.destinationCalls.Load())
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		assert.EqualValues(t, 2, r.destinationCalls.Load())
	})
}
