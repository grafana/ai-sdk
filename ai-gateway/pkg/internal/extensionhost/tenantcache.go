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

// tenantCache caches each namespace's configuration, read as the
// plugin on its own root context. A namespace's first call waits for its
// first read; concurrent calls share one read, and later calls are served
// from the cache while a background refresh runs after the TTL. A failed
// read is retried after the backoff; the last successful read is served for
// at most the maximum staleness, then the failure configuration.
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
	// config is the namespace's configuration served to calls; nil before
	// the first read completes.
	config *namespaceConfig
	// loadedAt is the time of the last successful read; zero before one.
	loadedAt time.Time
	// failed reports whether the last read failed.
	failed  bool
	retryAt time.Time
	// loading is closed when the read in flight completes; nil without one.
	loading chan struct{}
	evicted bool
	// unimplemented holds the types this build does not implement that are
	// already logged and counted.
	unimplemented map[string]bool
}

func newTenantCache(host *Host, reader TenantReader) *tenantCache {
	root, cancel := context.WithCancel(context.Background())
	return &tenantCache{host: host, reader: reader, config: host.cacheConfig, root: root, cancel: cancel, entries: map[string]*list.Element{}, order: list.New()}
}

// acquire returns namespace's configuration. It waits for the namespace's
// first read, or until ctx ends. A call without a namespace never reads and
// gets the failure configuration.
func (c *tenantCache) acquire(ctx context.Context, namespace string) (*namespaceConfig, error) {
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
		if entry.config == nil {
			loading := c.start(entry)
			c.mu.Unlock()
			select {
			case <-loading:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.mu.Lock()
			if entry.evicted || entry.config == nil {
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
		served := entry.config
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
	entry := &tenantEntry{namespace: namespace, unimplemented: map[string]bool{}}
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

// evict removes an entry. The caller holds the lock.
func (c *tenantCache) evict(element *list.Element) {
	victim := element.Value.(*tenantEntry)
	c.order.Remove(element)
	delete(c.entries, victim.namespace)
	victim.evicted = true
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
// configuration once reads have kept failing past the maximum staleness since it
// was read. An entry whose last read succeeded is never expired: its next
// refresh replaces it. The caller holds the lock.
func (c *tenantCache) expire(entry *tenantEntry, now time.Time) {
	if !entry.failed || entry.config == c.host.failed || now.Before(entry.loadedAt.Add(c.config.MaxStaleness)) {
		return
	}
	c.host.logger.Warn("gateway tenant configuration expired", "namespace", entry.namespace)
	entry.config = c.host.failed
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
		if entry.config == nil {
			entry.config = c.host.failed
		}
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	c.host.reads.WithLabelValues(readSuccess).Inc()
	digest := digestOf(resources)
	c.mu.Lock()
	entry.failed, entry.loadedAt, entry.retryAt = false, now, time.Time{}
	unchanged := entry.config != nil && entry.config != c.host.failed && entry.config.digest == digest
	if unchanged || c.closed {
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	unimplemented := c.unimplementedTypes(entry, resources)
	c.mu.Unlock()
	for _, name := range unimplemented {
		c.host.unimplementedTypes.Inc()
		c.host.logger.Warn("gateway tenant configuration has a type this build does not implement", "namespace", entry.namespace, "type", name)
	}
	built := c.host.buildNamespace(entry.namespace, resources, digest)
	c.mu.Lock()
	if !entry.evicted && !c.closed {
		entry.config = built
	}
	c.finish(entry)
	c.mu.Unlock()
}

// unimplementedTypes returns the enabled resources' types this build does not
// implement that entry has not logged yet, and marks them. The caller holds the lock.
func (c *tenantCache) unimplementedTypes(entry *tenantEntry, resources []TenantResource) []string {
	var fresh []string
	for _, resource := range resources {
		if resource.Invalid || !resource.Enabled || entry.unimplemented[resource.Name] || c.host.tenantDefinition(resource.Name) {
			continue
		}
		entry.unimplemented[resource.Name] = true
		fresh = append(fresh, resource.Name)
	}
	return fresh
}

// finish ends entry's read. The caller holds the lock.
func (c *tenantCache) finish(entry *tenantEntry) {
	close(entry.loading)
	entry.loading = nil
}

// close stops reads, drops every entry and waits, until ctx ends, for reads
// in flight, which apply nothing once the cache is closed.
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
