package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_ExactRoutesMethodsAndAuthenticationOrdering(t *testing.T) {
	authenticator := &serviceAuthenticator{info: serviceAuthInfo()}
	discoveryCalls := 0
	languageCalls := 0
	languagePath := ""
	handler := newTestRouter(t, authenticator,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { discoveryCalls++; w.WriteHeader(http.StatusOK) }),
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			languageCalls++
			languagePath = request.URL.Path
			w.WriteHeader(http.StatusOK)
		}),
	)

	supported := []struct {
		method string
		path   string
		auth   bool
	}{
		{method: http.MethodGet, path: "/live"},
		{method: http.MethodGet, path: "/ready"},
		{method: http.MethodGet, path: "/metrics"},
		{method: http.MethodGet, path: "/api/v1/aisdk/config", auth: true},
		{method: http.MethodPost, path: "/api/v1/aisdk/language-model", auth: true},
	}
	for _, tc := range supported {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth {
			request.Header.Set("X-Access-Token", "access")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code, tc.path)
	}
	assert.Equal(t, 2, authenticator.calls)
	assert.Equal(t, 1, discoveryCalls)
	assert.Equal(t, 1, languageCalls)
	assert.Equal(t, providerv4.LanguageModelPath, languagePath)

	for _, tc := range []struct {
		path  string
		allow string
	}{
		{path: "/live", allow: http.MethodGet},
		{path: "/ready", allow: http.MethodGet},
		{path: "/metrics", allow: http.MethodGet},
		{path: "/api/v1/aisdk/config", allow: http.MethodGet},
		{path: "/api/v1/aisdk/language-model", allow: http.MethodPost},
	} {
		request := httptest.NewRequest(http.MethodHead, tc.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
		assert.Equal(t, tc.allow, response.Header().Get("Allow"))
	}
	assert.Equal(t, 2, authenticator.calls, "unsupported methods must not authenticate")
	assert.Equal(t, 1, discoveryCalls)
	assert.Equal(t, 1, languageCalls)

	for _, path := range []string{"/language-model", "/api/v1/aisdk", "/api/v1/aisdk/config/", "/unknown"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, response.Code)
	}
	encoded := httptest.NewRequest(http.MethodGet, "/api%2Fv1%2Faisdk%2Fconfig", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, encoded)
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, 2, authenticator.calls)
}

func TestRouter_AuthenticationPrecedesProtectedHandlers(t *testing.T) {
	authenticator := &serviceAuthenticator{err: assert.AnError}
	protectedCalls := 0
	handler := newTestRouter(t, authenticator,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { protectedCalls++ }),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { protectedCalls++ }),
	)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/aisdk/config"},
		{method: http.MethodPost, path: "/api/v1/aisdk/language-model"},
	} {
		body := &rejectReadBody{}
		request := httptest.NewRequest(tc.method, tc.path, body)
		request.Header.Set("X-Access-Token", "access")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.Zero(t, body.reads)
	}
	assert.Zero(t, protectedCalls)
}

