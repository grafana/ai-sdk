package extensionhost

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"
)

// Tenant configuration cache defaults.
const (
	// defaultTenantTTL is how long a namespace's configuration is served
	// before a call refreshes it in the background.
	defaultTenantTTL = time.Minute
	// defaultTenantMaxStaleness bounds how long the last configuration read
	// successfully is served while refreshes fail.
	defaultTenantMaxStaleness = 10 * time.Minute
	// defaultTenantReadTimeout bounds one namespace read.
	defaultTenantReadTimeout = 5 * time.Second
	// defaultTenantFailureBackoff is the earliest retry after a failed read.
	defaultTenantFailureBackoff = 30 * time.Second
	// defaultTenantMaxNamespaces bounds the namespaces cached at once.
	defaultTenantMaxNamespaces = 1024
)

// errTenantsClosed is the outcome of a call that needs namespace
// configuration after shutdown has begun.
var errTenantsClosed = errors.New("extensions: tenant configuration is closed")

// TenantCacheConfig bounds the tenant configuration cache. Zero fields use
// the defaults.
type TenantCacheConfig struct {
	TTL            time.Duration
	MaxStaleness   time.Duration
	ReadTimeout    time.Duration
	FailureBackoff time.Duration
	MaxNamespaces  int
}

func (c TenantCacheConfig) withDefaults() TenantCacheConfig {
	if c.TTL <= 0 {
		c.TTL = defaultTenantTTL
	}
	if c.MaxStaleness <= 0 {
		c.MaxStaleness = defaultTenantMaxStaleness
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = defaultTenantReadTimeout
	}
	if c.FailureBackoff <= 0 {
		c.FailureBackoff = defaultTenantFailureBackoff
	}
	if c.MaxNamespaces <= 0 {
		c.MaxNamespaces = defaultTenantMaxNamespaces
	}
	return c
}

// tenantCache caches each namespace's configuration snapshot, read as the
// plugin on its own root context. A namespace's first call waits for its
// first read; concurrent calls share one read, and later calls are served
// from the cache while a background refresh runs after the TTL. A failed
// read is retried after the backoff; the last successful read is served for
// at most the maximum staleness, then the failure snapshot.
type tenantCache struct {
	host   *Host
	reader TenantReader
	config TenantCacheConfig
	root   context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	closed  bool
	// loads counts reads in flight, so shutdown can wait for them. Add runs
	// under mu while the cache is open.
	loads sync.WaitGroup
}

// tenantEntry is one namespace's cache state. The cache lock guards it.
type tenantEntry struct {
	namespace string
	snapshot  *snapshot
	// loadedAt is the time of the last successful read; zero before one.
	loadedAt time.Time
	// failed reports whether the last read failed.
	failed  bool
	retryAt time.Time
	// loading is closed when the read in flight completes; nil without one.
	loading chan struct{}
	evicted bool
	// unknown holds the unknown types already logged and counted.
	unknown map[string]bool
}

func newTenantCache(host *Host, reader TenantReader, config TenantCacheConfig) *tenantCache {
	root, cancel := context.WithCancel(context.Background())
	return &tenantCache{host: host, reader: reader, config: config, root: root, cancel: cancel, entries: map[string]*list.Element{}, order: list.New()}
}

// acquire returns namespace's snapshot with a reference the caller must
// release. It waits for the namespace's first read, or until ctx ends. A
// call without a namespace never reads and gets the failure snapshot.
func (c *tenantCache) acquire(ctx context.Context, namespace string) (*snapshot, error) {
	if namespace == "" {
		return c.host.failed, nil
	}
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, errTenantsClosed
		}
		entry := c.touch(namespace)
		if entry.snapshot == nil {
			loading := c.start(entry)
			c.mu.Unlock()
			select {
			case <-loading:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.mu.Lock()
			if entry.evicted || entry.snapshot == nil {
				c.mu.Unlock()
				continue
			}
		} else {
			now := c.host.clock()
			c.expire(entry, now)
			if c.due(entry, now) {
				c.start(entry)
			}
		}
		served := entry.snapshot
		served.acquire()
		c.mu.Unlock()
		return served, nil
	}
}

// touch returns namespace's entry as the most recently used, creating it and
// evicting the least recently used other entries without a read in flight
// beyond the bound. Entries with reads in flight can exceed the bound until a
// later touch. The caller holds the lock.
func (c *tenantCache) touch(namespace string) *tenantEntry {
	if element, ok := c.entries[namespace]; ok {
		c.order.MoveToFront(element)
		return element.Value.(*tenantEntry)
	}
	entry := &tenantEntry{namespace: namespace, unknown: map[string]bool{}}
	c.entries[namespace] = c.order.PushFront(entry)
	for element := c.order.Back(); element != nil && c.order.Len() > c.config.MaxNamespaces; {
		previous := element.Prev()
		victim := element.Value.(*tenantEntry)
		if victim.loading == nil && victim != entry {
			c.evict(element)
		}
		element = previous
	}
	return entry
}

