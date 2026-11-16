package extensionhost

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Tenant configuration cache defaults.
const (
	// defaultTenantTTL is how long a namespace's read is used before a call
	// reads it again in the background, after a success or a failure.
	defaultTenantTTL = time.Minute
	// defaultTenantReadTimeout bounds one namespace read.
	defaultTenantReadTimeout = 5 * time.Second
	// defaultTenantMaxNamespaces bounds the namespaces cached at once.
	defaultTenantMaxNamespaces = 1024
)

// errTenantsClosed is the outcome of a call that needs namespace
// configuration after shutdown has begun.
var errTenantsClosed = errors.New("extensions: tenant configuration is closed")

// TenantCacheConfig bounds the tenant configuration cache. Zero fields use
// the defaults.
type TenantCacheConfig struct {
	TTL           time.Duration
	ReadTimeout   time.Duration
	MaxNamespaces int
}

func (c TenantCacheConfig) withDefaults() TenantCacheConfig {
	if c.TTL <= 0 {
		c.TTL = defaultTenantTTL
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = defaultTenantReadTimeout
	}
	if c.MaxNamespaces <= 0 {
		c.MaxNamespaces = defaultTenantMaxNamespaces
	}
	return c
}

// tenantCache caches each namespace's configuration, read as the plugin on
// its own root context. A namespace's first call waits for its first read;
// concurrent calls share one read, and later calls are served from the cache
// while a background read runs once the TTL has passed since the last one. A
// failed read keeps the last configuration read successfully; a namespace
// never read successfully gets the failure configuration.
type tenantCache struct {
	host   *Host
	reader TenantReader
	config TenantCacheConfig
	root   context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	entries map[string]*tenantEntry
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
	// resources are the resources config was built from; nil until a read
	// succeeds.
	resources []TenantResource
	// readAt is the time of the last read, successful or not.
	readAt   time.Time
	lastUsed time.Time
	// loading is closed when the read in flight completes; nil without one.
	loading chan struct{}
	evicted bool
}

func newTenantCache(host *Host, reader TenantReader) *tenantCache {
	root, cancel := context.WithCancel(context.Background())
	return &tenantCache{host: host, reader: reader, config: host.cacheConfig, root: root, cancel: cancel, entries: map[string]*tenantEntry{}}
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
		now := c.host.clock()
		entry := c.touch(namespace, now)
		if entry.config == nil {
			loading := c.start(entry)
			c.mu.Unlock()
			select {
			case <-loading:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				return nil, errTenantsClosed
			}
			if entry.config == nil {
				c.mu.Unlock()
				continue
			}
		} else if entry.loading == nil && !now.Before(entry.readAt.Add(c.config.TTL)) {
			c.start(entry)
		}
		served := entry.config
		c.mu.Unlock()
		return served, nil
	}
}

// touch returns namespace's entry, marked used at now. Creating one when the
// cache is full first evicts least recently used entries without a read in
// flight until it has room; entries with reads in flight can exceed the
// bound until a later insert. The caller holds the lock.
func (c *tenantCache) touch(namespace string, now time.Time) *tenantEntry {
	entry, ok := c.entries[namespace]
	if !ok {
		for len(c.entries) >= c.config.MaxNamespaces {
			if !c.evictLeastRecentlyUsed() {
				break
			}
		}
		entry = &tenantEntry{namespace: namespace}
		c.entries[namespace] = entry
	}
	entry.lastUsed = now
	return entry
}

// evictLeastRecentlyUsed removes the least recently used entry without a
// read in flight, and reports whether there was one. The caller holds the
// lock.
func (c *tenantCache) evictLeastRecentlyUsed() bool {
	var victim *tenantEntry
	for _, entry := range c.entries {
		if entry.loading == nil && (victim == nil || entry.lastUsed.Before(victim.lastUsed)) {
			victim = entry
		}
	}
	if victim == nil {
		return false
	}
	delete(c.entries, victim.namespace)
	victim.evicted = true
	return true
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
		entry.readAt = now
		if entry.config == nil {
			entry.config = c.host.failed
		}
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	c.host.reads.WithLabelValues(readSuccess).Inc()
	if resources == nil {
		resources = []TenantResource{}
	}
	c.mu.Lock()
	entry.readAt = now
	unchanged := c.closed || entry.resources != nil && slices.EqualFunc(entry.resources, resources, sameResource)
	if unchanged {
		c.finish(entry)
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	built := c.host.buildNamespace(entry.namespace, resources)
	c.mu.Lock()
	if !entry.evicted && !c.closed {
		entry.config, entry.resources = built, resources
	}
	c.finish(entry)
	c.mu.Unlock()
}

// sameResource reports whether two reads of a resource configure it the
// same way.
func sameResource(a, b TenantResource) bool {
	return a.Name == b.Name && a.Type == b.Type && a.Enabled == b.Enabled && a.Invalid == b.Invalid && bytes.Equal(a.Config, b.Config)
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
	for namespace, entry := range c.entries {
		delete(c.entries, namespace)
		entry.evicted = true
	}
	c.mu.Unlock()
	waitUntil(ctx, &c.loads)
}