func TestRouter_AuthenticationFailureTelemetryUsesFixedClassOnce(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	readiness := &Readiness{}
	readiness.Set(true)
	errorWriter := providerv4.NewHostErrorWriter()
	handler, err := NewRouter(RouterConfig{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: &serviceAuthenticator{err: errors.New("private verifier detail")},
		ErrorWriter:   errorWriter,
		Discovery:     http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LanguageModel: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/aisdk/config", nil)
	request.Header.Set("X-Access-Token", "invalid")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	assert.Equal(t, 1, strings.Count(logs.String(), "http request completed"))
	assert.Contains(t, logs.String(), `"authentication":"authentication_failed"`)
	assert.NotContains(t, logs.String(), "private verifier detail")
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

func TestTelemetry_NormalizationMetricsAndPrivacy(t *testing.T) {
	for _, tc := range []struct{ input, output string }{
		{input: "/live", output: "live"},
		{input: "/ready", output: "ready"},
		{input: "/metrics", output: "metrics"},
		{input: "/api/v1/aisdk/config", output: "config"},
		{input: "/api/v1/aisdk/language-model", output: "language_model"},
		{input: "/arbitrary/private/model", output: "unmatched"},
	} {
		assert.Equal(t, tc.output, normalizeRoute(tc.input))
	}
	encodedRoute := httptest.NewRequest(http.MethodGet, "/api%2Fv1%2Faisdk%2Fconfig", nil)
	assert.Equal(t, "unmatched", normalizeRequestRoute(encodedRoute))

	for _, tc := range []struct{ input, output string }{{http.MethodGet, "GET"}, {http.MethodPost, "POST"}, {"PRIVATE", "other"}} {
		assert.Equal(t, tc.output, normalizeMethod(tc.input))
	}
	for _, tc := range []struct {
		input  gatewayauth.Outcome
		output string
	}{{0, "not_attempted"}, {gatewayauth.OutcomeAuthenticated, "authenticated"}, {gatewayauth.OutcomeFailed, "authentication_failed"}} {
		assert.Equal(t, tc.output, authenticationClass(tc.input))
	}
	for _, tc := range []struct {
		input  int
		output string
	}{{100, "1xx"}, {200, "2xx"}, {302, "3xx"}, {404, "4xx"}, {500, "5xx"}, {999, "5xx"}} {
		assert.Equal(t, tc.output, normalizeStatus(tc.input))
	}

	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	telemetry.SetReady(true)
	privateValues := []string{"private-model", "secret-token", "provider-private", "https://private.example", "stack-private", "caller-private"}
	handler := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	request := httptest.NewRequest("PRIVATE", "/arbitrary/private-model", nil)
	request.Header.Set("Authorization", "secret-token")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	metricsResponse := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	metrics := metricsResponse.Body.String()
	assert.Contains(t, metrics, "go_goroutines")
	assert.Contains(t, metrics, "process_cpu_seconds")
	assert.Contains(t, metrics, "grafana_ai_gateway_ready 1")
	assert.Contains(t, metrics, "grafana_ai_gateway_http_requests_in_flight")
	assert.Contains(t, metrics, "grafana_ai_gateway_http_requests_total")
	assert.Contains(t, metrics, "grafana_ai_gateway_http_request_duration_seconds")
	assert.Contains(t, metrics, `route="unmatched"`)
	assert.Contains(t, metrics, `method="other"`)
	assert.Contains(t, metrics, `status="4xx"`)
	for _, private := range privateValues {
		assert.NotContains(t, metrics, private)
		assert.NotContains(t, logs.String(), private)
	}
	assert.Contains(t, logs.String(), `"route":"unmatched"`)
	assert.Contains(t, logs.String(), `"method":"other"`)
	assert.Contains(t, logs.String(), `"status":"4xx"`)
}

func TestTelemetry_ObserveAuthenticationRetainsNormalizedCallerPrivately(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	state := &telemetryState{}
	ctx := context.WithValue(context.Background(), telemetryStateKey{}, state)
	caller := gatewayauth.Caller{Service: "caller-private", Namespace: "stack-private"}
	telemetry.ObserveAuthentication(ctx, gatewayauth.Observation{Outcome: gatewayauth.OutcomeAuthenticated, Caller: &caller})

	assert.Equal(t, uint32(gatewayauth.OutcomeAuthenticated), state.authOutcome.Load())
	state.callerMu.Lock()
	assert.Equal(t, "caller-private", state.service)
	assert.Equal(t, "stack-private", state.namespace)
	state.callerMu.Unlock()
}

func TestTelemetry_DuplicateRegistrationFails(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := newTelemetry(logger, registry)
	require.NoError(t, err)
	_, err = newTelemetry(logger, registry)
	require.Error(t, err)
}

func newTestRouter(t *testing.T, authenticator authn.Authenticator, discovery, language http.Handler) http.Handler {
	t.Helper()
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	readiness := &Readiness{}
	readiness.Set(true)
	telemetry.SetReady(true)
	errorWriter := providerv4.NewHostErrorWriter()
	handler, err := NewRouter(RouterConfig{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: authenticator,
		ErrorWriter:   errorWriter,
		Discovery:     discovery,
		LanguageModel: language,
	})
	require.NoError(t, err)
	return handler
}

type rejectReadBody struct {
	reads int
}

func (body *rejectReadBody) Read([]byte) (int, error) {
	body.reads++
	return 0, errors.New("protected body was read before authentication")
}

type serviceAuthenticator struct {
	info  types.AuthInfo
	err   error
	calls int
}

func (authenticator *serviceAuthenticator) Authenticate(context.Context, authn.TokenProvider) (types.AuthInfo, error) {
	authenticator.calls++
	return authenticator.info, authenticator.err
}

type serviceAuth struct{}

func serviceAuthInfo() types.AuthInfo                   { return serviceAuth{} }
func (serviceAuth) GetUID() string                      { return "access-policy:1" }
func (serviceAuth) GetIdentifier() string               { return "1" }
func (serviceAuth) GetIdentityType() types.IdentityType { return types.TypeAccessPolicy }
func (serviceAuth) GetNamespace() string                { return "stack-1" }
func (serviceAuth) GetGroups() []string                 { return nil }
func (serviceAuth) GetExtra() map[string][]string {
	return map[string][]string{authn.ServiceIdentityKey: {"service"}}
}
func (serviceAuth) GetSubject() string                     { return "access-policy:1" }
func (serviceAuth) GetAudience() []string                  { return []string{"ai-sdk"} }
func (serviceAuth) GetTokenPermissions() []string          { return nil }
func (serviceAuth) GetTokenDelegatedPermissions() []string { return nil }
func (serviceAuth) GetName() string                        { return "service" }
func (serviceAuth) GetEmail() string                       { return "" }
func (serviceAuth) GetEmailVerified() bool                 { return false }
func (serviceAuth) GetUsername() string                    { return "" }
func (serviceAuth) GetAuthenticatedBy() string             { return "" }
func (serviceAuth) GetAccessToken() string                 { return "" }
func (serviceAuth) GetIDToken() string                     { return "" }
