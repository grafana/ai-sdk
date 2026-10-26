package platformhost

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"go.opentelemetry.io/otel/propagation"

	clientv3 "github.com/grafana/grafana-app-sdk/plugin/client/v3"
	"github.com/grafana/grafana-app-sdk/plugin/httpadapter"
)

// Topology selects the platform shape the host imitates.
type Topology string

const (
	// TopologyCloud imitates Grafana Cloud: a front door that authenticates
	// sessions and service account tokens, and a router that delegates every
	// caller's access token to the plugin.
	TopologyCloud Topology = "cloud"
	// TopologySelfHosted imitates a self-hosted Grafana that exchanges the
	// caller's ID token for a token delegated to the plugin.
	TopologySelfHosted Topology = "self-hosted"
)

const (
	// RouterService is the service identity of the Cloud router hop.
	RouterService = "router"
	// FrontDoorService is the service identity of the Cloud front door hop.
	FrontDoorService = "front-door"
	// GrafanaService is the service identity of a self-hosted Grafana.
	GrafanaService = "grafana"
)

const (
	sessionCookie      = "grafana_session"
	serviceTokenPrefix = "glsa_"
)

// Config describes the plugin the host serves.
type Config struct {
	Topology Topology
	PluginID string
	Group    string
	Version  string
	// RouteDeadline, when positive, bounds each forwarded route call, as a
	// platform route deadline does.
	RouteDeadline time.Duration
}

type session struct {
	identityType types.IdentityType
	uid          string
	namespace    string
}

// Host is an in-memory plugin host: it issues caller credentials, exchanges
// them for delegated tokens and forwards HTTP requests to plugin routes.
type Host struct {
	config Config
	signer *signer

	mu              sync.Mutex
	serviceAccounts map[string]session
	sessions        map[string]session
	accessPolicies  map[string]string
}

// New returns a host for config.
func New(config Config) (*Host, error) {
	if config.PluginID == "" || config.Group == "" || config.Version == "" {
		return nil, errors.New("platformhost: plugin ID, group and version are required")
	}
	if config.Topology != TopologyCloud && config.Topology != TopologySelfHosted {
		return nil, errors.New("platformhost: unknown topology")
	}
	s, err := newSigner()
	if err != nil {
		return nil, err
	}
	return &Host{config: config, signer: s, serviceAccounts: map[string]session{}, sessions: map[string]session{}, accessPolicies: map[string]string{}}, nil
}

// Config returns the host configuration.
func (h *Host) Config() Config { return h.config }

// JWKS returns the public signing keys as a JSON Web Key Set.
func (h *Host) JWKS() ([]byte, error) { return h.signer.jwks() }

// Keys returns the public signing keys as an authlib key retriever.
func (h *Host) Keys() authn.KeyRetriever { return h.signer }

// Authenticator returns authlib's access token authenticator for delegated
// tokens addressed to the plugin ID or API group.
func (h *Host) Authenticator() authn.Authenticator {
	return authn.NewAccessTokenAuthenticator(authn.NewAccessTokenVerifier(authn.VerifierConfig{
		AllowedAudiences: []string{h.config.PluginID, h.config.Group},
	}, h.signer))
}

// ServiceToken returns an access token for an internal service, as the
// service would obtain from the platform token exchange.
func (h *Host) ServiceToken(service, namespace string) (string, error) {
	return h.signer.accessToken(typedUID(types.TypeAccessPolicy, service), []string{h.config.PluginID}, authn.AccessTokenClaims{
		Namespace:       namespace,
		ServiceIdentity: service,
	})
}

// IDToken returns a Grafana ID token for a user or service account.
func (h *Host) IDToken(identityType types.IdentityType, uid, namespace string) (string, error) {
	return h.signer.idToken(identityType, uid, namespace)
}

// Exchange signs a token for service that carries subjectToken, an ID or
// access token, as its actor. It imitates the platform token exchange used
// for on-behalf-of calls, relays and every platform hop.
func (h *Host) Exchange(ctx context.Context, service string, request authn.TokenExchangeRequest) (string, error) {
	return h.signer.exchange(ctx, service, request)
}

// AddServiceAccount registers a Grafana service account and returns the token
// clients send as an Authorization bearer.
func (h *Host) AddServiceAccount(uid, namespace string) string {
	token := serviceTokenPrefix + rand.Text()
	h.RegisterServiceAccount(token, uid, namespace)
	return token
}

// RegisterServiceAccount registers a Grafana service account under a fixed token.
func (h *Host) RegisterServiceAccount(token, uid, namespace string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serviceAccounts[token] = session{identityType: types.TypeServiceAccount, uid: uid, namespace: namespace}
}

// AddAccessPolicy registers an access policy token for an internal service
// and returns the token the service presents to TokenExchangeHandler.
func (h *Host) AddAccessPolicy(service string) string {
	token := rand.Text()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accessPolicies[token] = service
	return token
}

