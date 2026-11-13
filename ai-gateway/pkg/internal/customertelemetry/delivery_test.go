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

var (
	_ deliverySink   = (*Delivery[*poolTestClient])(nil)
	_ cachePublisher = (*Controller)(nil)
)

func newTestDelivery(t *testing.T, r *cacheTestResolver, co CacheOptions, po PoolOptions) (*Controller, *Delivery[*poolTestClient]) {
	t.Helper()
	p, err := NewPool(func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
		return poolTestResource(target, gate), nil
	}, po)
	require.NoError(t, err)
	d, err := NewDelivery(SignalMetrics, p)
	require.NoError(t, err)
	c, err := NewController(r, r, co, d)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, c.Close(context.Background())) })
	return c, d
}

func acquireDelivery(t *testing.T, d *Delivery[*poolTestClient], s Stack) (*Lease[*poolTestClient], Policy) {
	t.Helper()
	l, policy, err := d.Acquire(context.Background(), s)
	require.NoError(t, err)
	t.Cleanup(l.Release)
	return l, policy
}

func TestController_Admission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &cacheTestResolver{}
		factory := func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
			return poolTestResource(target, gate), nil
		}
		p := newTestPool(t, factory, poolTestOptions())
		d, err := NewDelivery(SignalMetrics, p)
		require.NoError(t, err)
		_, _, err = d.Acquire(context.Background(), testStack(t, "A"))
		assert.ErrorIs(t, err, ErrControllerOptions)
		_, err = NewDelivery[*poolTestClient](SignalMetrics, nil)
		assert.ErrorIs(t, err, ErrControllerOptions)
		_, err = NewDelivery(Signal("unknown"), p)
		assert.ErrorIs(t, err, ErrControllerOptions)
		var nilDelivery *Delivery[int]
		for _, deliveries := range [][]deliverySink{nil, {nil}, {nilDelivery}, {d, d}} {
			_, err = NewController(r, r, cacheTestOptions(), deliveries...)
			assert.ErrorIs(t, err, ErrControllerOptions)
		}
		_, err = NewController(nil, r, cacheTestOptions(), d)
		assert.ErrorIs(t, err, ErrCacheOptions)
		otherSignal, err := NewDelivery(SignalTraces, p)
		require.NoError(t, err)
		_, err = NewController(r, r, cacheTestOptions(), d, otherSignal)
		assert.ErrorIs(t, err, ErrControllerOptions)
		assert.Nil(t, d.owner.Load())
		c, err := NewController(r, r, cacheTestOptions(), d)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, c.Close(context.Background())) })
		_, err = NewController(r, r, cacheTestOptions(), d)
		assert.ErrorIs(t, err, ErrControllerOptions)
		synctest.Wait()
		assert.Zero(t, r.destinationCalls.Load())
		assert.Zero(t, r.policyCalls.Load())
		_, _, err = d.Acquire(context.Background(), Stack{})
		assert.ErrorIs(t, err, ErrBinding)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err = d.Acquire(ctx, testStack(t, "A"))
		assert.ErrorIs(t, err, context.Canceled)
		ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		_, _, err = d.Acquire(ctx, testStack(t, "A"))
		assert.ErrorIs(t, err, ErrBinding)
		assert.Zero(t, r.destinationCalls.Load())
		lease, policy := acquireDelivery(t, d, testStack(t, "A"))
		assert.Equal(t, CaptureMetadataOnly, policy.Capture(testStack(t, "A"), SignalGenerations, time.Now()))
		assert.EqualValues(t, 1, r.destinationCalls.Load())
		assert.EqualValues(t, 1, r.policyCalls.Load())
		_, err = p.Update(testStack(t, "A"), SignalMetrics, Destinations{}, Policy{})
		assert.ErrorIs(t, err, ErrBinding)
		assert.True(t, lease.Value().gate.valid())
		lease.Release()
		require.NoError(t, c.Close(context.Background()))
		_, _, err = d.Acquire(context.Background(), testStack(t, "A"))
		assert.ErrorIs(t, err, ErrPoolClosed)
	})
}

func TestDelivery_ConcurrentAcquisition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolve, build := make(chan struct{}), make(chan struct{})
		a, b := testStack(t, "A"), testStack(t, "B")
		r := &cacheTestResolver{destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
			if s == a {
				<-resolve
			}
			return cacheDestinations(s, now, time.Minute)
		}}
		var builds atomic.Int64
		p := newTestPool(t, func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
			builds.Add(1)
			if target.Stack == a {
				<-build
			}
			return poolTestResource(target, gate), nil
		}, poolTestOptions())
		d, err := NewDelivery(SignalMetrics, p)
		require.NoError(t, err)
		c, err := NewController(r, r, cacheTestOptions(), d)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, c.Close(context.Background())) })
		ctx, cancel := context.WithCancel(context.Background())
		canceled := make(chan error, 1)
		go func() { _, _, err := d.Acquire(ctx, a); canceled <- err }()
		results := make(chan *Lease[*poolTestClient], 8)
		for range 8 {
			go func() {
				l, _, err := d.Acquire(context.Background(), a)
				assert.NoError(t, err)
				results <- l
			}()
		}
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-canceled, context.Canceled)
		other, _ := acquireDelivery(t, d, b)
		assert.Equal(t, "B-metrics", other.Value().target.Destination.IngestionIdentity)
		close(resolve)
		synctest.Wait()
		another, _ := acquireDelivery(t, d, b)
		assert.Same(t, other.Value(), another.Value())
		assert.EqualValues(t, 2, builds.Load())
		close(build)
		var first *poolTestClient
		for range 8 {
			l := <-results
			require.NotNil(t, l)
			if first == nil {
				first = l.Value()
			}
			assert.Same(t, first, l.Value())
			l.Release()
		}
		assert.EqualValues(t, 2, r.destinationCalls.Load())
		assert.EqualValues(t, 2, r.policyCalls.Load())
	})
}

