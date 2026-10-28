package customertelemetry

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrCacheOptions indicates missing resolvers or invalid cache limits.
	ErrCacheOptions = errors.New("customertelemetry: invalid cache options")
	// ErrResolutionDenied marks authoritative denial by a metadata service.
	// Adapters must map this from a verified response, never from error text.
	ErrResolutionDenied = errors.New("customertelemetry: resolution denied")
)

// LookupOutcome is a bounded classification of a component's latest attempt.
// A transient failure can accompany a still-valid snapshot.
type LookupOutcome string

const (
	// LookupReady identifies an available, recording-capable component.
	LookupReady LookupOutcome = "ready"
	// LookupNotReady identifies a valid empty destination or non-recording policy.
	LookupNotReady LookupOutcome = "not-ready"
	// LookupInvalid identifies invalid attribution, data, or expired output.
	LookupInvalid LookupOutcome = "invalid"
	// LookupDenied identifies authoritative metadata access denial.
	LookupDenied LookupOutcome = "denied"
	// LookupFailed identifies a transient metadata error without exposing it.
	LookupFailed LookupOutcome = "failed"
	// LookupTimeout identifies an exhausted metadata lookup budget.
	LookupTimeout LookupOutcome = "timeout"
	// LookupCanceled identifies a canceled request waiting for metadata.
	LookupCanceled LookupOutcome = "canceled"
	// LookupBudget identifies insufficient request time for a cold lookup.
	LookupBudget LookupOutcome = "budget"
	// LookupCapacity identifies exhausted cache capacity.
	LookupCapacity LookupOutcome = "capacity"
	// LookupClosed identifies a cache that no longer admits work.
	LookupClosed LookupOutcome = "closed"
	// LookupExpired identifies a snapshot past its hard validity deadline.
	LookupExpired LookupOutcome = "expired"
)

// CacheOptions supplies explicit operational bounds; there are no rollout defaults.
type CacheOptions struct {
	MaxEntries          int
	LookupTimeout       time.Duration
	InferenceHeadroom   time.Duration
	RefreshInterval     time.Duration
	RetryInterval       time.Duration
	IdleRetention       time.Duration
	MaintenanceInterval time.Duration
}

// CacheResult contains independently current snapshots and latest lookup outcomes.
// Missing or expired snapshots are zero values. Always use RecordingDestination
// at selection time; an outcome alone neither permits recording nor supplies auth.
type CacheResult struct {
	Destinations       Destinations
	Policy             Policy
	DestinationOutcome LookupOutcome
	PolicyOutcome      LookupOutcome
}

type cacheComponent uint8

const (
	destinationComponent cacheComponent = iota
	policyComponent
)

type cacheValue struct {
	destinations Destinations
	policy       Policy
	validUntil   time.Time
}

type cacheSlot struct {
	value   cacheValue
	outcome LookupOutcome
	next    time.Time
	pending *cacheAttempt
}

type cacheAttempt struct {
	cancel   context.CancelFunc
	stop     func() bool
	done     chan struct{}
	finished bool
}

type cacheEntry struct {
	stack    Stack
	lastUsed time.Time
	slots    [2]cacheSlot
}

// Cache owns bounded metadata work and immutable per-stack snapshots.
// It has no inference callback, exporter, secret material, or production wiring.
// Resolver implementations must honor cancellation and bound transport work.
type Cache struct {
	mu           sync.Mutex
	destinations DestinationResolver
	policies     PolicyResolver
	options      CacheOptions
	now          func() time.Time
	entries      map[Stack]*cacheEntry
	ctx          context.Context
	cancel       context.CancelFunc
	closed       bool
	wake         chan struct{}
	done         chan struct{}
	workers      sync.WaitGroup
}

