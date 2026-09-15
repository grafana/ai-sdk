package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestObservation_AuthenticatedTrustBoundary(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		TelemetryOptions{Region: "us-central1", Application: "ai-gateway"},
	)
	require.NoError(t, err)
	telemetry.generateCorrelationID = func() string { return "generated-correlation" }

	untrusted := []string{
		"header-correlation", "header-region", "header-application", "header-user",
		"arbitrary-context", "acting-user", "raw-claim", "access-secret",
	}
	type arbitraryContextKey struct{}
	var retained requestObservation
	var metadata map[string]any
	options := provider.CallOptions{
		Headers: map[string]string{"X-Existing": "provider-header"},
		ProviderOptions: provider.ProviderOptions{
			"provider": provider.RawProviderOption{Key: "provider", Raw: []byte(`{"private":"provider-option"}`)},
		},
	}
	originalHeaders := options.Headers["X-Existing"]
	originalOption := append([]byte(nil), options.ProviderOptions["provider"].(provider.RawProviderOption).Raw...)

	handler := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		telemetry.ObserveAuthentication(request.Context(), gatewayauth.Observation{
			Outcome: gatewayauth.OutcomeAuthenticated,
			Caller: &gatewayauth.Caller{
				Service:   "verified-service",
				Namespace: "verified-namespace",
				ActingUser: &gatewayauth.ActingUser{
					Subject: "acting-user",
					Type:    types.TypeUser,
				},
			},
		})
		retained = observationFromContext(request.Context())
		metadata = observationMetadata(request.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/aisdk/language-model", nil)
	request.Header.Set("X-Request-ID", "header-correlation")
	request.Header.Set("X-Region", "header-region")
	request.Header.Set("X-Application", "header-application")
	request.Header.Set("X-User", "header-user")
	request.Header.Set("Authorization", "access-secret")
	request = request.WithContext(context.WithValue(request.Context(), arbitraryContextKey{}, "arbitrary-context"))
	handler.ServeHTTP(httptest.NewRecorder(), request)

	assert.Equal(t, requestObservation{
		correlationID: "generated-correlation",
		callerService: "verified-service",
		namespace:     "verified-namespace",
		region:        "us-central1",
		application:   "ai-gateway",
	}, retained)
	assert.Equal(t, map[string]any{
		"gateway.correlation_id": "generated-correlation",
		"gateway.caller_service": "verified-service",
		"gateway.namespace":      "verified-namespace",
		"gateway.region":         "us-central1",
		"gateway.application":    "ai-gateway",
	}, metadata)
	assert.Equal(t, originalHeaders, options.Headers["X-Existing"])
	assert.Equal(t, originalOption, []byte(options.ProviderOptions["provider"].(provider.RawProviderOption).Raw))

	serialized := logs.String()
	for _, expected := range []string{"generated-correlation", "verified-service", "verified-namespace", "us-central1", "ai-gateway"} {
		assert.Contains(t, serialized, expected)
	}
	for _, value := range untrusted {
		assert.NotContains(t, retained.correlationID+retained.callerService+retained.namespace+retained.region+retained.application, value)
		assert.NotContains(t, serialized, value)
	}
}

func TestRequestObservation_OptionalAndInvalidValuesAreOmitted(t *testing.T) {
	tooLong := strings.Repeat("x", maxObservationValueLen+1)
	telemetry, err := NewTelemetry(
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		TelemetryOptions{Region: "bad\nregion", Application: tooLong},
	)
	require.NoError(t, err)
	telemetry.generateCorrelationID = func() string { return "correlation" }

	tests := []struct {
		name      string
		service   string
		namespace string
	}{
		{name: "empty", service: "", namespace: ""},
		{name: "blank", service: "blank value", namespace: "\tnamespace"},
		{name: "over limit", service: tooLong, namespace: tooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got requestObservation
			handler := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				telemetry.ObserveAuthentication(request.Context(), gatewayauth.Observation{
					Outcome: gatewayauth.OutcomeAuthenticated,
					Caller:  &gatewayauth.Caller{Service: test.service, Namespace: test.namespace},
				})
				got = observationFromContext(request.Context())
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/live", nil))
			assert.Equal(t, "correlation", got.correlationID)
			assert.Empty(t, got.callerService)
			assert.Empty(t, got.namespace)
			assert.Empty(t, got.region)
			assert.Empty(t, got.application)
		})
	}
}

func TestRequestObservation_FailedAuthenticationCannotProjectCaller(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	require.NoError(t, err)
	telemetry.generateCorrelationID = func() string { return "correlation" }

	var got requestObservation
	handler := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		telemetry.ObserveAuthentication(request.Context(), gatewayauth.Observation{
			Outcome: gatewayauth.OutcomeFailed,
			Caller:  &gatewayauth.Caller{Service: "must-not-project", Namespace: "must-not-project"},
		})
		got = observationFromContext(request.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/live", nil))

	assert.Equal(t, "correlation", got.correlationID)
	assert.Empty(t, got.callerService)
	assert.Empty(t, got.namespace)
}

func TestRequestObservation_MetadataIsDefensivelyCopied(t *testing.T) {
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "correlation", callerService: "service"})
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)

	metadata := observationMetadata(ctx)
	metadata["gateway.correlation_id"] = "mutated"
	delete(metadata, "gateway.caller_service")

	again := observationMetadata(ctx)
	assert.Equal(t, "correlation", again["gateway.correlation_id"])
	assert.Equal(t, "service", again["gateway.caller_service"])
}

func TestObservationBridge_SnapshotsApprovedValueAndPreservesProviderContext(t *testing.T) {
	state := &telemetryState{}
	state.observation.Store(&requestObservation{correlationID: "initial", callerService: "approved"})
	type providerKey struct{}
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	ctx = context.WithValue(ctx, providerKey{}, "provider-context")
	var observed requestObservation
	model := &observabilityTestModel{generate: func(inner context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
		state.observation.Store(&requestObservation{correlationID: "mutated", callerService: "mutated"})
		observed = observationFromContext(inner)
		assert.Equal(t, "provider-context", inner.Value(providerKey{}))
		return &provider.GenerateResult{}, nil
	}}
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{observationBridgeMiddleware()},
	})

	_, err := wrapped.DoGenerate(ctx, provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, requestObservation{correlationID: "initial", callerService: "approved"}, observed)
}

func TestNewRequestCorrelationID_IsBoundedOpaqueAndUnique(t *testing.T) {
	first := newRequestCorrelationID()
	second := newRequestCorrelationID()
	assert.Len(t, first, requestCorrelationBytes*2)
	assert.Len(t, second, requestCorrelationBytes*2)
	assert.NotEqual(t, first, second)
	assert.Equal(t, first, boundedObservationValue(first))
}
