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
	handler := NewRouter(RouterDependencies{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: gatewayauth.NewAccessTokenAuthenticator(&serviceAuthenticator{err: errors.New("private verifier detail")}),
		ErrorWriter:   errorWriter,
		Discovery:     http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LanguageModel: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	})
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
	retained := observationFromContext(ctx)
	assert.Equal(t, "caller-private", retained.callerService)
	assert.Equal(t, "stack-private", retained.namespace)
}

func TestTelemetry_DuplicateRegistrationFails(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := newTelemetry(logger, registry)
	require.NoError(t, err)
	_, err = newTelemetry(logger, registry)
	require.Error(t, err)
}

func TestTelemetry_RegistererSharesExistingMetricsRouteAndRejectsDuplicates(t *testing.T) {
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	collector := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "grafana_ai_gateway",
		Name:      "model_test_total",
		Help:      "Test logical model collector.",
	})
	require.NoError(t, telemetry.Registerer().Register(collector))
	require.Error(t, telemetry.Registerer().Register(collector))
	collector.Inc()

	response := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Contains(t, response.Body.String(), "grafana_ai_gateway_model_test_total 1")
}

func TestTelemetry_AgentExportFailuresUseOnlyFixedDiagnosticsAndBoundedLabels(t *testing.T) {
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)

	for _, class := range []agentExportFailureClass{
		agentExportFailureQueue,
		agentExportFailureSerialization,
		agentExportFailureTransport,
		agentExportFailureRejected,
		agentExportFailureShutdown,
		agentExportFailureClass("https://private.invalid bearer-secret"),
	} {
		telemetry.observeAgentExportFailure(class)
	}

	response := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	serialized := logs.String() + response.Body.String()
	for _, class := range []string{"queue_full", "serialization", "transport", "rejected", "shutdown", "unknown"} {
		assert.Contains(t, serialized, class)
	}
	assert.NotContains(t, serialized, "https://private.invalid")
	assert.NotContains(t, serialized, "bearer-secret")
}

func TestTelemetry_AuthenticationClosedNormalizationAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		source                  gatewayauth.Source
		outcome                 gatewayauth.Outcome
		wantSource, wantOutcome string
	}{
		{"access success", gatewayauth.SourceAccessToken, gatewayauth.OutcomeAuthenticated, "access-token", "authenticated"},
		{"cloud failure", gatewayauth.SourceCloudGateway, gatewayauth.OutcomeFailed, "cloud-gateway", "authentication_failed"},
		{"zero", "", 0, "unknown", "not_attempted"},
		{"untrusted", "private-source", 255, "unknown", "not_attempted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
			require.NoError(t, err)
			handler := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				telemetry.ObserveAuthentication(r.Context(), gatewayauth.Observation{
					Source: tc.source, Outcome: tc.outcome,
					Caller: &gatewayauth.Caller{Source: "private-caller-source", Service: "private-service", Namespace: "private-namespace", ActingUser: &gatewayauth.ActingUser{Subject: "private-subject"}},
				})
				w.WriteHeader(http.StatusOK)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/live", nil))
			metrics := telemetryMetrics(telemetry)
			assert.Contains(t, metrics, `grafana_ai_gateway_authentication_total{outcome="`+tc.wantOutcome+`",source="`+tc.wantSource+`"} 1`)
			assert.Contains(t, logs.String(), `"authentication":"`+tc.wantOutcome+`"`)
			assert.Contains(t, logs.String(), `"authentication_source":"`+tc.wantSource+`"`)
			for _, private := range []string{"private-source", "private-caller-source", "private-subject"} {
				assert.NotContains(t, metrics, private)
				assert.NotContains(t, logs.String(), private)
			}
		})
	}
}