// NewCache creates an empty cache without invoking either resolver. Close must be
// called by its owner. No caller context is retained or used as shared authority.
func NewCache(destinations DestinationResolver, policies PolicyResolver, options CacheOptions) (*Cache, error) {
	if destinations == nil || policies == nil || options.MaxEntries <= 0 ||
		options.LookupTimeout <= 0 || options.InferenceHeadroom < 0 ||
		options.RefreshInterval <= 0 || options.RetryInterval <= 0 ||
		options.RetryInterval > options.RefreshInterval || options.IdleRetention <= 0 ||
		options.MaintenanceInterval <= 0 || options.MaintenanceInterval > options.RetryInterval ||
		options.MaintenanceInterval > options.IdleRetention {
		return nil, ErrCacheOptions
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{destinations: destinations, policies: policies, options: options, now: time.Now,
		entries: make(map[Stack]*cacheEntry), ctx: ctx, cancel: cancel,
		wake: make(chan struct{}, 1), done: make(chan struct{})}
	c.workers.Add(1)
	go c.maintain()
	go func() { c.workers.Wait(); close(c.done) }()
	return c, nil
}

// Lookup resolves an authorized stack on demand, sharing in-flight component work.
// Cold waiting is bounded independently of the inference deadline. A canceled
// waiter does not cancel the shared lookup. Failure means omit telemetry.
func (c *Cache) Lookup(ctx context.Context, stack Stack) CacheResult {
	if ctx.Err() != nil {
		return cacheOutcome(LookupCanceled)
	}
	if stack.namespace == "" {
		return cacheOutcome(LookupInvalid)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return cacheOutcome(LookupClosed)
	}
	now := c.now()
	entry := c.entries[stack]
	cold := entry == nil || !entry.slots[0].value.validUntil.After(now) || !entry.slots[1].value.validUntil.After(now)
	if cold {
		if deadline, ok := ctx.Deadline(); ok && (deadline.Sub(now) < c.options.LookupTimeout ||
			deadline.Sub(now)-c.options.LookupTimeout < c.options.InferenceHeadroom) {
			c.mu.Unlock()
			return cacheOutcome(LookupBudget)
		}
	}
	if entry == nil {
		c.prune(now)
		if len(c.entries) >= c.options.MaxEntries {
			c.mu.Unlock()
			return cacheOutcome(LookupCapacity)
		}
		entry = &cacheEntry{stack: stack}
		c.entries[stack] = entry
	}
	entry.lastUsed = now
	var waiting []<-chan struct{}
	for kind := destinationComponent; kind <= policyComponent; kind++ {
		slot := &entry.slots[kind]
		if slot.pending == nil && !now.Before(slot.next) {
			c.start(entry, kind)
		}
		if !slot.value.validUntil.After(now) && slot.pending != nil && !slot.pending.finished {
			waiting = append(waiting, slot.pending.done)
		}
	}
	c.notify()
	c.mu.Unlock()
	for _, done := range waiting {
		select {
		case <-ctx.Done():
			return cacheOutcome(LookupCanceled)
		case <-c.ctx.Done():
			return cacheOutcome(LookupClosed)
		case <-done:
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return cacheOutcome(LookupCanceled)
	}
	if c.closed {
		return cacheOutcome(LookupClosed)
	}
	if c.entries[stack] != entry {
		return cacheOutcome(LookupExpired)
	}
	return c.result(entry, c.now())
}

// Close stops admissions, cancels owned lookups, and waits only within ctx's
// deadline. Repeated calls may wait again for a non-cooperative resolver to exit.
// Go cannot terminate arbitrary resolver code; canceled calls retain their slots.
func (c *Cache) Close(ctx context.Context) error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.cancel()
	}
	c.mu.Unlock()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func cacheOutcome(outcome LookupOutcome) CacheResult {
	return CacheResult{DestinationOutcome: outcome, PolicyOutcome: outcome}
}

func (c *Cache) result(entry *cacheEntry, now time.Time) CacheResult {
	result := CacheResult{DestinationOutcome: entry.slots[0].outcome, PolicyOutcome: entry.slots[1].outcome}
	if entry.slots[0].value.validUntil.After(now) {
		result.Destinations = entry.slots[0].value.destinations
	} else if result.DestinationOutcome == LookupReady || result.DestinationOutcome == LookupNotReady {
		result.DestinationOutcome = LookupExpired
	}
	if entry.slots[1].value.validUntil.After(now) {
		result.Policy = entry.slots[1].value.policy
	} else if result.PolicyOutcome == LookupReady || result.PolicyOutcome == LookupNotReady {
		result.PolicyOutcome = LookupExpired
	}
	return result
}

func (c *Cache) start(entry *cacheEntry, kind cacheComponent) {
	ctx, cancel := context.WithTimeout(c.ctx, c.options.LookupTimeout)
	a := &cacheAttempt{cancel: cancel, done: make(chan struct{})}
	entry.slots[kind].pending = a
	a.stop = context.AfterFunc(ctx, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !a.finished {
			if !c.closed {
				c.apply(entry, kind, cacheValue{}, LookupTimeout, c.now())
			}
			a.finished = true
			close(a.done)
		}
	})
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		value, err := c.resolve(ctx, entry.stack, kind)
		c.mu.Lock()
		defer c.mu.Unlock()
		a.stop()
		if !a.finished {
			if !c.closed {
				outcome := classifyLookup(err)
				if ctx.Err() != nil {
					outcome = LookupTimeout
				}
				c.apply(entry, kind, value, outcome, c.now())
			}
			a.finished = true
			close(a.done)
		}
		entry.slots[kind].pending = nil
		a.cancel()
		c.notify()
	}()
}

