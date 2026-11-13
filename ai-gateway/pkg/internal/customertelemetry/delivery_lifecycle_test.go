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

func TestDelivery_ExpiryAndLateCompletion(t *testing.T) {
	for _, component := range []cacheComponent{destinationComponent, policyComponent} {
		t.Run(map[cacheComponent]string{destinationComponent: "destination", policyComponent: "policy"}[component], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				blocked := make(chan struct{})
				var destCalls, policyCalls atomic.Int64
				deadline := time.Now().Add(6 * time.Second)
				r := &cacheTestResolver{
					destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
						if component == destinationComponent && destCalls.Add(1) > 1 {
							<-blocked
						}
						d, err := cacheDestinations(s, time.Now(), time.Minute)
						if component == destinationComponent && destCalls.Load() == 1 {
							d.validUntil = deadline
						}
						return d, err
					},
					policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
						if component == policyComponent && policyCalls.Add(1) > 1 {
							<-blocked
						}
						p, err := cachePolicy(s, time.Now(), time.Minute, PreferenceAbsent)
						if component == policyComponent && policyCalls.Load() == 1 {
							p.validUntil = deadline
						}
						return p, err
					},
				}
				co := cacheTestOptions()
				co.IdleRetention = 5 * time.Second
				c, d := newTestDelivery(t, r, co, poolTestOptions())
				s := testStack(t, "A")
				lease, _ := acquireDelivery(t, d, s)
				operation, stop, err := lease.Value().gate.operation(context.Background())
				require.NoError(t, err)
				defer stop()
				time.Sleep(6 * time.Second)
				synctest.Wait()
				assert.False(t, lease.Value().gate.valid())
				assert.Error(t, operation.Err())
				c.cache.mu.Lock()
				assert.Len(t, c.cache.entries, 1)
				d.pool.mu.Lock()
				assert.NotContains(t, d.pool.current, poolKey{s, SignalMetrics})
				d.pool.mu.Unlock()
				c.cache.mu.Unlock()
				close(blocked)
				synctest.Wait()
				assert.False(t, lease.Value().gate.valid())
				c.cache.mu.Lock()
				assert.Empty(t, c.cache.entries)
				c.cache.mu.Unlock()
			})
		})
	}
}

func TestDelivery_DelayedDestinationCannotUndoDisablement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		var changed atomic.Bool
		r := &cacheTestResolver{
			destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
				if changed.Load() {
					<-blocked
				}
				return cacheDestinations(s, time.Now(), time.Minute)
			},
			policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
				preference := PreferenceAbsent
				if changed.Load() {
					preference = PreferenceDisabled
				}
				return cachePolicy(s, now, time.Minute, preference)
			},
		}
		_, d := newTestDelivery(t, r, cacheTestOptions(), poolTestOptions())
		s := testStack(t, "A")
		l, _ := acquireDelivery(t, d, s)
		changed.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.False(t, l.Value().gate.valid())
		close(blocked)
		synctest.Wait()
		_, _, err := d.Acquire(context.Background(), s)
		assert.ErrorIs(t, err, ErrBinding)
		assert.False(t, l.Value().gate.valid())
	})
}

func TestDelivery_CapacityAndPoolIdleRecovery(t *testing.T) {
	for _, rotate := range []bool{true, false} {
		name := "pool idle"
		if rotate {
			name = "rotation at capacity"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var version atomic.Bool
				r := &cacheTestResolver{destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
					in := destinationInput(s.Namespace())
					in.ValidUntil = now.Add(time.Minute)
					if version.Load() {
						for signal, target := range in.Signals {
							target.Credential.Version = "new"
							in.Signals[signal] = target
						}
					}
					return NewDestinations(s, in, now)
				}}
				po := poolTestOptions()
				po.MaxResources = 1
				po.IdleRetention = time.Second
				_, d := newTestDelivery(t, r, cacheTestOptions(), po)
				s := testStack(t, "A")
				old, _ := acquireDelivery(t, d, s)
				if rotate {
					version.Store(true)
					time.Sleep(10 * time.Second)
					synctest.Wait()
					_, _, err := d.Acquire(context.Background(), s)
					require.ErrorIs(t, err, ErrPoolCapacity)
					assert.True(t, old.Value().gate.valid())
				}
				old.Release()
				time.Sleep(time.Second)
				synctest.Wait()
				calls := r.destinationCalls.Load()
				newer, _ := acquireDelivery(t, d, s)
				assert.NotSame(t, old.Value(), newer.Value())
				assert.Equal(t, calls, r.destinationCalls.Load())
				assert.EqualValues(t, 1, old.Value().closed.Load())
			})
		})
	}
}

