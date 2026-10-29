package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const (
	testNamespace       = "stacks-1"
	testInternalService = "internal-service"
	internalDocument    = `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`
	permissionDocument  = `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`
	byokDocument        = `{"error":{"message":"catalog discovery is unsupported for BYOK","type":"invalid_request_error","param":null,"code":"invalid_request"}}`
)

type authInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info authInfo) GetExtra() map[string][]string { return info.extra }

// withAccess serves next to a caller with the given account access. Any
// access other than configured or BYOK serves next without a verified caller.
func withAccess(access gatewayauth.AccountAccess, next http.Handler) http.Handler {
	if access != gatewayauth.ConfiguredAccounts && access != gatewayauth.RequestBYOK {
		return next
	}
	service, identity := testInternalService, ""
	if access == gatewayauth.RequestBYOK {
		service, identity = "front-door", "user:u1"
	}
	policy := gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeCloud, InternalServices: []string{testInternalService}}
	protected := gatewayauth.AccessMiddleware(policy, func(w http.ResponseWriter) { w.WriteHeader(http.StatusTeapot) }, func(context.Context, gatewayauth.Caller) {}, next)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var claims authn.Claims[authn.AccessTokenClaims]
		claims.Rest.Namespace = testNamespace
		if typ, uid, err := types.ParseTypeID(identity); err == nil {
			claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: typ, Identifier: uid}}
		}
		ctx := types.WithAuthInfo(request.Context(), authInfo{AuthInfo: authn.NewAccessTokenAuthInfo(claims), extra: map[string][]string{gatewayauth.DirectCallerServiceKey: {service}}})
		ctx = resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: testNamespace}})
		protected.ServeHTTP(w, request.WithContext(ctx))
	})
}

func serve(ctx context.Context, handler http.Handler, access gatewayauth.AccountAccess) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	withAccess(access, handler).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil).WithContext(ctx))
	return response
}

func newTestHandler(lister catalog.ModelLister) http.Handler {
	return New(lister, providerv4.DiscoveryProjection{}, providerv4.NewHostErrorWriter())
}

type testLister struct {
	models []catalog.ModelInfo
	err    error
	panic  bool
	calls  atomic.Int32
}

func (lister *testLister) ListModels(context.Context) ([]catalog.ModelInfo, error) {
	lister.calls.Add(1)
	if lister.panic {
		panic("private panic")
	}
	return lister.models, lister.err
}

type failingProjection struct{ panic bool }

func (projection failingProjection) ProjectDiscovery([]catalog.ModelInfo) ([]byte, error) {
	if projection.panic {
		panic("private panic")
	}
	return nil, errors.New("private projection failure")
}

type listedDocument struct {
	Models []struct {
		ID      string `json:"id"`
		Gateway struct {
			Aliases []string `json:"aliases"`
			Primary struct {
				ProviderModelID string `json:"providerModelId"`
			} `json:"primary"`
			Fallbacks []json.RawMessage `json:"fallbacks"`
		} `json:"gateway"`
	} `json:"models"`
}

func configuredInfo() catalog.ModelInfo {
	return catalog.ModelInfo{ID: "public", Name: "Model", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "instance", Provider: "anthropic", ModelID: "native"}}}
}

func TestHandler_ConfiguredCallerGetsTheProjection(t *testing.T) {
	models := []catalog.ModelInfo{configuredInfo(), {ID: "middle", Name: "Middle"}}
	response := serve(context.Background(), newTestHandler(&testLister{models: models}), gatewayauth.ConfiguredAccounts)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, http.Header{"Content-Type": {"application/json"}}, response.Header())
	expected, err := providerv4.DiscoveryProjection{}.ProjectDiscovery(models)
	require.NoError(t, err)
	assert.Equal(t, string(expected), response.Body.String())

	empty := serve(context.Background(), newTestHandler(&testLister{}), gatewayauth.ConfiguredAccounts)
	assert.Equal(t, http.StatusOK, empty.Code)
	assert.Equal(t, `{"models":[]}`, empty.Body.String())
}

func TestHandler_AccessDecision(t *testing.T) {
	for _, tc := range []struct {
		name     string
		access   gatewayauth.AccountAccess
		status   int
		document string
	}{
		{"BYOK", gatewayauth.RequestBYOK, http.StatusBadRequest, byokDocument},
		{"no account access", gatewayauth.Refused, http.StatusForbidden, permissionDocument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lister := &testLister{models: []catalog.ModelInfo{configuredInfo()}}
			response := serve(context.Background(), newTestHandler(lister), tc.access)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.document, response.Body.String())
			assert.Zero(t, lister.calls.Load())
		})
	}
}