func TestRouter_RouteSeparationBothAuthenticationModes(t *testing.T) {
	for _, source := range []gatewayauth.Source{gatewayauth.SourceAccessToken, gatewayauth.SourceCloudGateway} {
		for _, composition := range []struct {
			name             string
			build            func(RouterDependencies) http.Handler
			api, operational bool
		}{
			{"combined", NewRouter, true, true},
			{"api", NewAPIRouter, true, false},
			{"operational", NewOperationalRouter, false, true},
		} {
			t.Run(string(source)+"/"+composition.name, func(t *testing.T) {
				telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
				require.NoError(t, err)
				readiness := &Readiness{}
				readiness.Set(true)
				authenticator, headers := serviceModeAuthentication(source)
				authCalls, protectedCalls := 0, 0
				deps := RouterDependencies{Telemetry: telemetry}
				if composition.operational {
					deps.Readiness = readiness
				}
				if composition.api {
					deps.AuthSource = source
					deps.Authenticator = requestAuthenticatorFunc(func(ctx context.Context, h http.Header) (gatewayauth.Caller, error) {
						authCalls++
						return authenticator.Authenticate(ctx, h)
					})
					deps.ErrorWriter = providerv4.NewHostErrorWriter()
					deps.Discovery = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						protectedCalls++
						caller, ok := gatewayauth.CallerFromContext(r.Context())
						assert.True(t, ok)
						assert.Equal(t, source, caller.Source)
						w.WriteHeader(http.StatusOK)
					})
					deps.LanguageModel = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.Equal(t, providerv4.LanguageModelPath, r.URL.Path)
						assert.Empty(t, r.URL.RawPath)
						deps.Discovery.ServeHTTP(w, r)
					})
				}
				handler := composition.build(deps)
				for _, route := range []struct {
					path, method string
					api          bool
				}{
					{"/live", http.MethodGet, false},
					{"/ready", http.MethodGet, false},
					{"/metrics", http.MethodGet, false},
					{"/api/v1/aisdk/config", http.MethodGet, true},
					{"/api/v1/aisdk/language-model", http.MethodPost, true},
				} {
					for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodOptions, http.MethodPut} {
						t.Run(method+route.path, func(t *testing.T) {
							beforeAuth, beforeProtected := authCalls, protectedCalls
							request := httptest.NewRequest(method, route.path, nil)
							request.Header = headers.Clone()
							response := httptest.NewRecorder()
							handler.ServeHTTP(response, request)
							present := (route.api && composition.api) || (!route.api && composition.operational)
							switch {
							case !present:
								assert.Equal(t, http.StatusNotFound, response.Code)
								assert.Empty(t, response.Header().Get("Allow"))
							case method != route.method:
								assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
								assert.Equal(t, route.method, response.Header().Get("Allow"))
							default:
								assert.Equal(t, http.StatusOK, response.Code)
							}
							if present && method == route.method && route.api {
								assert.Equal(t, beforeAuth+1, authCalls)
								assert.Equal(t, beforeProtected+1, protectedCalls)
							} else {
								assert.Equal(t, beforeAuth, authCalls)
								assert.Equal(t, beforeProtected, protectedCalls)
							}
						})
					}
				}
				beforeAuth := authCalls
				for _, path := range []string{"/language-model", "/api/v1/aisdk", "/api/v1/aisdk/config/", "/api/v1/aisdk/language-model/", "/live/", "/ready/", "/metrics/", "/unknown", "/api%2Fv1%2Faisdk%2Fconfig", "/api/v1/aisdk/language%2Dmodel", "/%6cive", "/%72eady", "/%6detrics", "/api/v1/../v1/aisdk/config", "//live"} {
					for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
						assert.Equal(t, http.StatusNotFound, response.Code, method+path)
						assert.Empty(t, response.Header().Get("Allow"))
					}
				}
				assert.Equal(t, beforeAuth, authCalls)
				if composition.api {
					assert.Contains(t, telemetryMetrics(telemetry), `grafana_ai_gateway_authentication_total{outcome="authenticated",source="`+string(source)+`"} 2`)
				} else {
					assert.NotContains(t, telemetryMetrics(telemetry), "grafana_ai_gateway_authentication_total")
				}
				if composition.operational {
					readiness.Set(false)
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
					assert.Equal(t, http.StatusServiceUnavailable, response.Code)
				}
			})
		}
	}
}

func TestRouter_FixedAuthenticationSourceIncludingFailures(t *testing.T) {
	for _, source := range []gatewayauth.Source{gatewayauth.SourceAccessToken, gatewayauth.SourceCloudGateway} {
		for _, tc := range []struct {
			name, outcome string
			err           error
			status, calls int
		}{
			{"success", "authenticated", nil, http.StatusOK, 2},
			{"failure", "authentication_failed", errors.New("private-verifier-detail"), http.StatusUnauthorized, 0},
		} {
			for _, composition := range []struct {
				name  string
				build func(RouterDependencies) http.Handler
			}{{"combined", NewRouter}, {"api", NewAPIRouter}} {
				t.Run(string(source)+"/"+composition.name+"/"+tc.name, func(t *testing.T) {
					var logs bytes.Buffer
					telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
					require.NoError(t, err)
					calls := 0
					protected := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusOK) })
					handler := composition.build(RouterDependencies{
						Telemetry: telemetry, AuthSource: source, ErrorWriter: providerv4.NewHostErrorWriter(),
						Authenticator: requestAuthenticatorFunc(func(context.Context, http.Header) (gatewayauth.Caller, error) {
							return gatewayauth.Caller{Source: "private-source", Service: "private-service", Namespace: "private-namespace"}, tc.err
						}),
						Discovery: protected, LanguageModel: protected,
					})
					for _, route := range []struct{ method, path string }{
						{http.MethodGet, "/api/v1/aisdk/config"}, {http.MethodPost, "/api/v1/aisdk/language-model"},
					} {
						request := httptest.NewRequest(route.method, route.path, &rejectReadBody{})
						request.Header.Set("Authorization", "private-credential")
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, request)
						assert.Equal(t, tc.status, response.Code)
					}
					assert.Equal(t, tc.calls, calls)
					metrics := telemetryMetrics(telemetry)
					assert.Contains(t, metrics, `grafana_ai_gateway_authentication_total{outcome="`+tc.outcome+`",source="`+string(source)+`"} 2`)
					assert.Equal(t, 2, strings.Count(logs.String(), `"authentication_source":"`+string(source)+`"`))
					assert.Equal(t, 2, strings.Count(logs.String(), `"authentication":"`+tc.outcome+`"`))
					for _, private := range []string{"private-credential", "private-verifier-detail", "private-source"} {
						assert.NotContains(t, metrics, private)
						assert.NotContains(t, logs.String(), private)
					}
					if source == gatewayauth.SourceCloudGateway || tc.err != nil {
						for _, private := range []string{"private-service", "private-namespace"} {
							assert.NotContains(t, metrics, private)
							assert.NotContains(t, logs.String(), private)
						}
					}
				})
			}
		}
	}
}

