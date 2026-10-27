package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
)

type telemetryStateKey struct{}

type telemetryState struct {
	// access is written and read on the request's handler goroutine.
	access      gatewayauth.AccountAccess
	observation atomic.Pointer[requestObservation]
}

// TelemetryOptions contains the registerer for Gateway collectors. Without a
// registerer, collectors use a private registry.
type TelemetryOptions struct {
	Registerer prometheus.Registerer
}

// Telemetry owns the Gateway's access and export metrics and request
// completion logging. Route request metrics come from the plugin host.
type Telemetry struct {
	logger                *slog.Logger
	registerer            prometheus.Registerer
	accessDecisions       *prometheus.CounterVec
	agentExportFailures   *prometheus.CounterVec
	physicalDrops         *prometheus.CounterVec
	generateCorrelationID func() string
}

// NewTelemetry registers the Gateway collectors exactly once.
func NewTelemetry(logger *slog.Logger, options ...TelemetryOptions) (*Telemetry, error) {
	var configured TelemetryOptions
	if len(options) != 0 {
		configured = options[0]
	}
	registerer := configured.Registerer
	if registerer == nil {
		registerer = prometheus.NewRegistry()
	}
	telemetry := &Telemetry{
		logger:                logger,
		registerer:            registerer,
		generateCorrelationID: newRequestCorrelationID,
		accessDecisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "grafana_ai_gateway",
			Name:      "access_decisions_total",
			Help:      "Route requests by account access decision.",
		}, []string{"decision"}),
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
		telemetry.accessDecisions,
		telemetry.agentExportFailures,
		telemetry.physicalDrops,
	} {
		if err := registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("gateway service: registering metrics: %w", err)
		}
	}
	return telemetry, nil
}

func (telemetry *Telemetry) observePhysicalDrop(class physicalDropClass) {
	switch class {
	case physicalDropQueue, physicalDropProjection, physicalDropShutdown, physicalDropWorker:
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

// Registerer exposes only collector registration on the Gateway registerer.
func (telemetry *Telemetry) Registerer() prometheus.Registerer {
	return telemetry.registerer
}

// ObserveAccess records the closed access decision and retains the verified
// caller's attributable values privately for request observations.
func (telemetry *Telemetry) ObserveAccess(ctx context.Context, caller gatewayauth.Caller) {
	access := caller.AccountAccess()
	telemetry.accessDecisions.WithLabelValues(string(access)).Inc()
	state, ok := ctx.Value(telemetryStateKey{}).(*telemetryState)
	if !ok {
		return
	}
	state.access = access
	if access != gatewayauth.Refused {
		current := observationFromContext(ctx)
		current.callerService = boundedObservationValue(caller.AttributedService())
		current.namespace = boundedObservationValue(caller.Namespace)
		state.observation.Store(&current)
	}
}

// Middleware records exactly one bounded completion observation per request,
// labeled with the fixed route name from route.
func (telemetry *Telemetry) Middleware(route func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		routeName := route(request)
		method := normalizeMethod(request.Method)
		var observation requestObservation
		observation.correlationID = boundedObservationValue(telemetry.generateCorrelationID())
		state := &telemetryState{}
		state.observation.Store(&observation)
		request = request.WithContext(context.WithValue(request.Context(), telemetryStateKey{}, state))
		wrapped := &responseWriter{ResponseWriter: w}
		defer func() {
			status := wrapped.status
			if status == 0 {
				status = http.StatusOK
			}
			statusClass := normalizeStatus(status)
			access := "not_attempted"
			if state.access != "" {
				access = string(state.access)
			}
			attrs := []any{
				"route", routeName,
				"method", method,
				"status", statusClass,
				"access", access,
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
