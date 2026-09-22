package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type telemetryStateKey struct{}

type telemetryState struct {
	authMu      sync.Mutex
	authSource  string
	authOutcome atomic.Uint32
	observation atomic.Pointer[requestObservation]
}

// TelemetryOptions contains trusted static observation values.
type TelemetryOptions struct {
	Region      string
	Application string
}

// Telemetry owns bounded HTTP lifecycle metrics and completion logging.
type Telemetry struct {
	logger                *slog.Logger
	registry              *prometheus.Registry
	ready                 prometheus.Gauge
	inFlight              *prometheus.GaugeVec
	requests              *prometheus.CounterVec
	duration              *prometheus.HistogramVec
	authentication        *prometheus.CounterVec
	agentExportFailures   *prometheus.CounterVec
	physicalDrops         *prometheus.CounterVec
	staticObservation     requestObservation
	generateCorrelationID func() string
}

// NewTelemetry constructs one service-owned Prometheus registry.
func NewTelemetry(logger *slog.Logger, options ...TelemetryOptions) (*Telemetry, error) {
	return newTelemetry(logger, prometheus.NewRegistry(), options...)
}

func newTelemetry(logger *slog.Logger, registry *prometheus.Registry, options ...TelemetryOptions) (*Telemetry, error) {
	var configured TelemetryOptions
	if len(options) != 0 {
		configured = options[0]
	}
	telemetry := &Telemetry{
		logger:   logger,
		registry: registry,
		staticObservation: requestObservation{
			region:      boundedObservationValue(configured.Region),
			application: boundedObservationValue(configured.Application),
		},
		generateCorrelationID: newRequestCorrelationID,
		ready: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "ready",
			Help:      "Whether the gateway is ready to receive protected requests.",
		}),
		inFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "http_requests_in_flight",
			Help:      "Current in-flight HTTP requests.",
		}, []string{"route", "method"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "http_requests_total",
			Help:      "Completed HTTP requests.",
		}, []string{"route", "method", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
		}, []string{"route", "method", "status"}),
		authentication: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "authentication_total",
			Help:      "Authentication attempts by source and outcome.",
		}, []string{"source", "outcome"}),
		agentExportFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "agento11y_export_failures_total",
			Help:      "Agent Observability export failures by fixed class.",
		}, []string{"class"}),
		physicalDrops: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway", Name: "physical_attempt_dropped_total",
			Help: "Dropped private physical attempt records by fixed class.",
		}, []string{"class"}),
	}
	for _, collector := range []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		telemetry.ready,
		telemetry.inFlight,
		telemetry.requests,
		telemetry.duration,
		telemetry.authentication,
		telemetry.agentExportFailures,
		telemetry.physicalDrops,
	} {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("gateway service: registering metrics: %w", err)
		}
	}
	return telemetry, nil
}

func (telemetry *Telemetry) observePhysicalDrop(class physicalDropClass) {
	switch class {
	case physicalDropQueue, physicalDropProjection, physicalDropTransport, physicalDropShutdown, physicalDropWorker:
	default:
		class = physicalDropWorker
	}
	telemetry.physicalDrops.WithLabelValues(string(class)).Inc()
}

type agentExportFailureClass string

const (
	agentExportFailureQueue         agentExportFailureClass = "queue_full"
	agentExportFailureSerialization agentExportFailureClass = "serialization"
	agentExportFailureTransport     agentExportFailureClass = "transport"
	agentExportFailureRejected      agentExportFailureClass = "rejected"
	agentExportFailureShutdown      agentExportFailureClass = "shutdown"
	agentExportFailureUnknown       agentExportFailureClass = "unknown"
)

// observeAgentExportFailure records only a fixed class; raw exporter errors,
// endpoints, credentials, and payloads never enter the metric or diagnostic.
func (telemetry *Telemetry) observeAgentExportFailure(class agentExportFailureClass) {
	switch class {
	case agentExportFailureQueue, agentExportFailureSerialization, agentExportFailureTransport, agentExportFailureRejected, agentExportFailureShutdown:
	default:
		class = agentExportFailureUnknown
	}
	telemetry.agentExportFailures.WithLabelValues(string(class)).Inc()
	telemetry.logger.Warn("agent observability export failed", "class", string(class))
}

// Handler exposes the service-owned registry.
func (telemetry *Telemetry) Handler() http.Handler {
	return promhttp.HandlerFor(telemetry.registry, promhttp.HandlerOpts{})
}

