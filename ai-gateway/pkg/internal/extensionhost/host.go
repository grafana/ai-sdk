package extensionhost

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
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
// around each call. Every extension, operator or tenant, has one instance per
// process; a namespace's tenant configuration reaches its calls as data. It
// is safe for concurrent use.
type Host struct {
	// startup holds every instance, operator and tenant, by name.
	startup []*instance
	// operators holds the operator-scope instances among startup.
	operators []*instance
	// tenantInstances holds each tenant-scope instance by name.
	tenantInstances map[string]*instance
	// defaults holds the decoded default configuration of each tenant
	// extension enabled by default, by name.
	defaults map[string]any
	// tenant holds every tenant-scope definition by name.
	tenant []extension.Definition
	// tenantKeys reports, per key ownership, whether a tenant definition
	// applies.
	tenantKeys map[extension.KeyOwnership]bool
	// lists are the default lists per key ownership, used without namespace
	// configuration: operator extensions and tenant extensions enabled by
	// default, with their default configuration.
	lists map[extension.KeyOwnership]*lists
	// failed is the configuration of a namespace that cannot be read: every
	// tenant extension applies its failure policy.
	failed             *namespaceConfig
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
	closed             atomic.Bool
	cacheConfig        TenantCacheConfig
	cache              atomic.Pointer[tenantCache]
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

// member is an instance that runs for a call, with its configuration for
// the call: nil for operator extensions.
type member struct {
	instance *instance
	config   any
}

// lists are the instances that apply to calls with one key ownership, by
// name, with their configuration, the positions of the transformers among
// them in transform order (tenant scope first, then operator scope, each by
// name), and the fail-closed tenant extensions whose configuration failed.
type lists struct {
	byName            []*instance
	configs           []any
	transforms        []int
	responseInspector bool
	observer          bool
	unavailable       []extension.Definition
}

// keyOwnerships are the key ownerships the host keeps lists for.
var keyOwnerships = []extension.KeyOwnership{extension.KeysOperator, extension.KeysCustomer}

// empty reports whether the lists leave a call to the selected model alone.
func (l *lists) empty() bool { return len(l.byName) == 0 && len(l.unavailable) == 0 }

// New registers the extension metrics, constructs one instance of every
// planned extension, and decodes the empty configuration of every tenant
// extension enabled by default. A factory or default decoding that fails or
// panics, or an instance without a phase, fails construction without the
// error text; instances already built are closed.
func New(config Config) (*Host, error) {
	host := &Host{
		logger:          config.Logger,
		clock:           config.Clock,
		requestInfo:     config.RequestInfo,
		drainTimeout:    config.DrainTimeout,
		tenant:          config.Plan.tenant,
		cacheConfig:     config.TenantCache.withDefaults(),
		tenantInstances: map[string]*instance{},
		defaults:        map[string]any{},
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
	fail := func(err error) (*Host, error) {
		ctx, cancel := context.WithTimeout(context.Background(), observeTimeout)
		defer cancel()
		host.Close(ctx)
		return nil, err
	}
	for _, planned := range config.Plan.enabled {
		built, err := construct(planned, config.Logger, config.Registerer, config.Clock)
		if built != nil {
			host.startup = append(host.startup, built)
		}
		if err != nil {
			return fail(err)
		}
		if planned.definition.Scope == extension.ScopeOperator {
			host.operators = append(host.operators, built)
			continue
		}
		host.tenantInstances[planned.definition.Name] = built
		if !planned.definition.EnabledByDefault {
			continue
		}
		decoded, _, err := decode(planned.definition, emptyConfig)
		if err != nil {
			return fail(fmt.Errorf("extensions: decoding the default configuration of extension %q failed", planned.definition.Name))
		}
		host.defaults[planned.definition.Name] = decoded
	}
	sortByName(host.startup)
	sortByName(host.operators)
	host.lists = host.buildNamespace("", nil, [sha256.Size]byte{}).lists
	host.tenantKeys = make(map[extension.KeyOwnership]bool, len(keyOwnerships))
	for _, keys := range keyOwnerships {
		host.tenantKeys[keys] = slices.ContainsFunc(host.tenant, func(definition extension.Definition) bool { return applies(definition, keys) })
	}
	host.failed = host.failureConfig()
	return host, nil
}

func sortByName(instances []*instance) {
	slices.SortFunc(instances, func(a, b *instance) int { return strings.Compare(a.definition.Name, b.definition.Name) })
}

// operatorMembers returns the operator instances as members without
// configuration.
func (h *Host) operatorMembers() []member {
	members := make([]member, len(h.operators))
	for i, built := range h.operators {
		members[i] = member{instance: built}
	}
	return members
}

// construct runs a definition's factory. It returns the instance when the
// factory returned one without a phase, so the caller can close it.
func construct(planned planned, logger *slog.Logger, registerer prometheus.Registerer, clock func() time.Time) (*instance, error) {
	definition := planned.definition
	deps := extension.Dependencies{
		Logger:     logger.With("extension", definition.Name),
		Registerer: prometheus.WrapRegistererWith(prometheus.Labels{"extension": definition.Name}, registerer),
		Clock:      clock,
		Secrets:    planned.secrets,
	}
	var value any
	err := contain(func() (err error) {
		value, err = definition.New(planned.config, deps)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("extensions: constructing extension %q failed", definition.Name)
	}
	built := &instance{definition: definition}
	built.admitter, _ = value.(extension.Admitter)
	built.transformer, _ = value.(extension.RequestTransformer)
	built.requestInspector, _ = value.(extension.RequestInspector)
	built.responseInspector, _ = value.(extension.ResponseInspector)
	built.observer, _ = value.(extension.Observer)
	built.closer, _ = value.(extension.Closer)
	if built.admitter == nil && built.transformer == nil && built.requestInspector == nil && built.responseInspector == nil && built.observer == nil {
		return built, fmt.Errorf("extensions: extension %q implements no phase", definition.Name)
	}
	return built, nil
}

// decode returns the value a tenant extension's calls receive for a
// configuration its schema accepts: nil for a type without a schema, the
// configuration itself without Decode, or what Decode returns. A failure
// returns its class.
func decode(definition extension.Definition, configuration json.RawMessage) (any, failureClass, error) {
	if definition.ConfigSchema.JSON() == nil {
		return nil, "", nil
	}
	if definition.Decode == nil {
		return slices.Clone(configuration), "", nil
	}
	var decoded any
	err := contain(func() (err error) {
		decoded, err = definition.Decode(configuration)
		return err
	})
	if err != nil {
		return nil, classOf(err), err
	}
	return decoded, "", nil
}

// newLists returns the lists of the members, which are in name order, that
// apply to calls whose keys are owned by keys, with the unavailable
// definitions that apply to them.
func newLists(members []member, keys extension.KeyOwnership, unavailable []extension.Definition) *lists {
	selected := &lists{}
	for _, running := range members {
		built := running.instance
		if !applies(built.definition, keys) {
			continue
		}
		selected.byName = append(selected.byName, built)
		selected.configs = append(selected.configs, running.config)
		selected.responseInspector = selected.responseInspector || built.responseInspector != nil
		selected.observer = selected.observer || built.observer != nil
	}
	for _, definition := range unavailable {
		if applies(definition, keys) {
			selected.unavailable = append(selected.unavailable, definition)
		}
	}
	for i, built := range selected.byName {
		if built.transformer != nil {
			selected.transforms = append(selected.transforms, i)
		}
	}
	slices.SortStableFunc(selected.transforms, func(a, b int) int {
		return cmp.Compare(scopeOrder(selected.byName[a].definition.Scope), scopeOrder(selected.byName[b].definition.Scope))
	})
	return selected
}

// scopeOrder orders transforms: tenant scope before operator scope. The list is
// already in name order, and the sort is stable.
func scopeOrder(scope extension.Scope) int {
	if scope == extension.ScopeTenant {
		return 0
	}
	return 1
}

// Compose is the extension layer of the runtime's model composer: it wraps a
// selection's model in the extension pipeline, or returns the model
// unchanged when no extension can apply to the key ownership of its source.
// Which extensions run, and with which configuration, is decided per call
// from the call's namespace configuration.
func (h *Host) Compose(selected execution.Selection) (provider.LanguageModel, error) {
	keys := selected.Source.Keys()
	applicable := h.lists[keys]
	if applicable.empty() && !h.tenantKeys[keys] {
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
		return
	}
	if h.closed.Load() {
		cache.close(context.Background())
	}
}

// resolve returns the lists for a call in namespace whose keys are owned by
// keys. They are immutable, so the call keeps using them after the
// namespace's configuration changes.
func (h *Host) resolve(ctx context.Context, namespace string, keys extension.KeyOwnership) (*lists, error) {
	cache := h.cache.Load()
	if cache == nil || !h.tenantKeys[keys] {
		return h.lists[keys], nil
	}
	configured, err := cache.acquire(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return configured.lists[keys], nil
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
// for reads and observations in flight, and closes every instance
// concurrently, until ctx ends. Observations still running when ctx ends are
// counted as dropped; close failures, and closes still running when ctx ends,
// are counted and logged as failures. Later calls have no effect.
func (h *Host) Close(ctx context.Context) {
	h.closeOnce.Do(func() { h.close(ctx) })
}

func (h *Host) close(ctx context.Context) {
	h.closed.Store(true)
	if cache := h.cache.Load(); cache != nil {
		cache.close(ctx)
	}
	h.dropped.Add(float64(h.observations.close(ctx)))
	h.closeInstances(ctx, h.startup)
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