// TokenExchangeHandler serves the platform token exchange for access policy
// tokens from AddAccessPolicy: it signs an access token for the service, or
// an on-behalf-of token when the request carries a subject token. Mount it
// on a POST route.
func (h *Host) TokenExchangeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		h.mu.Lock()
		service, known := h.accessPolicies[policy]
		h.mu.Unlock()
		if !ok || !known {
			http.Error(w, `{"status":"error"}`, http.StatusUnauthorized)
			return
		}
		var request authn.TokenExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Namespace == "" {
			http.Error(w, `{"status":"error"}`, http.StatusBadRequest)
			return
		}
		var token string
		var err error
		if request.SubjectToken == "" {
			token, err = h.signer.accessToken(typedUID(types.TypeAccessPolicy, service), request.Audiences, authn.AccessTokenClaims{Namespace: request.Namespace, ServiceIdentity: service})
		} else {
			token, err = h.Exchange(r.Context(), service, request)
		}
		if err != nil {
			http.Error(w, `{"status":"error"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{"token": token}})
	})
}

// AddSession registers a signed-in Grafana identity, such as a user or an
// anonymous viewer, and returns its session cookie value.
func (h *Host) AddSession(identityType types.IdentityType, uid, namespace string) string {
	token := rand.Text()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[token] = session{identityType: identityType, uid: uid, namespace: namespace}
	return token
}

// SessionCookie returns the cookie that carries a session from AddSession.
func SessionCookie(value string) *http.Cookie {
	return &http.Cookie{Name: sessionCookie, Value: value}
}

// Exchanger returns the token exchanger the plugin host uses to delegate
// callers to the plugin.
func (h *Host) Exchanger() authn.TokenExchanger { return hostExchanger{h} }

type hostExchanger struct{ host *Host }

func (e hostExchanger) Exchange(ctx context.Context, request authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
	token, err := e.host.Exchange(ctx, e.host.hostService(), request)
	if err != nil {
		return nil, err
	}
	return &authn.TokenExchangeResponse{Token: token}, nil
}

func (h *Host) hostService() string {
	if h.config.Topology == TopologySelfHosted {
		return GrafanaService
	}
	return RouterService
}

// Handler serves platform API requests under /apis/<group>/<version>/ by
// authenticating the caller and forwarding the request through client, which
// must delegate callers with Exchanger, as grpcplugin.NewClientV3 does.
func (h *Host) Handler(client clientv3.RouteClient) http.Handler {
	forward := httpadapter.HandlerFunc(client)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := h.routeInfo(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		caller, err := h.authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !types.NamespaceMatches(caller.GetNamespace(), info.Namespace) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx = httpadapter.WithRouteInfo(types.WithAuthInfo(ctx, caller), info)
		if h.config.RouteDeadline > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, h.config.RouteDeadline)
			defer cancel()
		}
		forward(w, r.WithContext(ctx))
	})
}

func (h *Host) routeInfo(path string) (httpadapter.RouteInfo, bool) {
	prefix := "/apis/" + h.config.Group + "/" + h.config.Version + "/namespaces/"
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return httpadapter.RouteInfo{}, false
	}
	namespace, route, ok := strings.Cut(rest, "/")
	if !ok || namespace == "" || route == "" {
		return httpadapter.RouteInfo{}, false
	}
	return httpadapter.RouteInfo{Group: h.config.Group, Version: h.config.Version, Namespace: namespace, Path: "/" + route}, true
}

type caller struct {
	types.AuthInfo
	accessToken string
}

func (c caller) GetAccessToken() string { return c.accessToken }

func (h *Host) authenticate(r *http.Request) (caller, error) {
	accessTokens := r.Header.Values("X-Access-Token")
	authorization := r.Header.Values("Authorization")
	cookie, cookieErr := r.Cookie(sessionCookie)
	credentials := len(accessTokens) + len(authorization)
	if cookieErr == nil {
		credentials++
	}
	if credentials != 1 {
		return caller{}, errors.New("platformhost: exactly one credential is required")
	}
	switch {
	case len(accessTokens) == 1:
		claims, err := h.signer.access.Verify(r.Context(), accessTokens[0])
		if err != nil {
			return caller{}, err
		}
		return caller{AuthInfo: authn.NewAccessTokenAuthInfo(*claims), accessToken: accessTokens[0]}, nil
	case len(authorization) == 1:
		token, ok := strings.CutPrefix(authorization[0], "Bearer ")
		if !ok {
			return caller{}, errors.New("platformhost: bearer credential is required")
		}
		h.mu.Lock()
		account, ok := h.serviceAccounts[token]
		h.mu.Unlock()
		if !ok {
			return caller{}, errors.New("platformhost: unknown service account token")
		}
		return h.signedIn(r.Context(), account)
	default:
		h.mu.Lock()
		signedIn, ok := h.sessions[cookie.Value]
		h.mu.Unlock()
		if !ok {
			return caller{}, errors.New("platformhost: unknown session")
		}
		return h.signedIn(r.Context(), signedIn)
	}
}

func (h *Host) signedIn(ctx context.Context, s session) (caller, error) {
	idToken, err := h.signer.idToken(s.identityType, s.uid, s.namespace)
	if err != nil {
		return caller{}, err
	}
	if h.config.Topology == TopologySelfHosted {
		claims, err := h.signer.id.Verify(ctx, idToken)
		if err != nil {
			return caller{}, err
		}
		return caller{AuthInfo: authn.NewIDTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{}, claims)}, nil
	}
	accessToken, err := h.Exchange(ctx, FrontDoorService, authn.TokenExchangeRequest{Namespace: s.namespace, SubjectToken: idToken})
	if err != nil {
		return caller{}, err
	}
	claims, err := h.signer.access.Verify(ctx, accessToken)
	if err != nil {
		return caller{}, err
	}
	return caller{AuthInfo: authn.NewAccessTokenAuthInfo(*claims), accessToken: accessToken}, nil
}