func TestDelivery_CacheEvictionRevokesAndReadmits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		co := cacheTestOptions()
		co.MaxEntries = 1
		co.RefreshInterval, co.RetryInterval = 2*time.Second, time.Second
		co.IdleRetention = 5 * time.Second
		c, d := newTestDelivery(t, &cacheTestResolver{}, co, poolTestOptions())
		a, b := testStack(t, "A"), testStack(t, "B")
		old, _ := acquireDelivery(t, d, a)
		_, _, err := d.Acquire(context.Background(), b)
		require.ErrorIs(t, err, ErrBinding)
		lastUsed := time.Now()
		time.Sleep(4 * time.Second)
		synctest.Wait()
		c.cache.mu.Lock()
		assert.Equal(t, lastUsed, c.cache.entries[a].lastUsed)
		c.cache.mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		c.cache.mu.Lock()
		assert.Empty(t, c.cache.entries)
		c.cache.mu.Unlock()
		assert.False(t, old.Value().gate.valid())
		newer, _ := acquireDelivery(t, d, a)
		assert.NotSame(t, old.Value(), newer.Value())
		assert.False(t, old.Value().gate.valid())
	})
}

func TestController_CloseDuringBlockedRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		var block atomic.Bool
		r := &cacheTestResolver{policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			if block.Load() {
				<-blocked
			}
			return cachePolicy(s, time.Now(), time.Minute, PreferenceAbsent)
		}}
		c, d := newTestDelivery(t, r, cacheTestOptions(), poolTestOptions())
		s := testStack(t, "A")
		l, _ := acquireDelivery(t, d, s)
		block.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		closed := make(chan error, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		go func() { closed <- c.Close(ctx) }()
		synctest.Wait()
		d.pool.mu.Lock()
		assert.True(t, d.pool.closed)
		deadline := d.pool.deadline
		d.pool.mu.Unlock()
		assert.True(t, l.Value().gate.valid())
		_, _, err := d.Acquire(context.Background(), s)
		assert.ErrorIs(t, err, ErrPoolClosed)
		require.Error(t, <-closed)
		assert.False(t, l.Value().gate.valid())
		close(blocked)
		l.Release()
		synctest.Wait()
		require.NoError(t, c.Close(context.Background()))
		d.pool.mu.Lock()
		assert.Equal(t, deadline, d.pool.deadline)
		d.pool.mu.Unlock()
		assert.EqualValues(t, 1, l.Value().closed.Load())
	})
}

func TestDelivery_TransientFailurePreservesOriginalDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		r := &cacheTestResolver{policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			if calls.Add(1) > 1 {
				return Policy{}, errors.New("private service diagnostic")
			}
			return cachePolicy(s, now, 6*time.Second, PreferenceAbsent)
		}}
		_, d := newTestDelivery(t, r, cacheTestOptions(), poolTestOptions())
		l, _ := acquireDelivery(t, d, testStack(t, "A"))
		time.Sleep(5 * time.Second)
		synctest.Wait()
		assert.True(t, l.Value().gate.valid())
		time.Sleep(time.Second)
		synctest.Wait()
		assert.False(t, l.Value().gate.valid())
	})
}

func TestDelivery_RevocationCancelsActiveExport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var disabled atomic.Bool
		r := &cacheTestResolver{policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			preference := PreferenceAbsent
			if disabled.Load() {
				preference = PreferenceDisabled
			}
			return cachePolicy(s, now, time.Minute, preference)
		}}
		po := poolTestOptions()
		po.ExportTimeout = time.Minute
		_, d := newTestDelivery(t, r, cacheTestOptions(), po)
		l, _ := acquireDelivery(t, d, testStack(t, "A"))
		result := make(chan error, 1)
		go func() {
			result <- l.Value().gate.Run(context.Background(), func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
		}()
		synctest.Wait()
		disabled.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.ErrorIs(t, <-result, context.Canceled)
	})
}

func TestDelivery_RevocationDuringConstruction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var disabled atomic.Bool
		r := &cacheTestResolver{policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
			preference := PreferenceAbsent
			if disabled.Load() {
				preference = PreferenceDisabled
			}
			return cachePolicy(s, now, time.Minute, preference)
		}}
		blocked := make(chan struct{})
		var client *poolTestClient
		po := poolTestOptions()
		po.BuildTimeout = 20 * time.Second
		p := newTestPool(t, func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
			resource := poolTestResource(target, gate)
			client = resource.Value
			<-blocked
			return resource, nil
		}, po)
		d, err := NewDelivery(SignalMetrics, p)
		require.NoError(t, err)
		c, err := NewController(r, r, cacheTestOptions(), d)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, c.Close(context.Background())) })
		result := make(chan error, 1)
		s := testStack(t, "A")
		go func() { _, _, err := d.Acquire(context.Background(), s); result <- err }()
		synctest.Wait()
		disabled.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.False(t, client.gate.valid())
		close(blocked)
		require.ErrorIs(t, <-result, ErrBinding)
		synctest.Wait()
		assert.EqualValues(t, 1, client.closed.Load())
	})
}
