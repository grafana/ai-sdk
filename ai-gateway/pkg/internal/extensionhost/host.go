package extensionhost

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/provider"
)

// observeTimeout bounds every observation of one call.
const observeTimeout = time.Second

// phaseClose labels failures of [extension.Closer.Close], which is not a call
// phase.
const phaseClose extension.Phase = "close"

var errUnknownSource = errors.New("extensions: selection source has no extension configuration")

// Config configures a [Host].
type Config struct {
	Plan       *Plan
	Logger     *slog.Logger
	Registerer prometheus.Registerer
	Clock      func() time.Time
	// RequestInfo returns a call's namespace, caller type, caller identity and
	// correlation ID.
	RequestInfo func(context.Context) extension.Call
	// DrainTimeout bounds draining a model stream the pipeline stopped.
	DrainTimeout time.Duration
	// TenantCache overrides the tenant configuration cache's bounds. Zero
	// fields use the defaults.
	TenantCache TenantCacheConfig
}

// Host owns the constructed extension instances and composes their pipeline
// around each call. It is safe for concurrent use.
type Host struct {
	// startup holds the instances built at startup: enabled operator
	// extensions and tenant extensions enabled by default, by name.
	startup []*instance
	// operators holds the operator-scope instances among startup.
	operators []*instance
	// defaults holds each default-enabled tenant extension's startup
	// instance by name.
	defaults map[string]*instance
	// tenant holds every tenant-scope definition by name.
	tenant []extension.Definition
	// tenantSources reports, per source, whether a tenant definition applies.
	tenantSources map[extension.Source]bool
	// lists are the startup instances' lists, used without namespace
	// configuration.
	lists map[extension.Source]*lists
	// failed is the snapshot of a namespace whose configuration cannot be
	// read: every tenant extension applies its failure policy.
	failed             *snapshot
	logger             *slog.Logger
	clock              func() time.Time
	requestInfo        func(context.Context) extension.Call
	drainTimeout       time.Duration
	events             *prometheus.CounterVec
	dropped            prometheus.Counter
	reads              *prometheus.CounterVec
	unimplementedTypes prometheus.Counter
	observations       observations
	closeOnce          sync.Once
	// registerers holds each definition's shared registerer.
	registerers map[string]sharedRegisterer
	cacheConfig TenantCacheConfig
	cache       atomic.Pointer[tenantCache]

	// mu guards live and shuttingDown, and orders closing.Add before the
	// shutdown wait.
	mu sync.Mutex
	// live holds the snapshots whose instances are not closed yet.
	live         map[*snapshot]struct{}
	shuttingDown bool
	closing      sync.WaitGroup
}

type instance struct {
	definition        extension.Definition
	admitter          extension.Admitter
	transformer       extension.RequestTransformer
	requestInspector  extension.RequestInspector
	responseInspector extension.ResponseInspector
	observer          extension.Observer
	closer            extension.Closer
}

// lists are the instances that apply to calls from one source, by name, the
// positions of the transformers among them in transform order, and the
// fail-closed tenant extensions whose configuration failed.
type lists struct {
	byName            []*instance
	transforms        []int
	responseInspector bool
	observer          bool
	unavailable       []extension.Definition
}

// empty reports whether the lists leave a call to the selected model alone.
func (l *lists) empty() bool { return len(l.byName) == 0 && len(l.unavailable) == 0 }

// New registers the extension metrics and constructs every planned instance.
// A factory that fails or panics, or an instance without a phase, fails
// construction without the factory's error text; instances already built are
// closed.
func New(config Config) (*Host, error) {
	host := &Host{
		logger:       config.Logger,
		clock:        config.Clock,
		requestInfo:  config.RequestInfo,
		drainTimeout: config.DrainTimeout,
		tenant:       config.Plan.tenant,
		cacheConfig:  config.TenantCache.withDefaults(),
		live:         map[*snapshot]struct{}{},
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "extension_events_total",
			Help:      "Extension denials and failures by extension, phase and fixed event.",
		}, []string{"extension", "phase", "event"}),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "extension_observations_dropped_total",
			Help:      "Extension observations dropped because shutdown had begun.",
		}),
		reads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "tenant_config_reads_total",
			Help:      "Namespace tenant configuration reads by fixed result.",
		}, []string{"result"}),
		unimplementedTypes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "tenant_config_unimplemented_types_total",
			Help:      "Enabled tenant configuration resources whose type this build does not implement, once per namespace and type.",
		}),
	}
	for _, collector := range []prometheus.Collector{host.events, host.dropped, host.reads, host.unimplementedTypes} {
		if err := config.Registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("extensions: registering metrics: %w", err)
		}
	}
	host.registerers = make(map[string]sharedRegisterer, len(config.Plan.enabled)+len(host.tenant))
	for _, planned := range config.Plan.enabled {
		host.registerers[planned.definition.Name] = newSharedRegisterer(config.Registerer, planned.definition.Name)
	}
	for _, definition := range host.tenant {
		host.registerers[definition.Name] = newSharedRegisterer(config.Registerer, definition.Name)
	}
	host.defaults = map[string]*instance{}
	for _, planned := range config.Plan.enabled {
		built, _, err := construct(planned, config.Logger, host.registerers[planned.definition.Name], config.Clock)
		if built != nil {
			host.startup = append(host.startup, built)
		}
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), observeTimeout)
			host.Close(ctx)
			cancel()
			return nil, err
		}
		if planned.definition.Scope == extension.ScopeTenant {
			host.defaults[planned.definition.Name] = built
		} else {
			host.operators = append(host.operators, built)
		}
	}
	sortByName(host.startup)
	sortByName(host.operators)
	host.lists = make(map[extension.Source]*lists, len(extension.Sources))
	host.tenantSources = make(map[extension.Source]bool, len(extension.Sources))
	for _, source := range extension.Sources {
		host.lists[source] = newLists(host.startup, source, nil)
		host.tenantSources[source] = slices.ContainsFunc(host.tenant, func(definition extension.Definition) bool { return applies(definition, source) })
	}
	host.failed = host.failureSnapshot()
	return host, nil
}

