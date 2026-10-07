package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
)

const unsafeAuthenticationWarning = "UNSAFE DEVELOPMENT AUTHENTICATION IS ENABLED"

// Outcome is a closed authentication telemetry outcome.
type Outcome uint8

const (
	// OutcomeAuthenticated reports successful verified authentication.
	OutcomeAuthenticated Outcome = iota + 1
	// OutcomeFailed reports a rejected authentication attempt.
	OutcomeFailed
)

// Observation contains an authentication result and normalized caller, when available.
type Observation struct {
	Source  Source
	Outcome Outcome
	Caller  *Caller
}

// Source identifies the configured authentication mode.
type Source string

const (
	// SourceAccessToken identifies JWT authentication.
	SourceAccessToken Source = "access-token"
	// SourceCloudGateway identifies authentication by trusted proxy assertions.
	SourceCloudGateway Source = "cloud-gateway"
)

type AccountAccess uint8

const (
	ConfiguredAccounts AccountAccess = iota + 1
	RequestBYOK
)

// Caller is the private normalized authenticated caller retained in context.
type Caller struct {
	Source     Source
	Service    string
	Subject    string
	Namespace  string
	ActingUser *ActingUser
	stackID    int64
	access     AccountAccess
}

func (caller Caller) AccountAccess() AccountAccess { return caller.access }

// ActingUser is an optional verified non-access-policy identity.
type ActingUser struct {
	Subject string
	Type    types.IdentityType
}

type callerContextKey struct{}

type tokenProvider struct {
	accessToken string
	idToken     string
}

func (provider tokenProvider) AccessToken(context.Context) (string, bool) {
	return provider.accessToken, provider.accessToken != ""
}

func (provider tokenProvider) IDToken(context.Context) (string, bool) {
	return provider.idToken, provider.idToken != ""
}

// NewAuthenticator constructs access and ID token verification over one key retriever.
func NewAuthenticator(keys authn.KeyRetriever, audiences []string) (authn.Authenticator, error) {
	verifierConfig, err := newVerifierConfig(audiences)
	if err != nil {
		return nil, err
	}
	return authn.NewDefaultAuthenticator(
		authn.NewAccessTokenVerifier(verifierConfig, keys),
		authn.NewIDTokenVerifier(authn.VerifierConfig{}, keys),
	), nil
}

// NewUnsafeAuthenticator constructs development-only access and ID token verification.
func NewUnsafeAuthenticator(audiences []string, warn func(string)) (authn.Authenticator, error) {
	verifierConfig, err := newVerifierConfig(audiences)
	if err != nil {
		return nil, err
	}
	warn(unsafeAuthenticationWarning)
	return authn.NewDefaultAuthenticator(
		authn.NewUnsafeAccessTokenVerifier(verifierConfig),
		authn.NewUnsafeIDTokenVerifier(authn.VerifierConfig{}),
	), nil
}

func newVerifierConfig(audiences []string) (authn.VerifierConfig, error) {
	if len(audiences) != 1 || audiences[0] != "ai-sdk" {
		return authn.VerifierConfig{}, fmt.Errorf("gateway auth: audience must be ai-sdk")
	}
	return authn.VerifierConfig{AllowedAudiences: []string{"ai-sdk"}}, nil
}

// RequestAuthenticator authenticates headers without access to the request body.
type RequestAuthenticator interface {
	Authenticate(context.Context, http.Header) (Caller, error)
}

type accessTokenAuthenticator struct {
	verifier authn.Authenticator
}

// NewAccessTokenAuthenticator adapts JWT verification to RequestAuthenticator.
func NewAccessTokenAuthenticator(verifier authn.Authenticator) RequestAuthenticator {
	return accessTokenAuthenticator{verifier: verifier}
}

func (authenticator accessTokenAuthenticator) Authenticate(ctx context.Context, headers http.Header) (Caller, error) {
	provider, err := normalizeHeaders(headers)
	if err != nil {
		return Caller{}, err
	}
	info, err := authenticator.verifier.Authenticate(ctx, provider)
	if err != nil {
		return Caller{}, err
	}
	return callerFromAuthInfo(info)
}

