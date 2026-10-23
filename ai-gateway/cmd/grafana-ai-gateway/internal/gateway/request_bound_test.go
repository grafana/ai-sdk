package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/service"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const boundTestBody = `{"prompt":[]}`

type routeAuthInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info routeAuthInfo) GetExtra() map[string][]string { return info.extra }

type boundTestCaller struct {
	name, service, identity string
}

var (
	boundConfiguredCaller = boundTestCaller{name: "configured", service: testInternalService}
	boundBYOKCaller       = boundTestCaller{name: "byok", service: "front-door", identity: "user:u1"}
	boundRefusedCaller    = boundTestCaller{name: "refused", service: "svc-b"}
)

func boundTestHandler(t *testing.T, limit int64, routes routeHandlers) http.Handler {
	t.Helper()
	telemetry, err := service.NewTelemetry(slog.New(slog.DiscardHandler), service.TelemetryOptions{Registerer: prometheus.NewRegistry()})
	require.NoError(t, err)
	policy := gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeCloud, InternalServices: []string{testInternalService}}
	return newHandler(policy, limit, telemetry, providerv4.NewHostErrorWriter(), routes)
}

func boundLanguageRequest(caller boundTestCaller, body io.Reader) *http.Request {
	url := strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/language-model"
	request := httptest.NewRequest(http.MethodPost, url, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "public")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	var claims authn.Claims[authn.AccessTokenClaims]
	claims.Rest.Namespace = testNamespace
	if typ, uid, err := types.ParseTypeID(caller.identity); err == nil {
		claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: typ, Identifier: uid}}
	}
	info := routeAuthInfo{AuthInfo: authn.NewAccessTokenAuthInfo(claims), extra: map[string][]string{gatewayauth.DirectCallerServiceKey: {caller.service}}}
	ctx := types.WithAuthInfo(request.Context(), info)
	ctx = resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: testNamespace}})
	return request.WithContext(ctx)
}

func TestRequestBound_LanguageModelBoundary(t *testing.T) {
	limit := int64(len(boundTestBody) + 1)
	var selections atomic.Int32
	language, err := providerv4.New(providerv4.Config{
		Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (providerv4.Selection, error) {
			selections.Add(1)
			return providerv4.Selection{}, catalog.ErrUnknownModel
		},
		Limits: providerv4.Limits{
			UnaryResponseBytes:  1 << 20,
			StreamParts:         1_000,
			StreamFrameBytes:    1 << 20,
			ModelDuration:       time.Second,
			StreamIdleDuration:  time.Second,
			StreamDrainDuration: time.Second,
		},
	})
	require.NoError(t, err)
	handler := boundTestHandler(t, limit, routeHandlers{configuredLanguage: language})
	for _, tc := range []struct {
		name    string
		padding int
		status  int
		selects int32
	}{
		{name: "below", padding: 0, status: http.StatusNotFound, selects: 1},
		{name: "at", padding: 1, status: http.StatusNotFound, selects: 1},
		{name: "above", padding: 2, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selections.Store(0)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, boundLanguageRequest(boundConfiguredCaller, strings.NewReader(boundTestBody+strings.Repeat(" ", tc.padding))))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.selects, selections.Load())
			if tc.status == http.StatusBadRequest {
				assert.JSONEq(t, `{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}`, response.Body.String())
			}
		})
	}
}

func TestRequestBound_EveryRouteHandlerInheritsTheBound(t *testing.T) {
	limit := int64(len(boundTestBody))
	type observation struct {
		read     int
		tooLarge bool
	}
	var observed observation
	probe := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		var tooLarge *http.MaxBytesError
		observed = observation{read: len(body), tooLarge: errors.As(err, &tooLarge)}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := boundTestHandler(t, limit, routeHandlers{configuredLanguage: probe, byokLanguage: probe})
	for _, caller := range []boundTestCaller{boundConfiguredCaller, boundBYOKCaller} {
		t.Run(caller.name, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				body     string
				tooLarge bool
			}{
				{name: "at", body: boundTestBody},
				{name: "above", body: boundTestBody + " ", tooLarge: true},
			} {
				observed = observation{}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, boundLanguageRequest(caller, strings.NewReader(tc.body)))
				assert.Equal(t, http.StatusNoContent, response.Code, tc.name)
				assert.Equal(t, tc.tooLarge, observed.tooLarge, tc.name)
				assert.LessOrEqual(t, int64(observed.read), limit, tc.name)
			}
		})
	}
}

type countingBody struct {
	reads atomic.Int32
}

func (body *countingBody) Read([]byte) (int, error) {
	body.reads.Add(1)
	return 0, io.EOF
}

func TestRequestBound_RefusedCallerBodyIsNotRead(t *testing.T) {
	var served atomic.Int32
	probe := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served.Add(1) })
	handler := boundTestHandler(t, 1, routeHandlers{configuredLanguage: probe, byokLanguage: probe})
	body := &countingBody{}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, boundLanguageRequest(boundRefusedCaller, body))
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Zero(t, body.reads.Load())
	assert.Zero(t, served.Load())
}
