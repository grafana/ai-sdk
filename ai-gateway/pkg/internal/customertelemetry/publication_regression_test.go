package customertelemetry

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_RetiringValidity(t *testing.T) {
	for _, capacity := range []int{1, 4} {
		t.Run(fmt.Sprintf("capacity-%d", capacity), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				o := poolTestOptions()
				o.MaxResources = capacity
				p := newTestPool(t, func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
					return poolTestResource(target, gate), nil
				}, o)
				stack := testStack(t, "A")
				old := acquireTestLease(t, poolTestBinding(t, p, stack, "v1"))
				d, policy := poolTestSnapshots(t, stack, "v2")
				policy.validUntil = time.Now().Add(5 * time.Second)
				_, err := p.Update(stack, SignalMetrics, d, policy)
				if capacity == 1 {
					require.ErrorIs(t, err, ErrPoolCapacity)
				} else {
					require.NoError(t, err)
					policy.validUntil = time.Now().Add(time.Minute)
					_, err = p.Update(stack, SignalMetrics, d, policy)
					require.NoError(t, err)
				}
				time.Sleep(5 * time.Second)
				assert.ErrorIs(t, old.Value().gate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
			})
		})
	}
}

func TestDelivery_BackgroundRevocation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var disabled atomic.Bool
		r := &cacheTestResolver{policy: func(_ context.Context, stack Stack, now time.Time) (Policy, error) {
			preference := PreferenceAbsent
			if disabled.Load() {
				preference = PreferenceDisabled
			}
			return cachePolicy(stack, now, time.Minute, preference)
		}}
		p := newTestPool(t, func(_ context.Context, target ExportTarget, gate *ExportGate) (Resource[*poolTestClient], error) {
			return poolTestResource(target, gate), nil
		}, poolTestOptions())
		stack := testStack(t, "A")
		delivery, err := NewDelivery(SignalMetrics, p)
		require.NoError(t, err)
		controller, err := NewController(r, r, cacheTestOptions(), delivery)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, controller.Close(context.Background())) })
		lease, _, err := delivery.Acquire(context.Background(), stack)
		require.NoError(t, err)
		t.Cleanup(lease.Release)
		disabled.Store(true)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.ErrorIs(t, lease.Value().gate.Run(context.Background(), func(context.Context) error { return nil }), ErrExportDenied)
	})
}