func (c *Cache) resolve(ctx context.Context, stack Stack, kind cacheComponent) (cacheValue, error) {
	if kind == destinationComponent {
		d, err := c.destinations.ResolveDestinations(ctx, stack, c.now())
		if err != nil {
			return cacheValue{}, err
		}
		if d.stack != stack || !validMetadata(d.revision, d.validUntil, c.now()) {
			return cacheValue{}, ErrSnapshot
		}
		return cacheValue{destinations: d, validUntil: d.validUntil}, nil
	}
	p, err := c.policies.ResolvePolicy(ctx, stack, c.now())
	if err != nil {
		return cacheValue{}, err
	}
	if p.stack != stack || !validMetadata(p.revision, p.validUntil, c.now()) {
		return cacheValue{}, ErrSnapshot
	}
	return cacheValue{policy: p, validUntil: p.validUntil}, nil
}

func classifyLookup(err error) LookupOutcome {
	switch {
	case err == nil:
		return LookupReady
	case errors.Is(err, ErrResolutionDenied):
		return LookupDenied
	case errors.Is(err, ErrStack), errors.Is(err, ErrSnapshot), errors.Is(err, ErrDestination), errors.Is(err, ErrPolicy):
		return LookupInvalid
	case errors.Is(err, context.DeadlineExceeded):
		return LookupTimeout
	default:
		return LookupFailed
	}
}

func (c *Cache) apply(entry *cacheEntry, kind cacheComponent, value cacheValue, outcome LookupOutcome, now time.Time) {
	slot := &entry.slots[kind]
	delay := c.options.RetryInterval
	if outcome == LookupReady {
		if !value.validUntil.After(now) {
			outcome = LookupInvalid
		} else {
			slot.value = value
			ready := len(value.destinations.signals) > 0
			if kind == policyComponent {
				for _, allowed := range value.policy.allowed {
					ready = ready || allowed
				}
			}
			if ready {
				delay = max(c.options.RetryInterval, min(c.options.RefreshInterval, value.validUntil.Sub(now)/2))
			} else {
				outcome = LookupNotReady
			}
		}
	}
	if outcome == LookupDenied || outcome == LookupInvalid {
		slot.value = cacheValue{}
	}
	slot.outcome = outcome
	slot.next = now.Add(delay)
}

func (c *Cache) prune(now time.Time) {
	for stack, entry := range c.entries {
		if !now.Before(entry.lastUsed.Add(c.options.IdleRetention)) && entry.slots[0].pending == nil && entry.slots[1].pending == nil {
			delete(c.entries, stack)
		}
	}
}

func (c *Cache) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Cache) maintain() {
	defer c.workers.Done()
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		now := c.now()
		c.prune(now)
		delay := c.options.MaintenanceInterval
		for _, entry := range c.entries {
			idleAt := entry.lastUsed.Add(c.options.IdleRetention)
			if !now.Before(idleAt) {
				continue
			}
			delay = min(delay, idleAt.Sub(now))
			for kind := destinationComponent; kind <= policyComponent; kind++ {
				slot := &entry.slots[kind]
				if slot.pending != nil {
					continue
				}
				if !now.Before(slot.next) {
					c.start(entry, kind)
				} else {
					delay = min(delay, slot.next.Sub(now))
				}
			}
		}
		c.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return
		case <-c.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}
