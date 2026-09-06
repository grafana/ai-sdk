package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHeaders(t *testing.T) {
	tests := []struct {
		name       string
		headers    http.Header
		access     string
		id         string
		shouldFail bool
	}{
		{name: "exact access", headers: http.Header{"X-Access-Token": {"access"}}, access: "access"},
		{name: "case variant", headers: http.Header{"x-access-token": {"access"}}, access: "access"},
		{name: "bearer stripping", headers: http.Header{"X-Access-Token": {"Bearer access"}, "X-Grafana-Id": {"Bearer id"}}, access: "access", id: "id"},
		{name: "authorization ignored with access", headers: http.Header{"Authorization": {"Bearer other"}, "X-Access-Token": {"access"}}, access: "access"},
		{name: "missing access", headers: http.Header{}, shouldFail: true},
		{name: "authorization only", headers: http.Header{"Authorization": {"Bearer access"}}, shouldFail: true},
		{name: "empty access", headers: http.Header{"X-Access-Token": {""}}, shouldFail: true},
		{name: "empty bearer access", headers: http.Header{"X-Access-Token": {"Bearer "}}, shouldFail: true},
		{name: "multiple access values", headers: http.Header{"X-Access-Token": {"one", "two"}}, shouldFail: true},
		{name: "case-colliding access", headers: http.Header{"X-Access-Token": {"one"}, "x-access-token": {"two"}}, shouldFail: true},
		{name: "coalesced access", headers: http.Header{"X-Access-Token": {"one,two"}}, shouldFail: true},
		{name: "empty id", headers: http.Header{"X-Access-Token": {"access"}, "X-Grafana-Id": {""}}, shouldFail: true},
		{name: "empty bearer id", headers: http.Header{"X-Access-Token": {"access"}, "X-Grafana-Id": {"Bearer "}}, shouldFail: true},
		{name: "multiple id values", headers: http.Header{"X-Access-Token": {"access"}, "X-Grafana-Id": {"one", "two"}}, shouldFail: true},
		{name: "case-colliding id", headers: http.Header{"X-Access-Token": {"access"}, "X-Grafana-Id": {"one"}, "x-grafana-id": {"two"}}, shouldFail: true},
		{name: "coalesced id", headers: http.Header{"X-Access-Token": {"access"}, "X-Grafana-Id": {"one,two"}}, shouldFail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := normalizeHeaders(tc.headers)
			if tc.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			access, ok := provider.AccessToken(context.Background())
			require.True(t, ok)
			assert.Equal(t, tc.access, access)
			id, ok := provider.IDToken(context.Background())
			assert.Equal(t, tc.id != "", ok)
			assert.Equal(t, tc.id, id)
		})
	}
}

func TestCallerFromAuthInfo_StrictIdentitySeparation(t *testing.T) {
	tests := []struct {
		name       string
		info       *fakeAuthInfo
		shouldFail bool
	}{
		{name: "service", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"service"}}, namespace: "stack-1", identityType: types.TypeAccessPolicy, subject: "acting-user"}},
		{name: "acting user separate", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"service"}}, namespace: "stack-1", identityType: types.TypeUser, subject: "user:42"}},
		{name: "missing service", info: &fakeAuthInfo{extra: map[string][]string{}, namespace: "stack-1", subject: "must-not-fallback"}, shouldFail: true},
		{name: "empty service", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {""}}, namespace: "stack-1", subject: "must-not-fallback"}, shouldFail: true},
		{name: "blank service", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"  "}}, namespace: "stack-1"}, shouldFail: true},
		{name: "multiple services", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"one", "two"}}, namespace: "stack-1"}, shouldFail: true},
		{name: "empty namespace", info: &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"service"}}}, shouldFail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := callerFromAuthInfo(tc.info)
			if tc.shouldFail {
				require.Error(t, err)
				assert.Empty(t, caller)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "service", caller.Service)
			assert.Equal(t, "stack-1", caller.Namespace)
			if tc.info.identityType == types.TypeUser {
				require.NotNil(t, caller.ActingUser)
				assert.Equal(t, "user:42", caller.ActingUser.Subject)
				assert.Equal(t, types.TypeUser, caller.ActingUser.Type)
			} else {
				assert.Nil(t, caller.ActingUser)
			}
		})
	}
}