// evict removes an entry and retires its snapshot. The caller holds the lock.
func (c *tenantCache) evict(element *list.Element) {
	victim := element.Value.(*tenantEntry)
	c.order.Remove(element)
	delete(c.entries, victim.namespace)
	victim.evicted = true
	if victim.snapshot != nil {
		c.host.retire(victim.snapshot)
	}
}

// due reports whether a call should start a background refresh. The caller
// holds the lock.
func (c *tenantCache) due(entry *tenantEntry, now time.Time) bool {
	if entry.loading != nil || now.Before(entry.retryAt) {
		return false
	}
	return entry.failed || !now.Before(entry.loadedAt.Add(c.config.TTL))
}

// expire replaces the last configuration read successfully with the failure
// snapshot once reads have kept failing past the maximum staleness since it
// was read. An entry whose last read succeeded is never expired: its next
// refresh replaces it. The caller holds the lock.
func (c *tenantCache) expire(entry *tenantEntry, now time.Time) {
	if !entry.failed || !entry.snapshot.read || now.Before(entry.loadedAt.Add(c.config.MaxStaleness)) {
		return
	}
	c.host.logger.Warn("gateway tenant configuration expired", "namespace", entry.namespace)
	c.swap(entry, c.host.failed)
}

// swap serves next for entry and retires the snapshot it replaces. The
// caller holds the lock.
func (c *tenantCache) swap(entry *tenantEntry, next *snapshot) {
	previous := entry.snapshot
	entry.snapshot = next
	if previous != nil && previous != next {
		c.host.retire(previous)
	}
}

// start starts a read for entry unless one is in flight, and returns the
// channel closed when it completes. The caller holds the lock.
func (c *tenantCache) start(entry *tenantEntry) chan struct{} {
	if entry.loading == nil {
		entry.loading = make(chan struct{})
		c.loads.Add(1)
		go c.load(entry)
	}
	return entry.loading
}

// load reads entry's namespace on the cache's root context and applies the
// result.
func (c *tenantCache) load(entry *tenantEntry) {
	defer c.loads.Done()
	ctx, cancel := context.WithTimeout(c.root, c.config.ReadTimeout)
	resources, err := c.reader.Read(ctx, entry.namespace)
	cancel()
	now := c.host.clock()
	if err != nil {
		if c.root.Err() == nil {
			c.host.recordReadFailure(entry.namespace, err)
		}
		c.mu.Lock()
		entry.failed, entry.retryAt = true, now.Add(c.config.FailureBackoff)
		if entry.snapshot == nil {
			entry.snapshot = c.host.failed
		}
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	c.host.reads.WithLabelValues(readSuccess).Inc()
	digest := digestOf(resources)
	c.mu.Lock()
	entry.failed, entry.loadedAt, entry.retryAt = false, now, time.Time{}
	unknown := c.unknownTypes(entry, resources)
	unchanged := entry.snapshot != nil && entry.snapshot.read && entry.snapshot.digest == digest
	if unchanged || c.closed {
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	for _, name := range unknown {
		c.host.unknownTypes.Inc()
		c.host.logger.Warn("gateway tenant configuration has an unknown extension type", "namespace", entry.namespace, "type", name)
	}
	built := c.host.buildSnapshot(entry.namespace, resources, digest)
	c.mu.Lock()
	unused := entry.evicted || c.closed
	var previous *snapshot
	if !unused {
		previous = entry.snapshot
		entry.snapshot = built
	}
	c.finish(entry)
	c.mu.Unlock()
	if unused {
		c.host.retire(built)
	} else if previous != nil && previous != built {
		c.host.retire(previous)
	}
}

// unknownTypes returns the enabled resources' unknown types that entry has
// not logged yet, and marks them. The caller holds the lock.
func (c *tenantCache) unknownTypes(entry *tenantEntry, resources []TenantResource) []string {
	var fresh []string
	for _, resource := range resources {
		if resource.Enabled == nil || !*resource.Enabled || entry.unknown[resource.Name] || c.host.tenantDefinition(resource.Name) {
			continue
		}
		entry.unknown[resource.Name] = true
		fresh = append(fresh, resource.Name)
	}
	return fresh
}

// finish ends entry's read. The caller holds the lock.
func (c *tenantCache) finish(entry *tenantEntry) {
	close(entry.loading)
	entry.loading = nil
}

// close stops reads, retires every entry's snapshot and waits, until ctx
// ends, for reads in flight, which build nothing once the cache is closed.
func (c *tenantCache) close(ctx context.Context) {
	c.cancel()
	c.mu.Lock()
	c.closed = true
	for element := c.order.Front(); element != nil; {
		next := element.Next()
		c.evict(element)
		element = next
	}
	c.mu.Unlock()
	waitUntil(ctx, &c.loads)
}
