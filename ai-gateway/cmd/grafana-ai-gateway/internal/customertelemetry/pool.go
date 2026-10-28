package customertelemetry

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrPoolOptions indicates missing construction dependencies or invalid bounds.
	ErrPoolOptions = errors.New("customertelemetry: invalid pool options")
	// ErrPoolClosed indicates shutdown has started.
	ErrPoolClosed = errors.New("customertelemetry: pool closed")
	// ErrPoolCapacity indicates current and retiring resources exhaust capacity.
	ErrPoolCapacity = errors.New("customertelemetry: pool capacity exhausted")
	// ErrBinding indicates obsolete, expired, or disallowed binding authority.
	ErrBinding = errors.New("customertelemetry: binding unavailable")
	// ErrBuild indicates construction failure without exposing adapter diagnostics.
	ErrBuild = errors.New("customertelemetry: exporter construction failed")
	// ErrPoolTimeout indicates an exhausted construction or shutdown budget.
	ErrPoolTimeout = errors.New("customertelemetry: pool operation timed out")
	// ErrCleanup indicates a resource shutdown failure without raw diagnostics.
	ErrCleanup = errors.New("customertelemetry: exporter cleanup failed")
)

// PoolOptions requires explicit resource, timing and retry bounds. These are not
// rollout recommendations. Exporter factories separately bound SDK queues/retries.
type PoolOptions struct {
	MaxResources        int
	BuildTimeout        time.Duration
	RetryInterval       time.Duration
	ExportTimeout       time.Duration
	IdleRetention       time.Duration
	MaintenanceInterval time.Duration
	ShutdownTimeout     time.Duration
}

// ExportTarget is immutable factory input without credentials or request context.
type ExportTarget struct {
	Stack       Stack
	Signal      Signal
	Destination Destination
}

// Resource retains a typed client and its required shutdown operation. Factories
// may return a partial resource with an error; its Shutdown will still run once.
// Shutdown must honor context and close exclusively owned transports as needed.
type Resource[T any] struct {
	Value    T
	Shutdown func(context.Context) error
}

// ResourceFactory resolves the exact credential reference/version and constructs
// a signal-specific resource. It must install the gate at its real export seam,
// honor cancellation and bound SDK queues/retries. No caller values are supplied.
type ResourceFactory[T any] func(context.Context, ExportTarget, *ExportGate) (Resource[T], error)

type poolKey struct {
	stack  Stack
	signal Signal
}
type poolBuild struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// Binding identifies one current immutable delivery configuration. Only the
// pool's ordered Update owner may create bindings; requests acquire from them.
type Binding[T any] struct {
	pool        *Pool[T]
	target      ExportTarget
	gate        *ExportGate
	lastUsed    time.Time
	refs        int
	retired     bool
	closing     bool
	quarantined bool
	build       *poolBuild
	resource    *Resource[T]
	failure     error
	retryAt     time.Time
}

// Lease owns one recording's use of an immutable resource. Call Release exactly
// once when recording finishes; repeated calls are harmless. Never retain or use
// Value after release. The resource's export gate also enforces revocation/expiry.
type Lease[T any] struct {
	binding *Binding[T]
	once    sync.Once
	value   T
}

// Value returns the typed client while the lease is held. Ownership and transport
// authentication remain with the pool's factory; do not mutate client settings.
func (l *Lease[T]) Value() T { return l.value }

// Release relinquishes recording ownership and allows retired-resource cleanup.
func (l *Lease[T]) Release() {
	l.once.Do(func() {
		p := l.binding.pool
		p.mu.Lock()
		l.binding.refs--
		p.notify()
		p.mu.Unlock()
	})
}

// Pool owns bounded exporter generations, including construction and retirement.
// It has no production consumer or default credential source. Its configuration
// owner must serialize authoritative Update calls per stack; opaque revisions
// cannot order arbitrary stale request snapshots.
type Pool[T any] struct {
	mu            sync.Mutex
	factory       ResourceFactory[T]
	options       PoolOptions
	current       map[poolKey]*Binding[T]
	all           map[*Binding[T]]struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	closed        bool
	deadline      time.Time
	cleanupFailed bool
	stop          chan struct{}
	wake          chan struct{}
	done          chan struct{}
}