// Middleware authenticates a protected route before invoking next.
func Middleware(authenticator RequestAuthenticator, writeFailure func(http.ResponseWriter), observe func(context.Context, Observation), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		caller, err := authenticator.Authenticate(request.Context(), request.Header)
		if err != nil {
			observe(request.Context(), Observation{Outcome: OutcomeFailed})
			writeFailure(w)
			return
		}
		observe(request.Context(), Observation{Outcome: OutcomeAuthenticated, Caller: &caller})
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), callerContextKey{}, caller)))
	})
}

// CallerFromContext returns the normalized caller without retaining authlib state.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok
}

func normalizeHeaders(headers http.Header) (tokenProvider, error) {
	for name := range headers {
		if strings.EqualFold(name, "X-Scope-OrgID") || strings.EqualFold(name, "X-Cloud-Org-ID") || strings.EqualFold(name, "X-Access-Policy-ID") {
			return tokenProvider{}, fmt.Errorf("gateway auth: private request contains a cloud assertion")
		}
	}
	access, err := exactlyOneHeader(headers, "X-Access-Token", false)
	if err != nil {
		return tokenProvider{}, err
	}
	authorization, err := exactlyOneHeader(headers, "Authorization", false)
	if err != nil {
		return tokenProvider{}, err
	}
	if (access == "") == (authorization == "") {
		return tokenProvider{}, fmt.Errorf("gateway auth: exactly one access credential is required")
	}
	if authorization != "" {
		scheme, token, ok := strings.Cut(authorization, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return tokenProvider{}, fmt.Errorf("gateway auth: authorization must use bearer")
		}
		access = token
	}
	id, err := exactlyOneHeader(headers, "X-Grafana-Id", false)
	if err != nil {
		return tokenProvider{}, err
	}
	normalizedAccess := access
	if authorization == "" {
		normalizedAccess = stripBearer(access)
	}
	normalizedID := stripBearer(id)
	if !validTokenHeader(normalizedAccess) || (id != "" && !validTokenHeader(normalizedID)) {
		return tokenProvider{}, fmt.Errorf("gateway auth: bearer token is empty")
	}
	return tokenProvider{accessToken: normalizedAccess, idToken: normalizedID}, nil
}

func validTokenHeader(value string) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func exactlyOneHeader(headers http.Header, name string, required bool) (string, error) {
	var values []string
	matches := 0
	for key, candidates := range headers {
		if strings.EqualFold(key, name) {
			matches++
			values = append(values, candidates...)
		}
	}
	if matches == 0 && !required {
		return "", nil
	}
	if matches != 1 || len(values) != 1 || values[0] == "" || strings.Contains(values[0], ",") {
		return "", fmt.Errorf("gateway auth: invalid %s header", name)
	}
	return values[0], nil
}

func stripBearer(value string) string {
	return strings.TrimPrefix(value, "Bearer ")
}

func callerFromAuthInfo(info types.AuthInfo) (Caller, error) {
	identities := info.GetExtra()[authn.ServiceIdentityKey]
	if len(identities) > 1 {
		return Caller{}, fmt.Errorf("gateway auth: ambiguous service identity")
	}
	var service string
	if len(identities) == 1 {
		service = identities[0]
		if service != "" && strings.TrimSpace(service) == "" {
			return Caller{}, fmt.Errorf("gateway auth: invalid service identity")
		}
	}
	namespace := info.GetNamespace()
	if namespace != "*" {
		stack, ok := strings.CutPrefix(namespace, "stacks-")
		if !ok {
			return Caller{}, fmt.Errorf("gateway auth: unsupported namespace")
		}
		if _, err := positiveDecimal(stack); err != nil {
			return Caller{}, err
		}
	}
	caller := Caller{Source: SourceAccessToken, Service: service, Subject: info.GetSubject(), Namespace: namespace, access: ConfiguredAccounts}
	if identityType := info.GetIdentityType(); identityType != types.TypeAccessPolicy {
		if namespace == "*" {
			return Caller{}, fmt.Errorf("gateway auth: acting user requires a concrete namespace")
		}
		caller.ActingUser = &ActingUser{Subject: info.GetSubject(), Type: identityType}
	}
	return caller, nil
}
