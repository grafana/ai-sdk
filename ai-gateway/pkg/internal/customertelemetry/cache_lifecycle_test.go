package customertelemetry

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_CapacityAndIdleCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := cacheTestOptions()
		o.MaxEntries, o.IdleRetention = 1, 12*time.Second
		r := &cacheTestResolver{}
		c := newTestCache(t, r, o)
		a, b := testStack(t, "A"), testStack(t, "B")
		assertCacheRecording(t, a, c.Lookup(context.Background(), a), true)
		assert.Equal(t, LookupCapacity, c.Lookup(context.Background(), b).DestinationOutcome)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		time.Sleep(2 * time.Second)
		synctest.Wait()
		c.mu.Lock()
		assert.Empty(t, c.entries)
		c.mu.Unlock()
		assertCacheRecording(t, b, c.Lookup(context.Background(), b), true)
		assert.EqualValues(t, 3, r.destinationCalls.Load())
	})
}

func TestCache_NonCooperativeResolverRemainsCharged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		r := &cacheTestResolver{}
		r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			<-release
			return cacheDestinations(s, now, time.Minute)
		}
		o := cacheTestOptions()
		o.MaxEntries, o.IdleRetention = 1, 3*time.Second
		c := newTestCache(t, r, o)
		t.Cleanup(func() { close(release) })
		a, b := testStack(t, "A"), testStack(t, "B")
		assert.Equal(t, LookupTimeout, c.Lookup(context.Background(), a).DestinationOutcome)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.Equal(t, LookupCapacity, c.Lookup(context.Background(), b).DestinationOutcome)
		assert.EqualValues(t, 1, r.destinationCalls.Load())
		assert.EqualValues(t, 1, r.policyCalls.Load())
		assert.Equal(t, LookupTimeout, c.Lookup(context.Background(), a).DestinationOutcome)
		assert.EqualValues(t, 1, r.destinationCalls.Load())
	})
}

func TestCache_Close(t *testing.T) {
	for _, cooperative := range []bool{true, false} {
		name := "cooperative"
		if !cooperative {
			name = "non-cooperative"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				r := &cacheTestResolver{}
				r.destination = func(ctx context.Context, s Stack, now time.Time) (Destinations, error) {
					if cooperative {
						<-ctx.Done()
						return Destinations{}, ctx.Err()
					}
					<-release
					return cacheDestinations(s, now, time.Minute)
				}
				c := newTestCache(t, r, cacheTestOptions())
				s := testStack(t, "A")
				result := make(chan CacheResult, 1)
				go func() { result <- c.Lookup(context.Background(), s) }()
				synctest.Wait()
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				err := c.Close(ctx)
				if cooperative {
					assert.NoError(t, err)
				} else {
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				}
				assert.Equal(t, LookupClosed, (<-result).DestinationOutcome)
				assert.Equal(t, LookupClosed, c.Lookup(context.Background(), s).DestinationOutcome)
				close(release)
				require.NoError(t, c.Close(context.Background()))
				require.NoError(t, c.Close(context.Background()))
				c.mu.Lock()
				assert.Zero(t, c.entries[s].slots[destinationComponent].value)
				c.mu.Unlock()
				time.Sleep(time.Minute)
				synctest.Wait()
				assert.EqualValues(t, 1, r.destinationCalls.Load())
				assert.EqualValues(t, 1, r.policyCalls.Load())
			})
		})
	}
}

func TestCache_IdleCleanupPreservesRetrySpacing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
	}{
		{"transient", errors.New("unavailable")},
		{"denied", ErrResolutionDenied},
		{"invalid", ErrSnapshot},
		{"timeout", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, warm := range []bool{false, true} {
				name := "cold"
				if warm {
					name = "refresh"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						r := &cacheTestResolver{}
						r.destination = func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
							if warm && r.destinationCalls.Load() == 1 {
								return cacheDestinations(s, now, time.Minute)
							}
							return Destinations{}, tc.failure
						}
						r.policy = func(_ context.Context, s Stack, now time.Time) (Policy, error) {
							if warm && r.policyCalls.Load() == 1 {
								return cachePolicy(s, now, time.Minute, PreferenceAbsent)
							}
							return Policy{}, tc.failure
						}
						o := cacheTestOptions()
						o.MaxEntries, o.IdleRetention, o.RetryInterval = 2, time.Second, 5*time.Second
						o.MaintenanceInterval = 100 * time.Millisecond
						if warm {
							o.IdleRetention = 12 * time.Second
						}
						c := newTestCache(t, r, o)
						a, b, other := testStack(t, "A"), testStack(t, "B"), testStack(t, "C")
						c.Lookup(context.Background(), a)
						expected := int64(1)
						if warm {
							time.Sleep(10 * time.Second)
							synctest.Wait()
							expected = 2
						}
						require.Equal(t, expected, r.destinationCalls.Load())
						time.Sleep(2100 * time.Millisecond)
						synctest.Wait()
						c.Lookup(context.Background(), a)
						synctest.Wait()
						require.Equal(t, expected, r.destinationCalls.Load())
						require.Equal(t, expected, r.policyCalls.Load())
						c.Lookup(context.Background(), b)
						require.Equal(t, expected+1, r.destinationCalls.Load())
						require.Equal(t, LookupCapacity, c.Lookup(context.Background(), other).DestinationOutcome)
						time.Sleep(3 * time.Second)
						synctest.Wait()
						c.Lookup(context.Background(), a)
						synctest.Wait()
						require.Equal(t, expected+2, r.destinationCalls.Load())
						require.Equal(t, expected+2, r.policyCalls.Load())
						time.Sleep(20 * time.Second)
						synctest.Wait()
						c.mu.Lock()
						assert.Empty(t, c.entries)
						c.mu.Unlock()
						require.NotEqual(t, LookupCapacity, c.Lookup(context.Background(), other).DestinationOutcome)
					})
				})
			}
		})
	}
}