// Registerer exposes only collector registration on the service-owned registry.
func (telemetry *Telemetry) Registerer() prometheus.Registerer {
	return telemetry.registry
}

// SetReady updates the readiness collector.
func (telemetry *Telemetry) SetReady(ready bool) {
	if ready {
		telemetry.ready.Set(1)
		return
	}
	telemetry.ready.Set(0)
}

// ObserveAuthentication records the closed outcome and normalized caller without retaining token-bearing authlib state.
func (telemetry *Telemetry) ObserveAuthentication(ctx context.Context, observation gatewayauth.Observation) {
	source := authenticationSourceClass(observation.Source)
	outcome := authenticationClass(observation.Outcome)
	telemetry.authentication.WithLabelValues(source, outcome).Inc()
	if state, ok := ctx.Value(telemetryStateKey{}).(*telemetryState); ok {
		state.authMu.Lock()
		state.authSource = source
		state.authMu.Unlock()
		state.authOutcome.Store(uint32(observation.Outcome))
		if observation.Outcome == gatewayauth.OutcomeAuthenticated && observation.Source != gatewayauth.SourceCloudGateway && observation.Caller != nil {
			current := observationFromContext(ctx)
			current.callerService = boundedObservationValue(observation.Caller.Service)
			current.namespace = boundedObservationValue(observation.Caller.Namespace)
			state.observation.Store(&current)
		}
	}
}

// Middleware records exactly one bounded completion observation per request.
func (telemetry *Telemetry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		route := normalizeRequestRoute(request)
		method := normalizeMethod(request.Method)
		observation := telemetry.staticObservation
		observation.correlationID = boundedObservationValue(telemetry.generateCorrelationID())
		state := &telemetryState{authSource: authenticationSourceClass("")}
		state.observation.Store(&observation)
		request = request.WithContext(context.WithValue(request.Context(), telemetryStateKey{}, state))
		wrapped := &responseWriter{ResponseWriter: w}
		started := time.Now()
		telemetry.inFlight.WithLabelValues(route, method).Inc()
		defer func() {
			telemetry.inFlight.WithLabelValues(route, method).Dec()
			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}
			statusClass := normalizeStatus(status)
			duration := time.Since(started).Seconds()
			telemetry.requests.WithLabelValues(route, method, statusClass).Inc()
			telemetry.duration.WithLabelValues(route, method, statusClass).Observe(duration)
			state.authMu.Lock()
			source := state.authSource
			state.authMu.Unlock()
			attrs := []any{
				"route", route,
				"method", method,
				"status", statusClass,
				"authentication", authenticationClass(gatewayauth.Outcome(state.authOutcome.Load())),
				"authentication_source", source,
			}
			for _, attr := range observationLogAttrs(request.Context()) {
				attrs = append(attrs, attr)
			}
			telemetry.logger.InfoContext(request.Context(), "http request completed", attrs...)
		}()
		next.ServeHTTP(wrapped, request)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (writer *responseWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseWriter) Write(value []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(value)
}

func (writer *responseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func authenticationSourceClass(source gatewayauth.Source) string {
	switch source {
	case gatewayauth.SourceAccessToken:
		return "access-token"
	case gatewayauth.SourceCloudGateway:
		return "cloud-gateway"
	default:
		return "unknown"
	}
}

func authenticationClass(outcome gatewayauth.Outcome) string {
	switch outcome {
	case gatewayauth.OutcomeAuthenticated:
		return "authenticated"
	case gatewayauth.OutcomeFailed:
		return "authentication_failed"
	default:
		return "not_attempted"
	}
}

func normalizeRequestRoute(request *http.Request) string {
	if request.URL.RawPath != "" {
		return "unmatched"
	}
	return normalizeRoute(request.URL.Path)
}

func normalizeRoute(path string) string {
	switch path {
	case "/live":
		return "live"
	case "/ready":
		return "ready"
	case "/metrics":
		return "metrics"
	case "/api/v1/aisdk/config":
		return "config"
	case "/api/v1/aisdk/language-model":
		return "language_model"
	default:
		return "unmatched"
	}
}

func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet:
		return "GET"
	case http.MethodPost:
		return "POST"
	default:
		return "other"
	}
}

func normalizeStatus(status int) string {
	class := status / 100
	if class < 1 || class > 5 {
		class = 5
	}
	return strconv.Itoa(class) + "xx"
}