func TestMiddleware_AuthenticationOrderingAndPrivateContext(t *testing.T) {
	errorWriter := providerv4.NewHostErrorWriter()

	t.Run("success retains only normalized caller", func(t *testing.T) {
		info := &fakeAuthInfo{
			extra:        map[string][]string{authn.ServiceIdentityKey: {"service"}, "id-token": {"raw-secret"}},
			namespace:    "stack-1",
			identityType: types.TypeUser,
			subject:      "user:42",
			email:        "private@example.com",
			groups:       []string{"private-group"},
			permissions:  []string{"private:permission"},
		}
		authenticator := &fakeAuthenticator{info: info}
		observations := 0
		nextCalls := 0
		var retained Caller
		next := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			nextCalls++
			var ok bool
			retained, ok = CallerFromContext(request.Context())
			require.True(t, ok)
		})
		handler, err := Middleware(authenticator, errorWriter, func(_ context.Context, observation Observation) {
			observations++
			assert.Equal(t, OutcomeAuthenticated, observation.Outcome)
			require.NotNil(t, observation.Caller)
			assert.Equal(t, "service", observation.Caller.Service)
		}, next)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("X-Access-Token", "Bearer access")
		request.Header.Set("X-Grafana-Id", "Bearer id")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		assert.Equal(t, 1, authenticator.calls)
		assert.Equal(t, "access", authenticator.accessToken)
		assert.Equal(t, "id", authenticator.idToken)
		assert.Equal(t, 1, observations)
		assert.Equal(t, 1, nextCalls)
		assert.Equal(t, Caller{Service: "service", Namespace: "stack-1", ActingUser: &ActingUser{Subject: "user:42", Type: types.TypeUser}}, retained)
		callerType := reflect.TypeOf(retained)
		assert.Equal(t, 3, callerType.NumField())
		assert.NotContains(t, assertableCaller(retained), "raw-secret")
		assert.NotContains(t, assertableCaller(retained), "private@example.com")
		assert.NotContains(t, assertableCaller(retained), "private-group")
		assert.NotContains(t, assertableCaller(retained), "private:permission")
	})

	for _, tc := range []struct {
		name          string
		headers       http.Header
		authenticator *fakeAuthenticator
	}{
		{name: "invalid headers", headers: http.Header{"Authorization": {"Bearer access"}}, authenticator: &fakeAuthenticator{info: validFakeAuthInfo()}},
		{name: "verifier failure", headers: http.Header{"X-Access-Token": {"access"}}, authenticator: &fakeAuthenticator{err: errors.New("private verifier detail")}},
		{name: "identity failure", headers: http.Header{"X-Access-Token": {"access"}}, authenticator: &fakeAuthenticator{info: &fakeAuthInfo{subject: "must-not-fallback", namespace: "stack-1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextCalls := 0
			observations := 0
			handler, err := Middleware(tc.authenticator, errorWriter, func(_ context.Context, observation Observation) {
				observations++
				assert.Equal(t, OutcomeFailed, observation.Outcome)
				assert.Nil(t, observation.Caller)
			}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalls++ }))
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header = tc.headers
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Equal(t, `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`, response.Body.String())
			assert.NotContains(t, response.Body.String(), "private")
			assert.Zero(t, nextCalls)
			assert.Equal(t, 1, observations)
		})
	}
}

func validFakeAuthInfo() *fakeAuthInfo {
	return &fakeAuthInfo{extra: map[string][]string{authn.ServiceIdentityKey: {"service"}}, namespace: "stack-1", identityType: types.TypeAccessPolicy}
}

type fakeAuthenticator struct {
	info        types.AuthInfo
	err         error
	calls       int
	accessToken string
	idToken     string
}

func (authenticator *fakeAuthenticator) Authenticate(ctx context.Context, provider authn.TokenProvider) (types.AuthInfo, error) {
	authenticator.calls++
	authenticator.accessToken, _ = provider.AccessToken(ctx)
	authenticator.idToken, _ = provider.IDToken(ctx)
	return authenticator.info, authenticator.err
}

type fakeAuthInfo struct {
	extra        map[string][]string
	namespace    string
	identityType types.IdentityType
	subject      string
	email        string
	groups       []string
	permissions  []string
}

func (info *fakeAuthInfo) GetUID() string                         { return info.subject }
func (info *fakeAuthInfo) GetIdentifier() string                  { return info.subject }
func (info *fakeAuthInfo) GetIdentityType() types.IdentityType    { return info.identityType }
func (info *fakeAuthInfo) GetNamespace() string                   { return info.namespace }
func (info *fakeAuthInfo) GetGroups() []string                    { return info.groups }
func (info *fakeAuthInfo) GetExtra() map[string][]string          { return info.extra }
func (info *fakeAuthInfo) GetSubject() string                     { return info.subject }
func (info *fakeAuthInfo) GetAudience() []string                  { return []string{"ai-sdk"} }
func (info *fakeAuthInfo) GetTokenPermissions() []string          { return info.permissions }
func (info *fakeAuthInfo) GetTokenDelegatedPermissions() []string { return nil }
func (info *fakeAuthInfo) GetName() string                        { return info.subject }
func (info *fakeAuthInfo) GetEmail() string                       { return info.email }
func (info *fakeAuthInfo) GetEmailVerified() bool                 { return false }
func (info *fakeAuthInfo) GetUsername() string                    { return "private-user" }
func (info *fakeAuthInfo) GetAuthenticatedBy() string             { return "private-auth" }
func (info *fakeAuthInfo) GetAccessToken() string                 { return "raw-access" }
func (info *fakeAuthInfo) GetIDToken() string                     { return "raw-id" }

func assertableCaller(caller Caller) string {
	return caller.Service + " " + caller.Namespace + " " + caller.ActingUser.Subject + " " + caller.ActingUser.Type.String()
}
