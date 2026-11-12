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
}

// Host owns the constructed extension instances and composes their pipeline
// around each call. It is safe for concurrent use.
type Host struct {
	instances    []*instance
	lists        map[extension.KeyOwnership]*lists
	logger       *slog.Logger
	clock        func() time.Time
	requestInfo  func(context.Context) extension.Call
	drainTimeout time.Duration
	events       *prometheus.CounterVec
	dropped      prometheus.Counter
	observations observations
	closeOnce    sync.Once
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

// lists are the instances that apply to calls with one key ownership, by
// name, and the positions of the transformers among them in transform order.
type lists struct {
	byName            []*instance
	transforms        []int
	responseInspector bool
	observer          bool
}

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
	}
	for _, collector := range []prometheus.Collector{host.events, host.dropped} {
		if err := config.Registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("extensions: registering metrics: %w", err)
		}
	}
	for _, planned := range config.Plan.enabled {
		built, err := construct(planned, config)
		if built != nil {
			host.instances = append(host.instances, built)
		}
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), observeTimeout)
			host.Close(ctx)
			cancel()
			return nil, err
		}
	}
	slices.SortFunc(host.instances, func(a, b *instance) int { return strings.Compare(a.definition.Name, b.definition.Name) })
	host.lists = map[extension.KeyOwnership]*lists{
		extension.KeysOperator: host.listsFor(extension.KeysOperator),
		extension.KeysCustomer: host.listsFor(extension.KeysCustomer),
	}
	return host, nil
}

func construct(planned planned, config Config) (*instance, error) {
	definition := planned.definition
	deps := extension.Dependencies{
		Logger:     config.Logger.With("extension", definition.Name),
		Registerer: prometheus.WrapRegistererWith(prometheus.Labels{"extension": definition.Name}, config.Registerer),
		Clock:      config.Clock,
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

func (h *Host) listsFor(keys extension.KeyOwnership) *lists {
	selected := &lists{}
	for _, built := range h.instances {
		if !applies(built.definition, keys) {
			continue
		}
		selected.byName = append(selected.byName, built)
		selected.responseInspector = selected.responseInspector || built.responseInspector != nil
		selected.observer = selected.observer || built.observer != nil
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
// selection's model in the pipeline of the extensions that apply to the key
// ownership of its source, or returns the model unchanged when none applies.
func (h *Host) Compose(selected execution.Selection) (provider.LanguageModel, error) {
	applicable := h.lists[selected.Source.Keys()]
	if len(applicable.byName) == 0 {
		return selected.Model, nil
	}
	return &pipeline{LanguageModel: selected.Model, host: h, lists: applicable, source: selected.Source, requested: selected.RequestedID(), canonical: selected.CanonicalID(), candidates: candidates(selected.Expected()), attempts: selected.Attempts}, nil
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

// Close stops accepting observations, waits for those in flight and closes
// every instance concurrently, until ctx ends. Observations still running when
// ctx ends are counted as dropped; close failures, and closes still running
// when ctx ends, are counted and logged as failures. Later calls have no
// effect.
func (h *Host) Close(ctx context.Context) {
	h.closeOnce.Do(func() { h.close(ctx) })
}

func (h *Host) close(ctx context.Context) {
	h.dropped.Add(float64(h.observations.close(ctx)))
	type closing struct {
		definition extension.Definition
		running    atomic.Bool
	}
	var started []*closing
	var group sync.WaitGroup
	for _, built := range h.instances {
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
