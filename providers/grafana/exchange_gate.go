package grafana

import (
	"context"
	"sync"
)

// exchangeGates admits one token exchange per subject at a time. Authlib runs
// concurrent exchanges for one subject as a single call under the first
// caller's context, so waiting here keeps each caller's cancellation its own
// while exchanges for different subjects proceed in parallel.
type exchangeGates struct {
	mu    sync.Mutex
	gates map[string]*exchangeGate
}

type exchangeGate struct {
	slot    chan struct{}
	waiters int
}

func (g *exchangeGates) acquire(ctx context.Context, subject string) (release func(), err error) {
	g.mu.Lock()
	if g.gates == nil {
		g.gates = map[string]*exchangeGate{}
	}
	gate, ok := g.gates[subject]
	if !ok {
		gate = &exchangeGate{slot: make(chan struct{}, 1)}
		g.gates[subject] = gate
	}
	gate.waiters++
	g.mu.Unlock()
	leave := func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if gate.waiters--; gate.waiters == 0 {
			delete(g.gates, subject)
		}
	}
	select {
	case gate.slot <- struct{}{}:
		return func() { <-gate.slot; leave() }, nil
	case <-ctx.Done():
		leave()
		return nil, ctx.Err()
	}
}
