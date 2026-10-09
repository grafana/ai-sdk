package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticAuth_AdmissionBeforeProtectedWork(t *testing.T) {
	const key = "STATIC_SECRET_SENTINEL"
	authenticator, err := gatewayauth.NewStaticKeyAuthenticator([]gatewayauth.StaticIdentity{{Name: "deployment", KeyDigest: sha256.Sum256([]byte(key))}})
	require.NoError(t, err)
	var logs bytes.Buffer
	telemetry, err := NewTelemetry(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	catalog := &accountCatalogSpy{model: &observabilityTestModel{}}
	writer := providerv4.NewHostErrorWriter()
	language, err := providerv4.New(providerv4.Config{Selector: NewConfiguredSelector(catalog), Limits: serviceTestLimits()})
	require.NoError(t, err)
	deps := RouterDependencies{Readiness: &Readiness{}, Telemetry: telemetry, Authenticator: authenticator, AuthSource: gatewayauth.SourceStaticKey, ErrorWriter: writer, Discovery: ConfiguredDiscovery(discovery.New(catalog, writer), writer), LanguageModel: language}
	router := NewAPIRouter(deps)
	for _, route := range []struct{ method, path, stream string }{{"GET", "/api/v1/aisdk/config", "false"}, {"POST", "/api/v1/aisdk/language-model", "false"}, {"POST", "/api/v1/aisdk/language-model", "true"}} {
		for _, headers := range []http.Header{{}, {"X-Access-Token": {"wrong"}}, {"Authorization": {"Bearer " + key}, "X-Access-Token": {key}}, {"X-Access-Token": {key}, "X-Scope-OrgID": {""}}} {
			body := &rejectReadBody{}
			request := httptest.NewRequest(route.method, route.path, body)
			request.Header = headers
			request.Header.Set(providerv4.HeaderStreaming, route.stream)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assert.Equal(t, 401, response.Code)
			assert.Zero(t, body.reads)
			assert.Zero(t, catalog.resolved.Load())
			assert.Zero(t, catalog.listed.Load())
			assert.NotContains(t, response.Body.String(), key)
		}
	}
	var observed gatewayauth.Caller
	handler := gatewayauth.Middleware(authenticator, func(http.ResponseWriter) { t.Error("unexpected rejection") }, func(_ context.Context, observation gatewayauth.Observation) {
		require.NotNil(t, observation.Caller)
		observed = *observation.Caller
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, ok := gatewayauth.CallerFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, observed, caller)
		w.WriteHeader(200)
	}))
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-Access-Token", key)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	assert.Equal(t, "deployment", observed.Service)
	assert.Equal(t, "local:deployment", observed.Subject)
	assert.Empty(t, observed.Namespace)
	assert.Nil(t, observed.ActingUser)
	assert.Equal(t, gatewayauth.ConfiguredAccounts, observed.AccountAccess())
	metrics := httptest.NewRecorder()
	NewOperationalRouter(deps).ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	raw, err := io.ReadAll(metrics.Result().Body)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `source="static-key"`)
	assert.NotContains(t, string(raw), key)
	assert.NotContains(t, logs.String(), key)
}
