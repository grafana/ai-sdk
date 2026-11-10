package extensionhost

import (
	"context"
	"sync"
)

// observations tracks asynchronous observations so shutdown can wait for them
// and drop later ones.
type observations struct {
	mu       sync.Mutex
	closing  bool
	inFlight sync.WaitGroup
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
	return true
}

// run runs a reserved observation on its own goroutine.
func (o *observations) run(observe func()) {
	go func() {
		defer o.inFlight.Done()
		observe()
	}()
}

// close stops accepting observations and waits for those in flight until ctx
// ends.
func (o *observations) close(ctx context.Context) {
	o.mu.Lock()
	o.closing = true
	o.mu.Unlock()
	done := make(chan struct{})
	go func() {
		o.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