func sortByName(instances []*instance) {
	slices.SortFunc(instances, func(a, b *instance) int { return strings.Compare(a.definition.Name, b.definition.Name) })
}

// construct runs a definition's factory. On failure it returns the failure's
// class, and the instance when the factory returned one without a phase, so
// the caller can close it.
func construct(planned planned, logger *slog.Logger, registerer sharedRegisterer, clock func() time.Time) (*instance, failureClass, error) {
	definition := planned.definition
	deps := extension.Dependencies{
		Logger:     logger.With("extension", definition.Name),
		Registerer: registerer,
		Clock:      clock,
		Secrets:    planned.secrets,
	}
	var value any
	err := contain(func() (err error) {
		value, err = definition.New(planned.config, deps)
		return err
	})
	if err != nil {
		return nil, classOf(err), fmt.Errorf("extensions: constructing extension %q failed", definition.Name)
	}
	built := &instance{definition: definition}
	built.admitter, _ = value.(extension.Admitter)
	built.transformer, _ = value.(extension.RequestTransformer)
	built.requestInspector, _ = value.(extension.RequestInspector)
	built.responseInspector, _ = value.(extension.ResponseInspector)
	built.observer, _ = value.(extension.Observer)
	built.closer, _ = value.(extension.Closer)
	if built.admitter == nil && built.transformer == nil && built.requestInspector == nil && built.responseInspector == nil && built.observer == nil {
		return built, classError, fmt.Errorf("extensions: extension %q implements no phase", definition.Name)
	}
	return built, "", nil
}

// newLists returns the lists of the instances, in name order, that apply to
// source, with the unavailable definitions that apply to it.
func newLists(instances []*instance, source extension.Source, unavailable []extension.Definition) *lists {
	selected := &lists{}
	for _, built := range instances {
		if !applies(built.definition, source) {
			continue
		}
		selected.byName = append(selected.byName, built)
		selected.responseInspector = selected.responseInspector || built.responseInspector != nil
		selected.observer = selected.observer || built.observer != nil
	}
	for _, definition := range unavailable {
		if applies(definition, source) {
			selected.unavailable = append(selected.unavailable, definition)
		}
	}
	for i, built := range selected.byName {
		if built.transformer != nil {
			selected.transforms = append(selected.transforms, i)
		}
	}
	slices.SortStableFunc(selected.transforms, func(a, b int) int {
		first, second := selected.byName[a].definition, selected.byName[b].definition
		if first.Scope != second.Scope {
			if first.Scope == extension.ScopeTenant {
				return -1
			}
			return 1
		}
		return cmp.Compare(first.Priority, second.Priority)
	})
	return selected
}

// Compose is the extension layer of the runtime's model composer: it wraps a
// selection's model in the extension pipeline, or returns the model
// unchanged when no extension can apply to its source. Which extensions run
// is decided per call, from the call's namespace configuration.
func (h *Host) Compose(selected execution.Selection) (provider.LanguageModel, error) {
	applicable, ok := h.lists[selected.Source]
	if !ok {
		return nil, errUnknownSource
	}
	if applicable.empty() && !h.tenantSources[selected.Source] {
		return selected.Model, nil
	}
	return &pipeline{LanguageModel: selected.Model, host: h, source: selected.Source, requested: selected.RequestedID(), canonical: selected.CanonicalID(), candidates: candidates(selected.Expected()), attempts: selected.Attempts}, nil
}

// ConnectTenants applies each namespace's stored configuration, read through
// reader, to tenant extensions. Without a call, tenant extensions keep their
// default state everywhere, as on a host without API access. Only the first
// call has an effect, and none when no tenant extension is registered.
func (h *Host) ConnectTenants(reader TenantReader) {
	if len(h.tenant) == 0 {
		return
	}
	cache := newTenantCache(h, reader)
	if !h.cache.CompareAndSwap(nil, cache) {
		cache.cancel()
	}
}

