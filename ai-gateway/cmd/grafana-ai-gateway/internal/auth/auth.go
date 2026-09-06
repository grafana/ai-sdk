package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
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

// Observation carries a closed outcome and an optional normalized caller.
type Observation struct {
	Outcome Outcome
	Caller  *Caller
}

// Caller is the private normalized authenticated caller retained in context.
type Caller struct {
	Service    string
	Namespace  string
	ActingUser *ActingUser
}

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
	if len(audiences) == 0 {
		return authn.VerifierConfig{}, fmt.Errorf("gateway auth: at least one audience is required")
	}
	seen := make(map[string]struct{}, len(audiences))
	for _, audience := range audiences {
		if strings.TrimSpace(audience) == "" {
			return authn.VerifierConfig{}, fmt.Errorf("gateway auth: audiences must not be empty")
		}
		if _, exists := seen[audience]; exists {
			return authn.VerifierConfig{}, fmt.Errorf("gateway auth: audiences must be unique")
		}
		seen[audience] = struct{}{}
	}
	return authn.VerifierConfig{AllowedAudiences: audiences}, nil
}

// Middleware authenticates a protected route before invoking next.
func Middleware(authenticator authn.Authenticator, errors *providerv4.HostErrorWriter, observe func(context.Context, Observation), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		provider, err := normalizeHeaders(request.Header)
		if err != nil {
			observe(request.Context(), Observation{Outcome: OutcomeFailed})
			errors.Write(w, providerv4.HostErrorAuthentication)
			return
		}
		info, err := authenticator.Authenticate(request.Context(), provider)
		if err != nil {
			observe(request.Context(), Observation{Outcome: OutcomeFailed})
			errors.Write(w, providerv4.HostErrorAuthentication)
			return
		}
		caller, err := callerFromAuthInfo(info)
		if err != nil {
			observe(request.Context(), Observation{Outcome: OutcomeFailed})
			errors.Write(w, providerv4.HostErrorAuthentication)
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
	access, err := exactlyOneHeader(headers, "X-Access-Token", true)
	if err != nil {
		return tokenProvider{}, err
	}
	id, err := exactlyOneHeader(headers, "X-Grafana-Id", false)
	if err != nil {
		return tokenProvider{}, err
	}
	normalizedAccess := stripBearer(access)
	normalizedID := stripBearer(id)
	if normalizedAccess == "" || (id != "" && normalizedID == "") {
		return tokenProvider{}, fmt.Errorf("gateway auth: bearer token is empty")
	}
	return tokenProvider{accessToken: normalizedAccess, idToken: normalizedID}, nil
}

func exactlyOneHeader(headers http.Header, name string, required bool) (string, error) {
	var values []string
	for key, candidates := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, candidates...)
		}
	}
	if len(values) == 0 && !required {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" || strings.Contains(values[0], ",") {
		return "", fmt.Errorf("gateway auth: invalid %s header", name)
	}
	return values[0], nil
}

func stripBearer(value string) string {
	return strings.TrimPrefix(value, "Bearer ")
}

func callerFromAuthInfo(info types.AuthInfo) (Caller, error) {
	identities := info.GetExtra()[authn.ServiceIdentityKey]
	if len(identities) != 1 || strings.TrimSpace(identities[0]) == "" {
		return Caller{}, fmt.Errorf("gateway auth: exactly one service identity is required")
	}
	namespace := info.GetNamespace()
	if strings.TrimSpace(namespace) == "" {
		return Caller{}, fmt.Errorf("gateway auth: namespace is required")
	}
	caller := Caller{Service: identities[0], Namespace: namespace}
	if identityType := info.GetIdentityType(); identityType != types.TypeAccessPolicy {
		caller.ActingUser = &ActingUser{Subject: info.GetSubject(), Type: identityType}
	}
	return caller, nil
}