func TestRouter_AuthenticationRejectionsBothModes(t *testing.T) {
	for _, source := range []gatewayauth.Source{gatewayauth.SourceAccessToken, gatewayauth.SourceCloudGateway} {
		for _, composition := range []struct {
			name  string
			build func(RouterDependencies) http.Handler
		}{{"combined", NewRouter}, {"api", NewAPIRouter}} {
			rejections := []string{"missing", "duplicate", "wrong mode"}
			if source == gatewayauth.SourceCloudGateway {
				rejections = append(rejections, "Authorization", "X-Access-Token", "X-Grafana-Id")
			}
			for _, rejection := range rejections {
				t.Run(string(source)+"/"+composition.name+"/"+rejection, func(t *testing.T) {
					telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
					require.NoError(t, err)
					authenticator, headers := serviceModeAuthentication(source)
					key := "X-Access-Token"
					if source == gatewayauth.SourceCloudGateway {
						key = "X-Scope-OrgID"
					}
					switch rejection {
					case "missing":
						headers.Del(key)
					case "duplicate":
						headers.Add(key, headers.Get(key))
					case "wrong mode":
						other := gatewayauth.SourceAccessToken
						if source == other {
							other = gatewayauth.SourceCloudGateway
						}
						_, headers = serviceModeAuthentication(other)
					default:
						headers.Set(rejection, "private-credential")
					}
					protected := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { assert.Fail(t, "rejected request reached protected handler") })
					handler := composition.build(RouterDependencies{Telemetry: telemetry, AuthSource: source, Authenticator: authenticator, ErrorWriter: providerv4.NewHostErrorWriter(), Discovery: protected, LanguageModel: protected})
					for _, route := range []struct{ method, path string }{
						{http.MethodGet, "/api/v1/aisdk/config"}, {http.MethodPost, "/api/v1/aisdk/language-model"},
					} {
						body := &rejectReadBody{}
						request := httptest.NewRequest(route.method, route.path, body)
						request.Header = headers.Clone()
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, request)
						assert.Equal(t, http.StatusUnauthorized, response.Code)
						assert.Zero(t, body.reads)
					}
					assert.Contains(t, telemetryMetrics(telemetry), `grafana_ai_gateway_authentication_total{outcome="authentication_failed",source="`+string(source)+`"} 2`)
				})
			}
		}
	}
}

func telemetryMetrics(telemetry *Telemetry) string {
	response := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return response.Body.String()
}

type requestAuthenticatorFunc func(context.Context, http.Header) (gatewayauth.Caller, error)

func (authenticate requestAuthenticatorFunc) Authenticate(ctx context.Context, headers http.Header) (gatewayauth.Caller, error) {
	return authenticate(ctx, headers)
}

func serviceModeAuthentication(source gatewayauth.Source) (gatewayauth.RequestAuthenticator, http.Header) {
	if source == gatewayauth.SourceCloudGateway {
		return gatewayauth.NewCloudProviderWireAuthenticator(), http.Header{
			"X-Scope-Orgid": {"123456789"},
		}
	}
	return gatewayauth.NewAccessTokenAuthenticator(&serviceAuthenticator{info: serviceAuthInfo()}), http.Header{"X-Access-Token": {"private-access-token"}}
}

func newTestRouter(t *testing.T, authenticator authn.Authenticator, discovery, language http.Handler) http.Handler {
	t.Helper()
	telemetry, err := NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	readiness := &Readiness{}
	readiness.Set(true)
	telemetry.SetReady(true)
	errorWriter := providerv4.NewHostErrorWriter()
	return NewRouter(RouterDependencies{
		Readiness:     readiness,
		Telemetry:     telemetry,
		Authenticator: gatewayauth.NewAccessTokenAuthenticator(authenticator),
		ErrorWriter:   errorWriter,
		Discovery:     discovery,
		LanguageModel: language,
	})
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