// NewPool starts lifecycle maintenance without constructing a resource.
func NewPool[T any](factory ResourceFactory[T], options PoolOptions) (*Pool[T], error) {
	if factory == nil || options.MaxResources <= 0 || options.BuildTimeout <= 0 || options.RetryInterval <= 0 || options.ExportTimeout <= 0 || options.IdleRetention <= 0 || options.MaintenanceInterval <= 0 || options.MaintenanceInterval > options.IdleRetention || options.ShutdownTimeout <= 0 {
		return nil, ErrPoolOptions
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool[T]{factory: factory, options: options, current: make(map[poolKey]*Binding[T]), all: make(map[*Binding[T]]struct{}), ctx: ctx, cancel: cancel, stop: make(chan struct{}), wake: make(chan struct{}, 1), done: make(chan struct{})}
	go p.maintain()
	return p, nil
}

// Update publishes ordered authoritative snapshots for one stack/signal. It
// returns a reusable binding, or revokes all its generations if state is invalid
// or recording is disallowed. Matching transport settings refresh validity without
// rebuilding. Changed settings retire the old binding even at capacity; callers
// can retry Update after retirement frees capacity. No inference is invoked.
func (p *Pool[T]) Update(stack Stack, signal Signal, destinations Destinations, policy Policy) (*Binding[T], error) {
	if stack.namespace == "" || !validSignal(signal) {
		return nil, ErrBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, ErrPoolClosed
	}
	key := poolKey{stack, signal}
	now := time.Now()
	destination, allowed := RecordingDestination(stack, destinations, policy, signal, now)
	if !allowed {
		for b := range p.all {
			if b.target.Stack == stack && b.target.Signal == signal {
				p.retire(b, true)
			}
		}
		p.notify()
		return nil, ErrBinding
	}
	until := destinations.validUntil
	if policy.validUntil.Before(until) {
		until = policy.validUntil
	}
	if old := p.current[key]; old != nil {
		if old.target.Destination == destination && old.gate.valid() {
			old.gate.refresh(until)
			return old, nil
		}
		p.retire(old, !old.gate.valid())
	}
	p.notify()
	if len(p.all) >= p.options.MaxResources {
		return nil, ErrPoolCapacity
	}
	b := &Binding[T]{pool: p, target: ExportTarget{stack, signal, destination}, gate: newExportGate(destination.Endpoint, until, p.options.ExportTimeout), lastUsed: now}
	p.current[key] = b
	p.all[b] = struct{}{}
	return b, nil
}

// Acquire shares construction and returns recording ownership. Cancellation only
// releases this waiter. Old bindings cannot acquire after rotation or revocation.
func (b *Binding[T]) Acquire(ctx context.Context) (*Lease[T], error) {
	p := b.pool
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}
		if b.retired || !b.gate.valid() {
			p.mu.Unlock()
			return nil, ErrBinding
		}
		b.lastUsed = time.Now()
		if b.failure != nil && (time.Now().Before(b.retryAt) || b.closing) {
			err := b.failure
			p.mu.Unlock()
			return nil, err
		}
		if b.resource != nil && b.failure == nil {
			b.refs++
			lease := &Lease[T]{binding: b, value: b.resource.Value}
			p.mu.Unlock()
			return lease, nil
		}
		if b.resource != nil {
			p.mu.Unlock()
			return nil, ErrBuild
		}
		if b.build == nil {
			p.start(b)
		}
		a := b.build
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.stop:
			return nil, ErrPoolClosed
		case <-a.ctx.Done():
			p.mu.Lock()
			closed := p.closed
			p.mu.Unlock()
			if closed {
				return nil, ErrPoolClosed
			}
			select {
			case <-a.done:
			default:
				return nil, ErrPoolTimeout
			}
		case <-a.done:
		}
	}
}

