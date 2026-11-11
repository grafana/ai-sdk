package extensionhost

import (
	"context"
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
)

// closeTimeout bounds closing the instances of one replaced or evicted
// namespace configuration.
const closeTimeout = observeTimeout

// phaseConfigure labels failures of a stored tenant configuration, which is
// not a call phase.
const phaseConfigure extension.Phase = "configure"

// snapshot is one namespace configuration revision: the lists each source's
// calls use, and the instances built for it. Calls hold a reference while
// they use its instances; once it is retired, the last release closes them.
// A snapshot without its own instances needs no references.
type snapshot struct {
	lists map[extension.Source]*lists
	owned []*instance
	// read reports whether the snapshot comes from a successful read, rather
	// than from a read failure.
	read   bool
	digest [sha256.Size]byte

	mu      sync.Mutex
	refs    int
	retired bool
}

// acquire takes a reference. The cache calls it under its lock, so a
// snapshot it serves is never retired before the reference is taken.
func (s *snapshot) acquire() {
	if len(s.owned) == 0 {
		return
	}
	s.mu.Lock()
	s.refs++
	s.mu.Unlock()
}

// release drops a reference, closing the instances of a retired snapshot
// when it was the last.
func (h *Host) release(s *snapshot) {
	if len(s.owned) == 0 {
		return
	}
	s.mu.Lock()
	s.refs--
	done := s.refs == 0 && s.retired
	s.mu.Unlock()
	if done {
		h.closeSnapshot(s)
	}
}

// retire marks a snapshot that no cache entry serves anymore, closing its
// instances once no call holds it.
func (h *Host) retire(s *snapshot) {
	if len(s.owned) == 0 {
		return
	}
	s.mu.Lock()
	s.retired = true
	done := s.refs == 0
	s.mu.Unlock()
	if done {
		h.closeSnapshot(s)
	}
}

// closeSnapshot closes a snapshot's instances once, on its own goroutine
// bounded by closeTimeout. After shutdown has begun, Close owns every
// snapshot it collected; a snapshot built later is closed synchronously.
func (h *Host) closeSnapshot(s *snapshot) {
	h.mu.Lock()
	if _, live := h.live[s]; !live {
		h.mu.Unlock()
		return
	}
	delete(h.live, s)
	if h.shuttingDown {
		h.mu.Unlock()
		h.closeBounded(s.owned)
		return
	}
	h.closing.Add(1)
	h.mu.Unlock()
	go func() {
		defer h.closing.Done()
		h.closeBounded(s.owned)
	}()
}

func (h *Host) closeBounded(instances []*instance) {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	h.closeInstances(ctx, instances)
}

// track registers a snapshot's instances for closing.
func (h *Host) track(s *snapshot) {
	if len(s.owned) == 0 {
		return
	}
	h.mu.Lock()
	h.live[s] = struct{}{}
	h.mu.Unlock()
}

// buildSnapshot builds the snapshot of a namespace's resources. A tenant
// extension without a resource uses its default state; a disabled one is
// off; an enabled one with a configuration its schema accepts gets its own
// instance. Any other resource, and a factory that fails, apply the
// extension's failure policy, logged and counted once per build. Resources of
// unknown types are ignored here.
func (h *Host) buildSnapshot(namespace string, resources []TenantResource, digest [sha256.Size]byte) *snapshot {
	byName := make(map[string]TenantResource, len(resources))
	for _, resource := range resources {
		byName[resource.Name] = resource
	}
	instances := slices.Clone(h.operators)
	var owned []*instance
	var unavailable []extension.Definition
	failed := func(definition extension.Definition, class failureClass) {
		h.recordConfigurationFailure(definition, namespace, class)
		if definition.FailurePolicy == extension.FailClosed {
			unavailable = append(unavailable, definition)
		}
	}
	logger := h.logger.With("namespace", namespace)
	for _, definition := range h.tenant {
		resource, found := byName[definition.Name]
		switch {
		case !found:
			if fallback := h.defaults[definition.Name]; fallback != nil {
				instances = append(instances, fallback)
			}
			continue
		case resource.Enabled == nil || resource.Type != resource.Name:
			failed(definition, classInvalid)
			continue
		case !*resource.Enabled:
			continue
		case CheckConfig(definition, resource.Config) != nil:
			failed(definition, classInvalid)
			continue
		}
		built, class, err := construct(planned{definition: definition, config: resource.Config}, logger, h.registerers[definition.Name], h.clock)
		if err != nil {
			if built != nil {
				h.closeBounded([]*instance{built})
			}
			failed(definition, class)
			continue
		}
		owned = append(owned, built)
		instances = append(instances, built)
	}
	sortByName(instances)
	built := &snapshot{lists: make(map[extension.Source]*lists, len(extension.Sources)), owned: owned, read: true, digest: digest}
	for _, source := range extension.Sources {
		built.lists[source] = newLists(instances, source, unavailable)
	}
	h.track(built)
	return built
}

// failureSnapshot returns the snapshot of a namespace without a usable
// configuration: operator extensions run, and every tenant extension applies
// its failure policy. It is shared and has no instances of its own.
func (h *Host) failureSnapshot() *snapshot {
	var unavailable []extension.Definition
	for _, definition := range h.tenant {
		if definition.FailurePolicy == extension.FailClosed {
			unavailable = append(unavailable, definition)
		}
	}
	failed := &snapshot{lists: make(map[extension.Source]*lists, len(extension.Sources))}
	for _, source := range extension.Sources {
		failed.lists[source] = newLists(h.operators, source, unavailable)
	}
	return failed
}

// digestOf fingerprints a namespace's resources, so an unchanged read keeps
// its snapshot.
func digestOf(resources []TenantResource) [sha256.Size]byte {
	hash := sha256.New()
	for _, resource := range resources {
		enabled := byte('-')
		if resource.Enabled != nil && *resource.Enabled {
			enabled = '1'
		} else if resource.Enabled != nil {
			enabled = '0'
		}
		for _, part := range [][]byte{[]byte(resource.Name), []byte(resource.Type), {enabled}, resource.Config} {
			hash.Write(part)
			hash.Write([]byte{0})
		}
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	return digest
}

// recordConfigurationFailure counts and logs one tenant configuration
// failure without configuration values or error text.
func (h *Host) recordConfigurationFailure(definition extension.Definition, namespace string, class failureClass) {
	h.events.WithLabelValues(definition.Name, string(phaseConfigure), eventFailed).Inc()
	h.logger.Warn("gateway tenant extension configuration failed", "extension", definition.Name, "namespace", namespace, "policy", string(definition.FailurePolicy), "class", string(class))
}
