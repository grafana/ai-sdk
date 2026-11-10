package extensionhost

import (
	"context"
	"sync"
	"sync/atomic"
)

// observations tracks asynchronous observations so shutdown can wait for them
// and drop later ones.
type observations struct {
	mu       sync.Mutex
	closing  bool
	inFlight sync.WaitGroup
	running  atomic.Int64
}

// reserve counts one observation in flight unless shutdown has begun, and
// reports whether it did. A reservation must be followed by run.
func (o *observations) reserve() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return false
	}
	o.inFlight.Add(1)
	o.running.Add(1)
	return true
}

// run runs a reserved observation on its own goroutine.
func (o *observations) run(observe func()) {
	go func() {
		defer o.inFlight.Done()
		defer o.running.Add(-1)
		observe()
	}()
}

// close stops accepting observations and waits for those in flight until ctx
// ends. It returns how many were still running then.
func (o *observations) close(ctx context.Context) int64 {
	o.mu.Lock()
	o.closing = true
	o.mu.Unlock()
	waitUntil(ctx, &o.inFlight)
	return o.running.Load()
}

// waitUntil waits for group until ctx ends.
func waitUntil(ctx context.Context, group *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
