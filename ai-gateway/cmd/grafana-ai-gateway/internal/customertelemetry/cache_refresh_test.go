package customertelemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_IndependentRefreshAndExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fail atomic.Bool
		r := &cacheTestResolver{}
		r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			if fail.Load() {
				return Destinations{}, errors.New("unavailable")
			}
			return cacheDestinations(s, now, 8*time.Second)
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		fail.Store(true)
		time.Sleep(4 * time.Second)
		synctest.Wait()
		result := c.Lookup(context.Background(), s)
		assert.Equal(t, LookupFailed, result.DestinationOutcome)
		assertCacheRecording(t, s, result, true)
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		assert.EqualValues(t, 1, r.policyCalls.Load())
		time.Sleep(4 * time.Second)
		synctest.Wait()
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), false)
		assert.EqualValues(t, 4, r.destinationCalls.Load())
		assert.EqualValues(t, 1, r.policyCalls.Load())
		fail.Store(false)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		assert.EqualValues(t, 5, r.destinationCalls.Load())
		assert.EqualValues(t, 2, r.policyCalls.Load())
	})
}

func TestCache_PolicyDisableFailureAndRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var phase atomic.Int32
		r := &cacheTestResolver{}
		r.policy = func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			switch phase.Load() {
			case 1:
				return cachePolicy(s, now, time.Minute, PreferenceDisabled)
			case 2:
				return Policy{}, ErrPolicyUnavailable
			default:
				return cachePolicy(s, now, time.Minute, PreferenceAbsent)
			}
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		phase.Store(1)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		result := c.Lookup(context.Background(), s)
		assert.Equal(t, LookupNotReady, result.PolicyOutcome)
		assertCacheRecording(t, s, result, false)
		phase.Store(2)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		result = c.Lookup(context.Background(), s)
		assert.Equal(t, LookupFailed, result.PolicyOutcome)
		assert.Equal(t, time.Now().Add(58*time.Second), result.Policy.ValidUntil())
		assertCacheRecording(t, s, result, false)
		for range 20 {
			assertCacheRecording(t, s, c.Lookup(context.Background(), s), false)
		}
		assert.EqualValues(t, 3, r.policyCalls.Load())
		phase.Store(3)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		assert.EqualValues(t, 4, r.policyCalls.Load())
		assert.EqualValues(t, 2, r.destinationCalls.Load())
	})
}

func TestCache_DestinationRemoval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var phase atomic.Int32
		r := &cacheTestResolver{}
		r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			in := destinationInput(s.Namespace())
			in.ValidUntil = now.Add(time.Minute)
			switch phase.Load() {
			case 1:
				delete(in.Signals, SignalMetrics)
			case 2:
				in.Signals = nil
			}
			return NewDestinations(s, in, now)
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		phase.Store(1)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		result := c.Lookup(context.Background(), s)
		assertCacheRecording(t, s, result, false)
		_, ok := RecordingDestination(s, result.Destinations, result.Policy, SignalTraces, time.Now())
		assert.True(t, ok)
		phase.Store(2)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		result = c.Lookup(context.Background(), s)
		assert.Equal(t, LookupNotReady, result.DestinationOutcome)
		assert.Empty(t, result.Destinations.signals)
		phase.Store(3)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
	})
}

func TestCache_Revocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want LookupOutcome
	}{
		{name: "denied", err: ErrResolutionDenied, want: LookupDenied},
		{name: "invalid", err: ErrSnapshot, want: LookupInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []cacheComponent{destinationComponent, policyComponent} {
				name := "destination"
				if kind == policyComponent {
					name = "policy"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						var revoke atomic.Bool
						r := &cacheTestResolver{}
						r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
							if revoke.Load() && kind == destinationComponent {
								return Destinations{}, tc.err
							}
							return cacheDestinations(s, now, time.Minute)
						}
						r.policy = func(_ context.Context, s Stack, now time.Time) (Policy, error) {
							if revoke.Load() && kind == policyComponent {
								return Policy{}, tc.err
							}
							return cachePolicy(s, now, time.Minute, PreferenceAbsent)
						}
						c := newTestCache(t, r, cacheTestOptions())
						s := testStack(t, "A")
						assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
						revoke.Store(true)
						time.Sleep(10 * time.Second)
						synctest.Wait()
						result := c.Lookup(context.Background(), s)
						assertCacheRecording(t, s, result, false)
						if kind == destinationComponent {
							assert.Equal(t, tc.want, result.DestinationOutcome)
							assert.Zero(t, result.Destinations)
						} else {
							assert.Equal(t, tc.want, result.PolicyOutcome)
							assert.Zero(t, result.Policy)
						}
					})
				})
			}
		})
	}
}

func TestCache_ShortLivedResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &cacheTestResolver{}
		r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			return cacheDestinations(s, now, time.Millisecond)
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		time.Sleep(time.Millisecond)
		synctest.Wait()
		result := c.Lookup(context.Background(), s)
		assert.Equal(t, LookupExpired, result.DestinationOutcome)
		assertCacheRecording(t, s, result, false)
		assert.EqualValues(t, 1, r.destinationCalls.Load())
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.EqualValues(t, 6, r.destinationCalls.Load())
	})
}

func TestCache_WarmReadDuringRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		r := &cacheTestResolver{}
		r.destination = func(ctx context.Context, s Stack, now time.Time) (Destinations, error) {
			if r.destinationCalls.Load() > 1 {
				select {
				case <-release:
				case <-ctx.Done():
					return Destinations{}, ctx.Err()
				}
			}
			return cacheDestinations(s, now, time.Minute)
		}
		c := newTestCache(t, r, cacheTestOptions())
		s := testStack(t, "A")
		assertCacheRecording(t, s, c.Lookup(context.Background(), s), true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		start := time.Now()
		assertCacheRecording(t, s, c.Lookup(ctx, s), true)
		assert.Zero(t, time.Since(start))
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		close(release)
		synctest.Wait()
	})
}

func TestCache_PolicyResultValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		foreign bool
		expired bool
	}{
		{name: "zero"}, {name: "foreign", foreign: true}, {name: "expired", expired: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := &cacheTestResolver{}
				other := testStack(t, "B")
				r.policy = func(_ context.Context, s Stack, now time.Time) (Policy, error) {
					if tc.foreign {
						return cachePolicy(other, now, time.Minute, PreferenceAbsent)
					}
					if tc.expired {
						p, err := cachePolicy(s, now, time.Millisecond, PreferenceAbsent)
						time.Sleep(2 * time.Millisecond)
						return p, err
					}
					return Policy{}, nil
				}
				c := newTestCache(t, r, cacheTestOptions())
				s := testStack(t, "A")
				result := c.Lookup(context.Background(), s)
				require.Equal(t, LookupInvalid, result.PolicyOutcome)
				assertCacheRecording(t, s, result, false)
			})
		})
	}
}
