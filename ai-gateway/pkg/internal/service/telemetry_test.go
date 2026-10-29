package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
)

func TestTelemetry_DuplicateRegistrationFails(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := NewTelemetry(logger, TelemetryOptions{Registerer: registry})
	require.NoError(t, err)
	_, err = NewTelemetry(logger, TelemetryOptions{Registerer: registry})
	require.Error(t, err)
}

func TestTelemetry_RegistersNoRuntimeCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)), TelemetryOptions{Registerer: registry})
	require.NoError(t, err)
	telemetry.ObserveAccess(context.Background(), verifiedCaller(gatewayauth.AccessModeCloud, nil, "", "", testNamespace))
	metrics := testMetrics(t, telemetry)
	for _, forbidden := range []string{"go_goroutines", "process_cpu_seconds", "grafana_ai_gateway_ready", "grafana_ai_gateway_authentication_total"} {
		assert.NotContains(t, metrics, forbidden)
	}
}

func TestTelemetry_AccessDecisionsUseFixedLabels(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	handler := telemetry.Middleware(fixedRoute("config"), http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		for _, caller := range []gatewayauth.Caller{
			verifiedCaller(gatewayauth.AccessModeCloud, []string{testInternalService}, testInternalService, "", testNamespace),
			verifiedCaller(gatewayauth.AccessModeCloud, nil, "front-door", "user:u1", testNamespace),
			verifiedCaller(gatewayauth.AccessModeCloud, nil, "", "", testNamespace),
			verifiedCaller(gatewayauth.AccessModeCloud, nil, "", "", ""),
		} {
			telemetry.ObserveAccess(request.Context(), caller)
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	metrics := testMetrics(t, telemetry)
	assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="configured"} 1`)
	assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="byok"} 1`)
	assert.Contains(t, metrics, `grafana_ai_gateway_access_decisions_total{decision="refused"} 2`)
}

func TestResponseWriter_UnwrapStatusAndFlush(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapped := &responseWriter{ResponseWriter: recorder}
	assert.Same(t, recorder, wrapped.Unwrap())
	_, err := wrapped.Write([]byte("frame"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, wrapped.status)
	require.NoError(t, http.NewResponseController(wrapped).Flush())
	assert.True(t, recorder.Flushed)
	wrapped.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusOK, wrapped.status)
}
