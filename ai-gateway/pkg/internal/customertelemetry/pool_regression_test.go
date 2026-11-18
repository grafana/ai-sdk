package customertelemetry

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPool_ConstructionTimeoutMustRespectRetryInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		o := poolTestOptions()
		o.RetryInterval = time.Minute
		p := newTestPool(t, func(ctx context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			calls.Add(1)
			<-ctx.Done()
			return Resource[*poolTestClient]{}, ctx.Err()
		}, o)
		stack := testStack(t, "A")
		first := poolTestBinding(t, p, stack, "v1")
		_, err := first.Acquire(context.Background())
		require.Error(t, err)
		synctest.Wait()
		second := poolTestBinding(t, p, stack, "v1")
		_, err = second.Acquire(context.Background())
		require.Error(t, err)
		assert.EqualValues(t, 1, calls.Load(), "reapplying unchanged metadata must preserve construction retry suppression")
	})
}

func TestPool_ActiveColdAcquisitionIsNotIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := poolTestOptions()
		o.IdleRetention = 100 * time.Millisecond
		o.MaintenanceInterval = 10 * time.Millisecond
		o.BuildTimeout = time.Second
		p := newTestPool(t, func(ctx context.Context, target ExportTarget, g *ExportGate) (Resource[*poolTestClient], error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return poolTestResource(target, g), nil
			case <-ctx.Done():
				return Resource[*poolTestClient]{}, ctx.Err()
			}
		}, o)
		b := poolTestBinding(t, p, testStack(t, "A"), "v1")
		l, err := b.Acquire(context.Background())
		if l != nil {
			l.Release()
		}
		require.NoError(t, err, "live waiter must receive successful construction within the configured build budget")
	})
}

func TestPool_TimeoutCleanupAndRetry(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "empty"
		if partial {
			name = "partial"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				o := poolTestOptions()
				o.IdleRetention, o.RetryInterval = time.Second, 5*time.Second
				var calls atomic.Int64
				var failedGate *ExportGate
				var failedClient *poolTestClient
				p := newTestPool(t, func(ctx context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
					if target.Stack.Namespace() == "A" && calls.Add(1) == 1 {
						failedGate = gate
						<-ctx.Done()
						if partial {
							r := poolTestResource(target, gate)
							failedClient = r.Value
							return r, ctx.Err()
						}
						return Resource[*poolTestClient]{}, ctx.Err()
					}
					return poolTestResource(target, gate), nil
				}, o)
				a := testStack(t, "A")
				_, err := poolTestBinding(t, p, a, "v1").Acquire(context.Background())
				require.ErrorIs(t, err, ErrPoolTimeout)
				synctest.Wait()
				require.ErrorIs(t, failedGate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
				if partial {
					require.EqualValues(t, 1, failedClient.closed.Load())
				}
				time.Sleep(2 * time.Second)
				synctest.Wait()
				_, err = poolTestBinding(t, p, a, "v1").Acquire(context.Background())
				require.ErrorIs(t, err, ErrPoolTimeout)
				require.EqualValues(t, 1, calls.Load())
				other := acquireTestLease(t, poolTestBinding(t, p, testStack(t, "B"), "v1"))
				other.Release()
				time.Sleep(3 * time.Second)
				synctest.Wait()
				lease := acquireTestLease(t, poolTestBinding(t, p, a, "v1"))
				require.EqualValues(t, 2, calls.Load())
				require.NoError(t, lease.Value().gate.Run(context.Background(), func(context.Context) error { return nil }))
				require.ErrorIs(t, failedGate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
			})
		})
	}
}

func TestPool_TimeoutDoesNotSuppressNewCredential(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := poolTestOptions()
		o.RetryInterval = time.Minute
		var calls atomic.Int64
		p := newTestPool(t, func(ctx context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
			calls.Add(1)
			if target.Destination.Credential.Version == "v1" {
				<-ctx.Done()
				return Resource[*poolTestClient]{}, ctx.Err()
			}
			return poolTestResource(target, gate), nil
		}, o)
		a := testStack(t, "A")
		_, err := poolTestBinding(t, p, a, "v1").Acquire(context.Background())
		require.ErrorIs(t, err, ErrPoolTimeout)
		synctest.Wait()
		lease := acquireTestLease(t, poolTestBinding(t, p, a, "v2"))
		assert.Equal(t, "v2", lease.Value().target.Destination.Credential.Version)
		assert.EqualValues(t, 2, calls.Load())
	})
}

func TestPool_ColdWaitersCancelAndRetire(t *testing.T) {
	for _, abandon := range []bool{false, true} {
		name := "remaining waiter"
		if abandon {
			name = "all canceled"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				o := poolTestOptions()
				o.IdleRetention, o.MaintenanceInterval = 100*time.Millisecond, 10*time.Millisecond
				var calls atomic.Int64
				var client *poolTestClient
				p := newTestPool(t, func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
					calls.Add(1)
					time.Sleep(200 * time.Millisecond)
					r := poolTestResource(target, gate)
					client = r.Value
					return r, nil
				}, o)
				b := poolTestBinding(t, p, testStack(t, "A"), "v1")
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				canceled := make(chan error, 1)
				go func() { _, err := b.Acquire(ctx); canceled <- err }()
				var result chan *Lease[*poolTestClient]
				if !abandon {
					result = make(chan *Lease[*poolTestClient], 1)
					go func() {
						lease, err := b.Acquire(context.Background())
						assert.NoError(t, err)
						result <- lease
					}()
				}
				synctest.Wait()
				cancel()
				require.ErrorIs(t, <-canceled, context.Canceled)
				time.Sleep(250 * time.Millisecond)
				synctest.Wait()
				require.EqualValues(t, 1, calls.Load())
				if !abandon {
					lease := <-result
					require.NotNil(t, lease)
					assert.Zero(t, client.closed.Load())
					lease.Release()
					time.Sleep(150 * time.Millisecond)
					synctest.Wait()
				}
				assert.EqualValues(t, 1, client.closed.Load())
				p.mu.Lock()
				assert.Empty(t, p.all)
				p.mu.Unlock()
			})
		})
	}
}
