package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const bodyTestPrompt = `{"prompt":[]}`

type routeAuthInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info routeAuthInfo) GetExtra() map[string][]string { return info.extra }

type bodyTestCaller struct {
	name, service, identity string
}

var (
	bodyConfiguredCaller = bodyTestCaller{name: "configured", service: testInternalService}
	bodyBYOKCaller       = bodyTestCaller{name: "byok", service: "front-door", identity: "user:u1"}
	bodyRefusedCaller    = bodyTestCaller{name: "refused", service: "svc-b"}
)

func bodyTestHandler(t *testing.T, language http.Handler) http.Handler {
	t.Helper()
	policy := gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeCloud, InternalServices: []string{testInternalService}}
	handler, err := newHandler(policy, newTestTelemetry(t), productionRoutes(http.NotFoundHandler(), language, http.NotFoundHandler()))
	require.NoError(t, err)
	return handler
}

func bodyLanguageRequest(caller bodyTestCaller, body io.Reader) *http.Request {
	request := callerRequest(caller, http.MethodPost, "/language-model", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "public")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	return request
}

// callerRequest targets path under the route prefix as the verified caller.
func callerRequest(caller bodyTestCaller, method, path string, body io.Reader) *http.Request {
	url := strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + path
	request := httptest.NewRequest(method, url, body)
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

func TestRouteBody_HandlersReadTheWholeBody(t *testing.T) {
	body := bodyTestPrompt + strings.Repeat(" ", 2<<20)
	var read int
	var readErr error
	probe := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		data, err := io.ReadAll(request.Body)
		read, readErr = len(data), err
		w.WriteHeader(http.StatusNoContent)
	})
	handler := bodyTestHandler(t, probe)
	for _, caller := range []bodyTestCaller{bodyConfiguredCaller, bodyBYOKCaller} {
		t.Run(caller.name, func(t *testing.T) {
			read, readErr = 0, nil
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, bodyLanguageRequest(caller, strings.NewReader(body)))
			assert.Equal(t, http.StatusNoContent, response.Code)
			require.NoError(t, readErr)
			assert.Equal(t, len(body), read)
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

func TestRouteBody_RefusedCallerBodyIsNotRead(t *testing.T) {
	var served atomic.Int32
	probe := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served.Add(1) })
	handler := bodyTestHandler(t, probe)
	body := &countingBody{}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, bodyLanguageRequest(bodyRefusedCaller, body))
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Zero(t, body.reads.Load())
	assert.Zero(t, served.Load())
}
