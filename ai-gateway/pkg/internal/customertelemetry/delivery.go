package customertelemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrControllerOptions indicates invalid or already-owned composition resources.
var ErrControllerOptions = errors.New("customertelemetry: invalid controller options")

type deliverySink interface {
	signalValue() Signal
	attach(*Controller) (time.Duration, error)
	detach(*Controller)
	publish(Stack, CacheResult)
	close(context.Context) error
}

// Delivery connects one signal's typed pool to its controller. It accepts no
// request-owned snapshots. After attachment only the controller publishes state.
type Delivery[T any] struct {
	signal Signal
	pool   *Pool[T]
	owner  atomic.Pointer[Controller]
}

// NewDelivery prepares a signal without claiming its pool or starting discovery.
// The pool must be empty and open when passed to NewController.
func NewDelivery[T any](signal Signal, pool *Pool[T]) (*Delivery[T], error) {
	if !validSignal(signal) || pool == nil {
		return nil, ErrControllerOptions
	}
	return &Delivery[T]{signal: signal, pool: pool}, nil
}

// Controller owns discovery and fixed typed deliveries. Publication is ordered
// with cache updates; no exporter construction or cleanup runs under that lock.
// There is no live Cloud adapter or production recorder in this composition.
type Controller struct {
	cache      *Cache
	deliveries []deliverySink
	ready      atomic.Bool
	timeout    time.Duration
}

// NewController claims empty open pools through their deliveries. On failure the
// caller retains ownership; on success Close owns cache and pool shutdown. Pools
// cannot be shared between deliveries or used for external Update after claiming.
func NewController(destinations DestinationResolver, policies PolicyResolver, options CacheOptions, deliveries ...deliverySink) (*Controller, error) {
	if len(deliveries) == 0 {
		return nil, ErrControllerOptions
	}
	seen := make(map[Signal]bool, len(deliveries))
	for _, d := range deliveries {
		if d == nil || !validSignal(d.signalValue()) || seen[d.signalValue()] {
			return nil, ErrControllerOptions
		}
		seen[d.signalValue()] = true
	}
	cache, err := newCache(destinations, policies, options)
	if err != nil {
		return nil, err
	}
	c := &Controller{cache: cache, deliveries: append([]deliverySink(nil), deliveries...)}
	for i, d := range c.deliveries {
		timeout, err := d.attach(c)
		if err != nil {
			for _, claimed := range c.deliveries[:i] {
				claimed.detach(c)
			}
			cache.cancel()
			return nil, err
		}
		c.timeout = max(c.timeout, timeout)
	}
	cache.publisher = c
	cache.run()
	c.ready.Store(true)
	return c, nil
}

func (d *Delivery[T]) signalValue() Signal {
	if d == nil {
		return ""
	}
	return d.signal
}

func (d *Delivery[T]) attach(c *Controller) (time.Duration, error) {
	d.pool.mu.Lock()
	defer d.pool.mu.Unlock()
	if d.pool.closed || d.pool.owner != nil || len(d.pool.all) != 0 || !d.owner.CompareAndSwap(nil, c) {
		return 0, ErrControllerOptions
	}
	d.pool.owner = c
	return d.pool.options.ShutdownTimeout, nil
}

func (d *Delivery[T]) detach(c *Controller) {
	d.pool.mu.Lock()
	defer d.pool.mu.Unlock()
	if d.owner.CompareAndSwap(c, nil) {
		d.pool.owner = nil
	}
}

func (c *Controller) publish(stack Stack, result CacheResult) {
	for _, d := range c.deliveries {
		d.publish(stack, result)
	}
}

func (c *Controller) forget(stack Stack) { c.publish(stack, CacheResult{}) }

func (d *Delivery[T]) publish(stack Stack, result CacheResult) {
	_, _ = d.bind(stack, result)
}

func (d *Delivery[T]) bind(stack Stack, result CacheResult) (*Binding[T], error) {
	d.pool.mu.Lock()
	defer d.pool.mu.Unlock()
	return d.pool.update(stack, d.signal, result.Destinations, result.Policy)
}

// Acquire resolves an authorized stack, selects its current binding and policy,
// then constructs/acquires outside the publication lock. Failed acquisition means
// omit telemetry; it never invokes inference. The caller owns Release after the
// recording completes. Idle cache eviction can revoke a still-held lease.
func (d *Delivery[T]) Acquire(ctx context.Context, stack Stack) (*Lease[T], Policy, error) {
	c := d.owner.Load()
	if c == nil || !c.ready.Load() {
		return nil, Policy{}, ErrControllerOptions
	}
	result := c.cache.Lookup(ctx, stack)
	if err := ctx.Err(); err != nil {
		return nil, Policy{}, err
	}
	switch result.DestinationOutcome {
	case LookupClosed:
		return nil, Policy{}, ErrPoolClosed
	case LookupBudget, LookupCapacity, LookupCanceled:
		return nil, Policy{}, ErrBinding
	}
	binding, policy, err := d.selectCurrent(stack)
	if err != nil {
		return nil, Policy{}, err
	}
	lease, err := binding.Acquire(ctx)
	if err != nil {
		return nil, Policy{}, err
	}
	return lease, policy, nil
}

func (d *Delivery[T]) selectCurrent(stack Stack) (*Binding[T], Policy, error) {
	c := d.owner.Load().cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, Policy{}, ErrPoolClosed
	}
	entry := c.entries[stack]
	if entry == nil {
		return nil, Policy{}, ErrBinding
	}
	result := c.result(entry, c.now())
	binding, err := d.bind(stack, result)
	return binding, result.Policy, err
}

func (d *Delivery[T]) close(ctx context.Context) error { return d.pool.Close(ctx) }

// Close stops publication, then concurrently drains owned pools and waits for
// cache work. Waiting is bounded by the largest pool shutdown budget or an earlier
// caller deadline. Repeated calls do not restart cleanup or extend pool deadlines.
func (c *Controller) Close(ctx context.Context) error {
	c.cache.stop()
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	results := make(chan error, len(c.deliveries)+1)
	go func() { results <- c.cache.Close(ctx) }()
	for _, d := range c.deliveries {
		go func() { results <- d.close(ctx) }()
	}
	var errs []error
	for range len(c.deliveries) + 1 {
		errs = append(errs, <-results)
	}
	return errors.Join(errs...)
}