func TestController_RejectsUsedAndClosedPools(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "used"
		if closed {
			name = "closed"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
					return poolTestResource(target, g), nil
				}, poolTestOptions())
				if closed {
					require.NoError(t, p.Close(context.Background()))
				} else {
					poolTestBinding(t, p, testStack(t, "A"), "v1")
				}
				d, err := NewDelivery(SignalMetrics, p)
				require.NoError(t, err)
				r := &cacheTestResolver{}
				_, err = NewController(r, r, cacheTestOptions(), d)
				assert.ErrorIs(t, err, ErrControllerOptions)
				assert.Nil(t, d.owner.Load())
				assert.Zero(t, r.destinationCalls.Load())
			})
		})
	}
}

func TestDelivery_CurrentStateAndOpaqueRevisions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var phase atomic.Int64
		r := &cacheTestResolver{policy: func(_ context.Context, stack Stack, now time.Time) (Policy, error) {
			in := policyInput(stack.Namespace())
			in.ValidUntil = now.Add(time.Minute)
			in.Revision = "z"
			if phase.Load() > 0 {
				in.Revision = "a"
			}
			if phase.Load() == 2 {
				in.Preference = PreferenceDisabled
			}
			return NewPolicy(stack, in, now)
		}}
		c, d := newTestDelivery(t, r, cacheTestOptions(), poolTestOptions())
		a := testStack(t, "A")
		old, before := acquireDelivery(t, d, a)
		phase.Store(1)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		current, after := acquireDelivery(t, d, a)
		assert.Equal(t, "z", before.Revision())
		assert.Equal(t, "a", after.Revision())
		assert.Same(t, old.Value(), current.Value())
		snapshot := c.cache.Lookup(context.Background(), a)
		phase.Store(2)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		_, err := d.pool.Update(a, SignalMetrics, snapshot.Destinations, snapshot.Policy)
		assert.ErrorIs(t, err, ErrBinding)
		_, _, err = d.Acquire(context.Background(), a)
		assert.ErrorIs(t, err, ErrBinding)
		_, err = old.binding.Acquire(context.Background())
		assert.ErrorIs(t, err, ErrBinding)
		assert.False(t, old.Value().gate.valid())
		phase.Store(1)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		reenabled, _ := acquireDelivery(t, d, a)
		assert.NotSame(t, old.Value(), reenabled.Value())
		assert.False(t, old.Value().gate.valid())
	})
}

func TestDelivery_IndependentSignals(t *testing.T) {
	for _, fault := range []string{"policy", "removal", "destination denial", "policy denial", "invalid", "transient"} {
		t.Run(fault, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var changed atomic.Bool
				a, b := testStack(t, "A"), testStack(t, "B")
				r := &cacheTestResolver{
					destination: func(_ context.Context, s Stack, now time.Time) (Destinations, error) {
						if s == a && changed.Load() && fault == "destination denial" {
							return Destinations{}, ErrResolutionDenied
						}
						in := destinationInput(s.Namespace())
						in.ValidUntil = now.Add(time.Minute)
						if s == a && changed.Load() && fault == "removal" {
							delete(in.Signals, SignalGenerations)
						}
						return NewDestinations(s, in, now)
					},
					policy: func(_ context.Context, s Stack, now time.Time) (Policy, error) {
						in := policyInput(s.Namespace())
						in.ValidUntil = now.Add(time.Minute)
						if s == a && changed.Load() {
							switch fault {
							case "policy":
								in.Eligibility[SignalGenerations] = EligibilityIneligible
							case "policy denial":
								return Policy{}, ErrResolutionDenied
							case "invalid":
								return Policy{}, nil
							case "transient":
								return Policy{}, errors.New("private diagnostic")
							}
						}
						return NewPolicy(s, in, now)
					},
				}
				pm := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
					return poolTestResource(target, g), nil
				}, poolTestOptions())
				pg, err := NewPool(func(_ context.Context, _ ExportTarget, g *ExportGate) (Resource[*ExportGate], error) {
					return Resource[*ExportGate]{Value: g, Shutdown: func(context.Context) error { return nil }}, nil
				}, poolTestOptions())
				require.NoError(t, err)
				dm, err := NewDelivery(SignalMetrics, pm)
				require.NoError(t, err)
				dg, err := NewDelivery(SignalGenerations, pg)
				require.NoError(t, err)
				c, err := NewController(r, r, cacheTestOptions(), dm, dg)
				require.NoError(t, err)
				t.Cleanup(func() { assert.NoError(t, c.Close(context.Background())) })
				metrics, _ := acquireDelivery(t, dm, a)
				generation, _, err := dg.Acquire(context.Background(), a)
				require.NoError(t, err)
				t.Cleanup(generation.Release)
				other, _ := acquireDelivery(t, dm, b)
				changed.Store(true)
				time.Sleep(10 * time.Second)
				synctest.Wait()
				assert.Equal(t, fault == "transient", generation.Value().valid())
				assert.Equal(t, fault == "policy" || fault == "removal" || fault == "transient", metrics.Value().gate.valid())
				assert.True(t, other.Value().gate.valid())
			})
		})
	}
}