// resolve returns the lists for a call from source in namespace, and the
// snapshot the call holds until it releases it.
func (h *Host) resolve(ctx context.Context, namespace string, source extension.Source) (*lists, *snapshot, error) {
	cache := h.cache.Load()
	if cache == nil || !h.tenantSources[source] {
		return h.lists[source], nil, nil
	}
	held, err := cache.acquire(ctx, namespace)
	if err != nil {
		return nil, nil, err
	}
	return held.lists[source], held, nil
}

// candidates returns the extension view of a selection's expected attempts.
func candidates(expected []execution.ExpectedAttempt) []extension.Candidate {
	if len(expected) == 0 {
		return nil
	}
	planned := make([]extension.Candidate, len(expected))
	for i, attempt := range expected {
		planned[i] = extension.Candidate{
			Model:            attempt.ModelID,
			CanonicalModel:   attempt.CanonicalID,
			Provider:         attempt.Destination.Provider,
			ProviderInstance: attempt.Destination.ProviderInstance,
			ProviderModel:    attempt.Destination.ModelID,
		}
	}
	return planned
}

// Close stops reading tenant configuration and accepting observations, waits
// for observations in flight, and closes every instance concurrently, until
// ctx ends: startup instances, every namespace's instances, and closes
// already started for replaced or evicted configurations. Observations still
// running when ctx ends are counted as dropped; close failures, and closes
// still running when ctx ends, are counted and logged as failures. A
// namespace read whose factory outlives ctx closes what it built itself,
// bounded by its own close timeout. Later calls have no effect.
func (h *Host) Close(ctx context.Context) {
	h.closeOnce.Do(func() { h.close(ctx) })
}

func (h *Host) close(ctx context.Context) {
	if cache := h.cache.Load(); cache != nil {
		cache.close(ctx)
	}
	h.dropped.Add(float64(h.observations.close(ctx)))
	h.mu.Lock()
	h.shuttingDown = true
	instances := slices.Clone(h.startup)
	for held := range h.live {
		instances = append(instances, held.owned...)
	}
	clear(h.live)
	h.mu.Unlock()
	var group sync.WaitGroup
	group.Go(func() { h.closeInstances(ctx, instances) })
	group.Go(func() { waitUntil(ctx, &h.closing) })
	group.Wait()
}

// closeInstances closes instances concurrently until ctx ends, counting
// failures and closes still running then.
func (h *Host) closeInstances(ctx context.Context, instances []*instance) {
	type closing struct {
		definition extension.Definition
		running    atomic.Bool
	}
	var started []*closing
	var group sync.WaitGroup
	for _, built := range instances {
		if built.closer == nil {
			continue
		}
		current := &closing{definition: built.definition}
		current.running.Store(true)
		started = append(started, current)
		group.Go(func() {
			err := contain(func() error { return built.closer.Close(ctx) })
			if current.running.CompareAndSwap(true, false) && err != nil {
				h.recordFailure(built.definition, phaseClose, classOf(err), "")
			}
		})
	}
	waitUntil(ctx, &group)
	for _, current := range started {
		if current.running.CompareAndSwap(true, false) {
			h.recordFailure(current.definition, phaseClose, classTimeout, "")
		}
	}
}

type failureClass string

const (
	classError  failureClass = "error"
	classPanic  failureClass = "panic"
	classPolicy failureClass = "policy"
	// classTimeout is a close still running at the shutdown deadline.
	classTimeout failureClass = "timeout"
	// classInvalid is a stored tenant configuration the extension does not
	// accept.
	classInvalid failureClass = "invalid"
)

var (
	errPanicked      = errors.New("extensions: phase panicked")
	errPolicyRefused = errors.New("extensions: host policy refused transformed parameters")
)

// contain runs extension code, turning a panic into errPanicked.
func contain(run func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errPanicked
		}
	}()
	return run()
}

func classOf(err error) failureClass {
	switch {
	case errors.Is(err, errPanicked):
		return classPanic
	case errors.Is(err, errPolicyRefused):
		return classPolicy
	default:
		return classError
	}
}

const (
	eventDenied = "denied"
	eventFailed = "failed"
)

func (h *Host) recordDenial(definition extension.Definition, phase extension.Phase) {
	h.events.WithLabelValues(definition.Name, string(phase), eventDenied).Inc()
}

// recordFailure counts and logs one failure without its error text.
func (h *Host) recordFailure(definition extension.Definition, phase extension.Phase, class failureClass, correlationID string) {
	h.events.WithLabelValues(definition.Name, string(phase), eventFailed).Inc()
	attrs := []any{"extension", definition.Name, "phase", string(phase), "policy", string(definition.FailurePolicy), "class", string(class)}
	if correlationID != "" {
		attrs = append(attrs, "correlation_id", correlationID)
	}
	h.logger.Warn("gateway extension failed", attrs...)
}
