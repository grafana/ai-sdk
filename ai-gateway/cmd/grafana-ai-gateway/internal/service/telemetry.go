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
	authOutcome atomic.Uint32
	callerMu    sync.Mutex
	service     string
	namespace   string
}

// Telemetry owns bounded HTTP lifecycle metrics and completion logging.
type Telemetry struct {
	logger   *slog.Logger
	registry *prometheus.Registry
	ready    prometheus.Gauge
	inFlight *prometheus.GaugeVec
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewTelemetry constructs one service-owned Prometheus registry.
func NewTelemetry(logger *slog.Logger) (*Telemetry, error) {
	return newTelemetry(logger, prometheus.NewRegistry())
}

func newTelemetry(logger *slog.Logger, registry *prometheus.Registry) (*Telemetry, error) {
	if logger == nil {
		return nil, fmt.Errorf("gateway service: logger is nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("gateway service: prometheus registry is nil")
	}
	telemetry := &Telemetry{
		logger:   logger,
		registry: registry,
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
	}
	for _, collector := range []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		telemetry.ready,
		telemetry.inFlight,
		telemetry.requests,
		telemetry.duration,
	} {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("gateway service: registering metrics: %w", err)
		}
	}
	return telemetry, nil
}

// Handler exposes the service-owned registry.
func (telemetry *Telemetry) Handler() http.Handler {
	return promhttp.HandlerFor(telemetry.registry, promhttp.HandlerOpts{})
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
	if state, ok := ctx.Value(telemetryStateKey{}).(*telemetryState); ok {
		state.authOutcome.Store(uint32(observation.Outcome))
		if observation.Caller != nil {
			state.callerMu.Lock()
			state.service = observation.Caller.Service
			state.namespace = observation.Caller.Namespace
			state.callerMu.Unlock()
		}
	}
}

// Middleware records exactly one bounded completion observation per request.
func (telemetry *Telemetry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		route := normalizeRequestRoute(request)
		method := normalizeMethod(request.Method)
		state := &telemetryState{}
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
			telemetry.logger.InfoContext(request.Context(), "http request completed",
				"route", route,
				"method", method,
				"status", statusClass,
				"authentication", authenticationClass(gatewayauth.Outcome(state.authOutcome.Load())),
			)
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
