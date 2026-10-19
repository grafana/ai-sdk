package platformhost

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/grafana/grafana-app-sdk/app"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
	"github.com/grafana/grafana-app-sdk/resource"
)

const (
	testPluginID  = "test-plugin-app"
	testGroup     = "test.ext.grafana.app"
	testVersion   = "v1alpha1"
	testNamespace = "stacks-1"
)

type stubApp struct {
	app.App
	handler http.Handler
}

func (s stubApp) ProvideRouteHandler() (http.Handler, error) { return s.handler, nil }

type observed struct {
	mu        sync.Mutex
	calls     atomic.Int32
	token     string
	namespace string
}

func (o *observed) handler(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.calls.Add(1)
		info, _ := types.AuthInfoFrom(r.Context())
		o.mu.Lock()
		o.token = info.GetAccessToken()
		o.namespace = resource.RouteRequestInfoFrom(r.Context()).Namespace
		o.mu.Unlock()
		if next != nil {
			next(w, r)
		}
	})
}

func newTestHost(t *testing.T, topology Topology, handler http.Handler) (*Host, *InProcess, *httptest.Server) {
	t.Helper()
	host, err := New(Config{Topology: topology, PluginID: testPluginID, Group: testGroup, Version: testVersion})
	require.NoError(t, err)
	served, err := host.ServeInProcess(grpcplugin.ServeOpts{
		RouteServer:   appadapter.NewRouteAdapter(stubApp{handler: handler}),
		Authenticator: host.Authenticator(),
		PluginID:      testPluginID,
	})
	require.NoError(t, err)
	server := httptest.NewServer(served.Handler())
	t.Cleanup(func() {
		server.Close()
		served.Close()
	})
	return host, served, server
}

func routeURL(server *httptest.Server, namespace string) string {
	return server.URL + "/apis/" + testGroup + "/" + testVersion + "/namespaces/" + namespace + "/echo"
}

func chain(t *testing.T, host *Host, token string) []string {
	t.Helper()
	claims, err := host.signer.access.Verify(context.Background(), token)
	require.NoError(t, err)
	result := []string{claims.Subject}
	for actor := claims.Rest.Actor; actor != nil; actor = actor.Actor {
		result = append(result, actor.Subject)
	}
	return result
}

func TestHost_ActorChains(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		topology Topology
		request  func(t *testing.T, host *Host, r *http.Request)
		chain    []string
	}{
		{
			name:     "direct service call",
			topology: TopologyCloud,
			request: func(t *testing.T, host *Host, r *http.Request) {
				token, err := host.ServiceToken("svc-a", testNamespace)
				require.NoError(t, err)
				r.Header.Set("X-Access-Token", token)
			},
			chain: []string{"access-policy:router", "access-policy:svc-a"},
		},
		{
			name:     "on behalf of a user",
			topology: TopologyCloud,
			request: func(t *testing.T, host *Host, r *http.Request) {
				user, err := host.IDToken(types.TypeUser, "u1", testNamespace)
				require.NoError(t, err)
				token, err := host.Exchange(ctx, "svc-a", authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{testPluginID}, SubjectToken: user})
				require.NoError(t, err)
				r.Header.Set("X-Access-Token", token)
			},
			chain: []string{"access-policy:router", "access-policy:svc-a", "user:u1"},
		},
		{
			name:     "plugin on behalf of a delegated user",
			topology: TopologyCloud,
			request: func(t *testing.T, host *Host, r *http.Request) {
				user, err := host.IDToken(types.TypeUser, "u1", testNamespace)
				require.NoError(t, err)
				viaFrontDoor, err := host.Exchange(ctx, FrontDoorService, authn.TokenExchangeRequest{Namespace: testNamespace, SubjectToken: user})
				require.NoError(t, err)
				delegated, err := host.Exchange(ctx, RouterService, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{"other-app"}, SubjectToken: viaFrontDoor})
				require.NoError(t, err)
				token, err := host.Exchange(ctx, "other-app-operator", authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{testPluginID}, SubjectToken: delegated})
				require.NoError(t, err)
				r.Header.Set("X-Access-Token", token)
			},
			chain: []string{"access-policy:router", "access-policy:other-app-operator", "access-policy:router", "access-policy:front-door", "user:u1"},
		},
		{
			name:     "relayed call",
			topology: TopologyCloud,
			request: func(t *testing.T, host *Host, r *http.Request) {
				original, err := host.ServiceToken("svc-a", testNamespace)
				require.NoError(t, err)
				token, err := host.Exchange(ctx, "relay", authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{testPluginID}, SubjectToken: original})
				require.NoError(t, err)
				r.Header.Set("X-Access-Token", token)
			},
			chain: []string{"access-policy:router", "access-policy:relay", "access-policy:svc-a"},
		},
		{
			name:     "front-door user",
			topology: TopologyCloud,
			request: func(_ *testing.T, host *Host, r *http.Request) {
				r.AddCookie(SessionCookie(host.AddSession(types.TypeUser, "u1", testNamespace)))
			},
			chain: []string{"access-policy:router", "access-policy:front-door", "user:u1"},
		},
		{
			name:     "front-door service account",
			topology: TopologyCloud,
			request: func(_ *testing.T, host *Host, r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+host.AddServiceAccount("sa1", testNamespace))
			},
			chain: []string{"access-policy:router", "access-policy:front-door", "service-account:sa1"},
		},
		{
			name:     "self-hosted service account",
			topology: TopologySelfHosted,
			request: func(_ *testing.T, host *Host, r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+host.AddServiceAccount("sa1", "default"))
			},
			chain: []string{"access-policy:grafana", "service-account:sa1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen observed
			host, _, server := newTestHost(t, tc.topology, seen.handler(nil))
			namespace := testNamespace
			if tc.topology == TopologySelfHosted {
				namespace = "default"
			}
			request, err := http.NewRequest(http.MethodGet, routeURL(server, namespace), nil)
			require.NoError(t, err)
			tc.request(t, host, request)
			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode)

			seen.mu.Lock()
			defer seen.mu.Unlock()
			assert.Equal(t, namespace, seen.namespace)
			assert.Equal(t, tc.chain, chain(t, host, seen.token))
		})
	}
}

