package customertelemetry

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type poolTestClient struct {
	target ExportTarget
	gate   *ExportGate
	closed atomic.Int64
}

func poolTestResource(target ExportTarget, gate *ExportGate) Resource[*poolTestClient] {
	c := &poolTestClient{target: target, gate: gate}
	return Resource[*poolTestClient]{Value: c, Shutdown: func(context.Context) error { c.closed.Add(1); return nil }}
}

func poolTestOptions() PoolOptions {
	return PoolOptions{MaxResources: 4, BuildTimeout: time.Second, RetryInterval: time.Second, ExportTimeout: time.Second, IdleRetention: 10 * time.Second, MaintenanceInterval: 100 * time.Millisecond, ShutdownTimeout: time.Second}
}

func poolTestSnapshots(t *testing.T, stack Stack, version string) (Destinations, Policy) {
	t.Helper()
	now := time.Now()
	in := destinationInput(stack.Namespace())
	in.ValidUntil = now.Add(time.Minute)
	for signal, d := range in.Signals {
		d.Credential.Version = version
		in.Signals[signal] = d
	}
	d, err := NewDestinations(stack, in, now)
	require.NoError(t, err)
	p, err := cachePolicy(stack, now, time.Minute, PreferenceAbsent)
	require.NoError(t, err)
	return d, p
}

func newTestPool(t *testing.T, factory ResourceFactory[*poolTestClient], options PoolOptions) *Pool[*poolTestClient] {
	t.Helper()
	p, err := NewPool(factory, options)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, p.Close(context.Background())) })
	return p
}

func poolTestBinding(t *testing.T, p *Pool[*poolTestClient], stack Stack, version string) *Binding[*poolTestClient] {
	t.Helper()
	d, policy := poolTestSnapshots(t, stack, version)
	b, err := p.Update(stack, SignalMetrics, d, policy)
	require.NoError(t, err)
	return b
}

func acquireTestLease(t *testing.T, b *Binding[*poolTestClient]) *Lease[*poolTestClient] {
	t.Helper()
	l, err := b.Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(l.Release)
	return l
}

func TestPool_Admission(t *testing.T) {
	factory := func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
		return poolTestResource(target, g), nil
	}
	for _, tc := range []struct {
		name   string
		mutate func(*PoolOptions)
	}{
		{"capacity", func(o *PoolOptions) { o.MaxResources = 0 }},
		{"build", func(o *PoolOptions) { o.BuildTimeout = 0 }},
		{"retry", func(o *PoolOptions) { o.RetryInterval = 0 }},
		{"export", func(o *PoolOptions) { o.ExportTimeout = 0 }},
		{"idle", func(o *PoolOptions) { o.IdleRetention = 0 }},
		{"cadence", func(o *PoolOptions) { o.MaintenanceInterval = 0 }},
		{"slow cadence", func(o *PoolOptions) { o.MaintenanceInterval = time.Hour }},
		{"shutdown", func(o *PoolOptions) { o.ShutdownTimeout = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := poolTestOptions()
			tc.mutate(&o)
			_, err := NewPool(factory, o)
			assert.ErrorIs(t, err, ErrPoolOptions)
		})
	}
	_, err := NewPool[*poolTestClient](nil, poolTestOptions())
	assert.ErrorIs(t, err, ErrPoolOptions)
	t.Run("authority", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var calls atomic.Int64
			p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
				calls.Add(1)
				return poolTestResource(target, g), nil
			}, poolTestOptions())
			a, b := testStack(t, "A"), testStack(t, "B")
			d, policy := poolTestSnapshots(t, a, "v1")
			for _, tc := range []struct {
				stack  Stack
				signal Signal
				d      Destinations
				p      Policy
			}{{Stack{}, SignalMetrics, d, policy}, {b, SignalMetrics, d, policy}, {a, SignalLogs, d, policy}, {a, SignalMetrics, Destinations{}, policy}, {a, SignalMetrics, d, Policy{}}} {
				_, err := p.Update(tc.stack, tc.signal, tc.d, tc.p)
				assert.ErrorIs(t, err, ErrBinding)
			}
			synctest.Wait()
			assert.Zero(t, calls.Load())
		})
	})
}

func TestPool_SharedBuildAndIndependentWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls atomic.Int64
		p := newTestPool(t, func(ctx context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			calls.Add(1)
			assert.Nil(t, ctx.Value(cacheRequestKey{}))
			if target.Stack.Namespace() == "A" {
				select {
				case <-release:
				case <-ctx.Done():
					return Resource[*poolTestClient]{}, ctx.Err()
				}
			}
			return poolTestResource(target, g), nil
		}, poolTestOptions())
		a := poolTestBinding(t, p, testStack(t, "A"), "v1")
		b := poolTestBinding(t, p, testStack(t, "B"), "v1")
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), cacheRequestKey{}, "private"))
		first := make(chan error, 1)
		second := make(chan *Lease[*poolTestClient], 1)
		go func() { _, err := a.Acquire(ctx); first <- err }()
		synctest.Wait()
		go func() { l, err := a.Acquire(context.Background()); assert.NoError(t, err); second <- l }()
		synctest.Wait()
		cancel()
		assert.ErrorIs(t, <-first, context.Canceled)
		other := acquireTestLease(t, b)
		assert.Equal(t, "B", other.Value().target.Stack.Namespace())
		assert.EqualValues(t, 2, calls.Load())
		close(release)
		lease := <-second
		require.NotNil(t, lease)
		defer lease.Release()
		warm := acquireTestLease(t, a)
		assert.Same(t, lease.Value(), warm.Value())
		assert.EqualValues(t, 2, calls.Load())
	})
}

func TestPool_RotationAndPolicyRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			calls.Add(1)
			return poolTestResource(target, g), nil
		}, poolTestOptions())
		s := testStack(t, "A")
		old := poolTestBinding(t, p, s, "v1")
		l := acquireTestLease(t, old)
		time.Sleep(time.Second)
		d, policy := poolTestSnapshots(t, s, "v1")
		policy.revision = "new-policy-revision"
		same, err := p.Update(s, SignalMetrics, d, policy)
		require.NoError(t, err)
		assert.Same(t, old, same)
		assert.Equal(t, time.Now().Add(time.Minute), old.gate.until)
		current := poolTestBinding(t, p, s, "v2")
		next := acquireTestLease(t, current)
		assert.NotSame(t, l.Value(), next.Value())
		assert.Equal(t, "v1", l.Value().target.Destination.Credential.Version)
		assert.Equal(t, "v2", next.Value().target.Destination.Credential.Version)
		_, err = old.Acquire(context.Background())
		assert.ErrorIs(t, err, ErrBinding)
		assert.NoError(t, l.Value().gate.Run(context.Background(), func(context.Context) error { return nil }))
		synctest.Wait()
		assert.Zero(t, l.Value().closed.Load())
		l.Release()
		l.Release()
		synctest.Wait()
		assert.EqualValues(t, 1, l.Value().closed.Load())
		assert.Zero(t, next.Value().closed.Load())
		assert.EqualValues(t, 2, calls.Load())
	})
}

func TestPool_RevokesRetiringAndCurrentOnlyForSignal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			return poolTestResource(target, g), nil
		}, poolTestOptions())
		s := testStack(t, "A")
		old := acquireTestLease(t, poolTestBinding(t, p, s, "v1"))
		current := acquireTestLease(t, poolTestBinding(t, p, s, "v2"))
		d, policy := poolTestSnapshots(t, s, "v2")
		traces, err := p.Update(s, SignalTraces, d, policy)
		require.NoError(t, err)
		traceLease := acquireTestLease(t, traces)
		policy.allowed[SignalMetrics] = false
		_, err = p.Update(s, SignalMetrics, d, policy)
		assert.ErrorIs(t, err, ErrBinding)
		for _, l := range []*Lease[*poolTestClient]{old, current} {
			assert.ErrorIs(t, l.Value().gate.Run(context.Background(), func(context.Context) error { t.Error("revoked export executed"); return nil }), ErrExportDenied)
		}
		assert.NoError(t, traceLease.Value().gate.Run(context.Background(), func(context.Context) error { return nil }))
	})
}

func TestPool_FailureCooldownAndPartialCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		var partial *poolTestClient
		p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			if calls.Add(1) == 1 {
				r := poolTestResource(target, g)
				partial = r.Value
				return r, errors.New("private credential error")
			}
			return poolTestResource(target, g), nil
		}, poolTestOptions())
		b := poolTestBinding(t, p, testStack(t, "A"), "v1")
		for range 10 {
			_, err := b.Acquire(context.Background())
			assert.ErrorIs(t, err, ErrBuild)
			assert.NotContains(t, err.Error(), "private")
		}
		synctest.Wait()
		assert.EqualValues(t, 1, calls.Load())
		assert.EqualValues(t, 1, partial.closed.Load())
		time.Sleep(time.Second)
		synctest.Wait()
		acquireTestLease(t, b)
		assert.EqualValues(t, 2, calls.Load())
	})
}

func TestPool_CapacityIdleAndNonCooperativeBuild(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			o := poolTestOptions()
			o.MaxResources = 1
			p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
				return poolTestResource(target, g), nil
			}, o)
			a, b := testStack(t, "A"), testStack(t, "B")
			first := poolTestBinding(t, p, a, "v1")
			l := acquireTestLease(t, first)
			l.Release()
			d, policy := poolTestSnapshots(t, b, "v1")
			_, err := p.Update(b, SignalMetrics, d, policy)
			assert.ErrorIs(t, err, ErrPoolCapacity)
			time.Sleep(10 * time.Second)
			synctest.Wait()
			assert.EqualValues(t, 1, l.Value().closed.Load())
			poolTestBinding(t, p, b, "v1")
			_, err = first.Acquire(context.Background())
			assert.ErrorIs(t, err, ErrBinding)
		})
	})
	t.Run("pending keeps capacity", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			o := poolTestOptions()
			o.MaxResources = 1
			release := make(chan struct{})
			var calls atomic.Int64
			var client *poolTestClient
			p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
				calls.Add(1)
				<-release
				r := poolTestResource(target, g)
				client = r.Value
				return r, nil
			}, o)
			t.Cleanup(func() { close(release); synctest.Wait() })
			b := poolTestBinding(t, p, testStack(t, "A"), "v1")
			_, err := b.Acquire(context.Background())
			assert.ErrorIs(t, err, ErrPoolTimeout)
			for range 10 {
				_, err = b.Acquire(context.Background())
				assert.ErrorIs(t, err, ErrPoolTimeout)
			}
			time.Sleep(11 * time.Second)
			synctest.Wait()
			d, policy := poolTestSnapshots(t, testStack(t, "B"), "v1")
			_, err = p.Update(testStack(t, "B"), SignalMetrics, d, policy)
			assert.ErrorIs(t, err, ErrPoolCapacity)
			assert.EqualValues(t, 1, calls.Load())
			assert.Nil(t, client)
		})
	})
}

func TestPool_CloseAndForcedDrain(t *testing.T) {
	for _, releaseEarly := range []bool{true, false} {
		name := "deadline"
		if releaseEarly {
			name = "release"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
					return poolTestResource(target, g), nil
				}, poolTestOptions())
				b := poolTestBinding(t, p, testStack(t, "A"), "v1")
				l := acquireTestLease(t, b)
				closed := make(chan error, 1)
				go func() { closed <- p.Close(context.Background()) }()
				synctest.Wait()
				_, err := b.Acquire(context.Background())
				assert.ErrorIs(t, err, ErrPoolClosed)
				assert.Zero(t, l.Value().closed.Load())
				if releaseEarly {
					l.Release()
				} else {
					time.Sleep(time.Second)
				}
				synctest.Wait()
				err = <-closed
				if releaseEarly {
					assert.NoError(t, err)
				} else {
					assert.True(t, err == nil || errors.Is(err, ErrPoolTimeout))
				}
				assert.ErrorIs(t, l.Value().gate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
				assert.EqualValues(t, 1, l.Value().closed.Load())
				assert.NoError(t, p.Close(context.Background()))
			})
		})
	}
}