func (p *Pool[T]) start(b *Binding[T]) {
	ctx, cancel := context.WithTimeout(p.ctx, p.options.BuildTimeout)
	a := &poolBuild{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	b.build = a
	go func() {
		resource, err := p.factory(ctx, b.target, b.gate)
		p.mu.Lock()
		defer p.mu.Unlock()
		b.build = nil
		if resource.Shutdown != nil {
			b.resource = &resource
		}
		b.failure = nil
		if err != nil || resource.Shutdown == nil {
			b.failure = ErrBuild
		}
		if ctx.Err() != nil {
			b.failure = ErrPoolTimeout
			p.retire(b, true)
		}
		if b.failure != nil {
			b.retryAt = time.Now().Add(p.options.RetryInterval)
		}
		close(a.done)
		a.cancel()
		p.notify()
	}()
}

func (p *Pool[T]) retire(b *Binding[T], revoke bool) {
	b.retired = true
	key := poolKey{b.target.Stack, b.target.Signal}
	if p.current[key] == b {
		delete(p.current, key)
	}
	if revoke {
		b.gate.revoke()
	}
}

func (p *Pool[T]) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Pool[T]) cleanup(b *Binding[T]) {
	b.closing = true
	resource := b.resource
	deadline := time.Now().Add(p.options.ShutdownTimeout)
	if p.closed && p.deadline.Before(deadline) {
		deadline = p.deadline
	}
	go func() {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		err := resource.Shutdown(ctx)
		cancel()
		p.mu.Lock()
		defer p.mu.Unlock()
		if err != nil {
			p.cleanupFailed = true
			b.quarantined = true
			p.retire(b, true)
		}
		b.resource = nil
		b.closing = false
		if b.retired && !b.quarantined {
			b.gate.revoke()
			delete(p.all, b)
		} else if !b.retired {
			b.gate.mu.Lock()
			until := b.gate.until
			b.gate.mu.Unlock()
			b.gate.revoke()
			b.gate = newExportGate(b.target.Destination.Endpoint, until, p.options.ExportTimeout)
		}
		p.notify()
	}()
}

func (p *Pool[T]) maintain() {
	for {
		p.mu.Lock()
		now := time.Now()
		delay := p.options.MaintenanceInterval
		for b := range p.all {
			if b.quarantined {
				if p.closed {
					delete(p.all, b)
				}
				continue
			}
			if !b.gate.valid() {
				p.retire(b, true)
			}
			if b.refs == 0 && !now.Before(b.lastUsed.Add(p.options.IdleRetention)) {
				p.retire(b, false)
			}
			force := p.closed && !now.Before(p.deadline)
			if force {
				b.gate.revoke()
			}
			if b.build != nil || b.closing {
				continue
			}
			if b.resource != nil && (b.retired || b.failure != nil) && (b.refs == 0 || force) {
				p.cleanup(b)
			}
			if b.resource == nil && b.retired {
				b.gate.revoke()
				delete(p.all, b)
			}
		}
		if p.closed && len(p.all) == 0 {
			close(p.done)
			p.mu.Unlock()
			return
		}
		if p.closed && now.Before(p.deadline) {
			delay = min(delay, p.deadline.Sub(now))
		}
		p.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-p.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// Close stops admission and bounds draining by the first caller's deadline and
// ShutdownTimeout. At that bound all remaining gates are revoked. Non-cooperative
// work remains accounted until it returns; repeated calls never restart cleanup.
func (p *Pool[T]) Close(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.deadline = time.Now().Add(p.options.ShutdownTimeout)
		if deadline, ok := ctx.Deadline(); ok && deadline.Before(p.deadline) {
			p.deadline = deadline
		}
		close(p.stop)
		p.cancel()
		for b := range p.all {
			p.retire(b, false)
			b.gate.mu.Lock()
			if p.deadline.Before(b.gate.until) {
				b.gate.until = p.deadline
			}
			b.gate.mu.Unlock()
		}
		p.notify()
	}
	deadline := p.deadline
	p.mu.Unlock()
	select {
	case <-p.done:
		return p.closeResult()
	default:
	}
	timer := time.NewTimer(max(0, time.Until(deadline)))
	defer timer.Stop()
	select {
	case <-p.done:
		return p.closeResult()
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrPoolTimeout
	}
}

func (p *Pool[T]) closeResult() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cleanupFailed {
		return ErrCleanup
	}
	return nil
}