func TestHost_StreamsIncrementally(t *testing.T) {
	release := make(chan struct{})
	host, _, server := newTestHost(t, TopologyCloud, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "second\n")
	}))
	request, err := http.NewRequest(http.MethodGet, routeURL(server, testNamespace), nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+host.AddServiceAccount("sa1", testNamespace))
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "first\n", line)
	close(release)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "second\n", line)
}

func TestHost_RefusesBeforeHandler(t *testing.T) {
	ctx := context.Background()
	t.Run("access token as bearer", func(t *testing.T) {
		var seen observed
		host, _, server := newTestHost(t, TopologyCloud, seen.handler(nil))
		token, err := host.ServiceToken("svc-a", testNamespace)
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodGet, routeURL(server, testNamespace), nil)
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Zero(t, seen.calls.Load())
	})

	tests := []struct {
		name      string
		audience  string
		namespace string
		code      codes.Code
	}{
		{name: "plugin ID audience", audience: testPluginID, namespace: testNamespace, code: codes.OK},
		{name: "API group audience", audience: testGroup, namespace: testNamespace, code: codes.OK},
		{name: "foreign audience", audience: "other-app", namespace: testNamespace, code: codes.Unauthenticated},
		{name: "uncovered namespace", audience: testPluginID, namespace: "stacks-2", code: codes.PermissionDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen observed
			host, served, _ := newTestHost(t, TopologyCloud, seen.handler(nil))
			subject, err := host.ServiceToken("svc-a", testNamespace)
			require.NoError(t, err)
			token, err := host.Exchange(ctx, RouterService, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{tc.audience}, SubjectToken: subject})
			require.NoError(t, err)

			request := &pluginv3.CallRouteRequest{}
			request.SetMethod(http.MethodGet)
			request.SetGroup(testGroup)
			request.SetVersion(testVersion)
			request.SetNamespace(tc.namespace)
			request.SetPath("/echo")
			request.SetUrl("/apis/" + testGroup + "/" + testVersion + "/namespaces/" + tc.namespace + "/echo")
			stream, err := served.BareRouteClient().CallRoute(metadata.AppendToOutgoingContext(ctx, "x-access-token", token), request)
			require.NoError(t, err)
			_, err = stream.Recv()
			assert.Equal(t, tc.code, status.Code(err))
			assert.Equal(t, tc.code == codes.OK, seen.calls.Load() == 1)
		})
	}
}