func TestHandler_ListingAndProjectionFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		lister     *testLister
		projection host.DiscoveryProjection
	}{
		{"listing error", &testLister{err: errors.New("private backend listing failure")}, providerv4.DiscoveryProjection{}},
		{"listing panic", &testLister{panic: true}, providerv4.DiscoveryProjection{}},
		{"projection error", &testLister{}, failingProjection{}},
		{"projection panic", &testLister{}, failingProjection{panic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := New(tc.lister, tc.projection, providerv4.NewHostErrorWriter())
			response := serve(context.Background(), handler, gatewayauth.ConfiguredAccounts)
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, internalDocument, response.Body.String())
		})
	}
}

func TestHandler_ServesCompleteCatalogWithoutResponseLimit(t *testing.T) {
	info := configuredInfo()
	info.Candidates = nil
	for i := range 16 {
		info.Candidates = append(info.Candidates, catalog.ConfiguredCandidate{ProviderInstance: "instance", Provider: "anthropic", ModelID: fmt.Sprintf("%d-", i) + strings.Repeat(`"`, 512)})
	}
	infos := make([]catalog.ModelInfo, 65)
	for i := range infos {
		infos[i] = info
		infos[i].ID = fmt.Sprintf("public-%03d", i)
		infos[i].Aliases = []string{fmt.Sprintf("alias-%d", i)}
	}
	response := serve(context.Background(), newTestHandler(&testLister{models: infos}), gatewayauth.ConfiguredAccounts)
	require.Equal(t, http.StatusOK, response.Code)
	require.Greater(t, response.Body.Len(), 1<<20)
	var document listedDocument
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	require.Len(t, document.Models, 65)
	for i, row := range document.Models {
		assert.Equal(t, infos[i].ID, row.ID)
		assert.Len(t, row.Gateway.Fallbacks, 15)
		assert.Equal(t, info.Candidates[0].ModelID, row.Gateway.Primary.ProviderModelID)
		assert.Equal(t, infos[i].Aliases, row.Gateway.Aliases)
	}
}

type scopeKey struct{}

type scopedTestCatalog struct{ scopes map[string]catalog.Catalog }

func (c scopedTestCatalog) ListModels(ctx context.Context) ([]catalog.ModelInfo, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ListModels(ctx)
}
func (c scopedTestCatalog) ResolveModel(ctx context.Context, id string) (catalog.ResolvedModel, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ResolveModel(ctx, id)
}

type discoveryOnlyModel struct{ calls atomic.Int32 }

func (*discoveryOnlyModel) SpecificationVersion() string               { return "v4" }
func (*discoveryOnlyModel) Provider() string                           { return "response-provider-not-configured" }
func (*discoveryOnlyModel) ModelID() string                            { return "response-model-not-configured" }
func (*discoveryOnlyModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *discoveryOnlyModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}
func (m *discoveryOnlyModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}

func TestHandler_ScopedListingMatchesResolution(t *testing.T) {
	backend := &discoveryOnlyModel{}
	scoped := scopedTestCatalog{scopes: map[string]catalog.Catalog{}}
	for _, scope := range []string{"first", "second"} {
		created, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: scope, Name: "sk-ordinary-" + scope, Aliases: []string{scope + "-alias"}, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: scope + "-instance", Provider: "anthropic", ModelID: scope + "-native"}}}, Model: backend}})
		require.NoError(t, err)
		scoped.scopes[scope] = created
	}
	handler := newTestHandler(scoped)
	for _, scope := range []string{"first", "second", "first"} {
		ctx := context.WithValue(context.Background(), scopeKey{}, scope)
		response := serve(ctx, handler, gatewayauth.ConfiguredAccounts)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), scope+"-instance")
		assert.Contains(t, response.Body.String(), "sk-ordinary-"+scope)
		other := "first"
		if scope == other {
			other = "second"
		}
		assert.NotContains(t, response.Body.String(), other+"-instance")
		assert.NotContains(t, response.Body.String(), "response-model-not-configured")
		var document listedDocument
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
		require.Len(t, document.Models, 1)
		assert.Equal(t, scope, document.Models[0].ID)
		assert.Equal(t, []string{scope + "-alias"}, document.Models[0].Gateway.Aliases)
		resolved, err := scoped.ResolveModel(ctx, scope+"-alias")
		require.NoError(t, err)
		assert.Equal(t, scope, resolved.ID)
		_, err = scoped.ResolveModel(ctx, other)
		require.ErrorIs(t, err, catalog.ErrUnknownModel)
	}
	assert.Zero(t, backend.calls.Load())
}