func TestPool_NonCooperativeCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls atomic.Int64
		p, err := NewPool(func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			r := poolTestResource(target, g)
			r.Shutdown = func(context.Context) error { calls.Add(1); <-release; return nil }
			return r, nil
		}, poolTestOptions())
		require.NoError(t, err)
		l := acquireTestLease(t, poolTestBinding(t, p, testStack(t, "A"), "v1"))
		l.Release()
		assert.ErrorIs(t, p.Close(context.Background()), ErrPoolTimeout)
		assert.EqualValues(t, 1, calls.Load())
		assert.ErrorIs(t, p.Close(context.Background()), ErrPoolTimeout)
		close(release)
		synctest.Wait()
		assert.NoError(t, p.Close(context.Background()))
		assert.EqualValues(t, 1, calls.Load())
	})
}

func TestPool_CleanupFailureRetainsCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := poolTestOptions()
		o.MaxResources = 1
		p, err := NewPool(func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			r := poolTestResource(target, g)
			r.Shutdown = func(context.Context) error { return errors.New("private cleanup failure") }
			return r, nil
		}, o)
		require.NoError(t, err)
		s := testStack(t, "A")
		l := acquireTestLease(t, poolTestBinding(t, p, s, "v1"))
		l.Release()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		d, policy := poolTestSnapshots(t, s, "v2")
		_, err = p.Update(s, SignalMetrics, d, policy)
		assert.ErrorIs(t, err, ErrPoolCapacity)
		assert.ErrorIs(t, l.Value().gate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
		err = p.Close(context.Background())
		assert.ErrorIs(t, err, ErrCleanup)
		assert.NotContains(t, err.Error(), "private")
	})
}

func TestPool_ExpiredBindingCannotBeRevived(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			return poolTestResource(target, g), nil
		}, poolTestOptions())
		s := testStack(t, "A")
		d, policy := poolTestSnapshots(t, s, "v1")
		d.validUntil = time.Now().Add(time.Second)
		old, err := p.Update(s, SignalMetrics, d, policy)
		require.NoError(t, err)
		l := acquireTestLease(t, old)
		time.Sleep(2 * time.Second)
		synctest.Wait()
		current := poolTestBinding(t, p, s, "v1")
		assert.NotSame(t, old, current)
		acquireTestLease(t, current)
		_, err = old.Acquire(context.Background())
		assert.ErrorIs(t, err, ErrBinding)
		assert.ErrorIs(t, l.Value().gate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
	})
}

func TestPool_ConcurrentAcquireRotateAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var built []*poolTestClient
		var mu sync.Mutex
		p := newTestPool(t, func(_ context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			r := poolTestResource(target, g)
			mu.Lock()
			built = append(built, r.Value)
			mu.Unlock()
			return r, nil
		}, poolTestOptions())
		s := testStack(t, "A")
		b := poolTestBinding(t, p, s, "v1")
		var workers sync.WaitGroup
		for range 30 {
			workers.Go(func() {
				for range 10 {
					l, err := b.Acquire(context.Background())
					if err != nil {
						assert.True(t, errors.Is(err, ErrBinding) || errors.Is(err, ErrPoolClosed))
						return
					}
					l.Release()
					l.Release()
				}
			})
		}
		rotated := poolTestBinding(t, p, s, "v2")
		for range 30 {
			workers.Go(func() {
				l, err := rotated.Acquire(context.Background())
				if err != nil {
					assert.ErrorIs(t, err, ErrPoolClosed)
					return
				}
				l.Release()
			})
		}
		assert.NoError(t, p.Close(context.Background()))
		workers.Wait()
		mu.Lock()
		defer mu.Unlock()
		for _, client := range built {
			assert.EqualValues(t, 1, client.closed.Load())
		}
	})
}
