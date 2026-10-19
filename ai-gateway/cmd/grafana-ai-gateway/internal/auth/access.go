package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/grafana/authlib/types"

	"github.com/grafana/grafana-app-sdk/resource"
)

// AccessMode selects how verified callers map to account access.
type AccessMode string

const (
	// AccessModeCloud grants the catalog to allowlisted internal services and
	// request-scoped BYOK to other users and service accounts.
	AccessModeCloud AccessMode = "cloud"
	// AccessModeSelfHosted grants the catalog to users and service accounts.
	AccessModeSelfHosted AccessMode = "self-hosted"
)

const (
	maxInternalServices    = 64
	maxServiceIdentityByte = 128
)

// AccountAccess is the account population a request may use.
type AccountAccess uint8

const (
	// ConfiguredAccounts allows configured accounts and the catalog.
	ConfiguredAccounts AccountAccess = iota + 1
	// RequestBYOK allows only request-supplied provider credentials.
	RequestBYOK
)

// Decision is the closed outcome of the access check.
type Decision string

const (
	DecisionConfigured Decision = "configured"
	DecisionBYOK       Decision = "byok"
	DecisionRefused    Decision = "refused"
)

// AccessPolicy maps verified callers to account access.
type AccessPolicy struct {
	Mode             AccessMode
	InternalServices []string
}

// ParseAccessPolicy validates the access mode and the comma-separated
// internal service allowlist, which only cloud mode accepts.
func ParseAccessPolicy(mode, internalServices string) (AccessPolicy, error) {
	policy := AccessPolicy{Mode: AccessMode(mode)}
	switch policy.Mode {
	case AccessModeCloud, AccessModeSelfHosted:
	case "":
		return AccessPolicy{}, errors.New("config: access mode is required and must be cloud or self-hosted")
	default:
		return AccessPolicy{}, errors.New("config: access mode must be cloud or self-hosted")
	}
	if internalServices == "" {
		return policy, nil
	}
	if policy.Mode != AccessModeCloud {
		return AccessPolicy{}, errors.New("config: the internal service allowlist requires cloud access mode")
	}
	entries := strings.Split(internalServices, ",")
	if len(entries) > maxInternalServices {
		return AccessPolicy{}, fmt.Errorf("config: the internal service allowlist accepts at most %d entries", maxInternalServices)
	}
	for _, entry := range entries {
		if !validServiceIdentity(entry) {
			return AccessPolicy{}, errors.New("config: internal service entries must be 1-128 visible ASCII characters without commas")
		}
		if slices.Contains(policy.InternalServices, entry) {
			return AccessPolicy{}, errors.New("config: internal service entries must be unique")
		}
		policy.InternalServices = append(policy.InternalServices, entry)
	}
	return policy, nil
}

func validServiceIdentity(value string) bool {
	if value == "" || len(value) > maxServiceIdentityByte {
		return false
	}
	for i := range len(value) {
		if value[i] < 0x21 || value[i] > 0x7e || value[i] == ',' {
			return false
		}
	}
	return true
}

// Caller is the private verified caller retained in the request context.
type Caller struct {
	// Namespace is the route namespace the platform authorized.
	Namespace string
	// Service is the direct caller's signed service identity, when present.
	Service string
	// Identity is the identity actor's typed UID, when present. It is never emitted.
	Identity   string
	access     AccountAccess
	attributed bool
}

// AccountAccess returns the account population the caller may use.
func (caller Caller) AccountAccess() AccountAccess { return caller.access }

// AttributedService returns the service identity that observations may
// attribute, which is set only for configured access in cloud mode.
func (caller Caller) AttributedService() string {
	if caller.attributed {
		return caller.Service
	}
	return ""
}

// Decide maps the verified caller and route namespace to account access.
func (policy AccessPolicy) Decide(info types.AuthInfo, namespace string) (Caller, Decision) {
	if info == nil || namespace == "" {
		return Caller{}, DecisionRefused
	}
	extra := info.GetExtra()
	caller := Caller{Namespace: namespace, Service: single(extra[DirectCallerServiceKey]), Identity: single(extra[IdentityActorKey])}
	switch {
	case policy.Mode == AccessModeCloud && caller.Service != "" && slices.Contains(policy.InternalServices, caller.Service):
		caller.access, caller.attributed = ConfiguredAccounts, true
		return caller, DecisionConfigured
	case caller.Identity == "":
		return Caller{}, DecisionRefused
	case policy.Mode == AccessModeCloud:
		caller.access = RequestBYOK
		return caller, DecisionBYOK
	case policy.Mode == AccessModeSelfHosted:
		caller.access = ConfiguredAccounts
		return caller, DecisionConfigured
	default:
		return Caller{}, DecisionRefused
	}
}

func single(values []string) string {
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

type callerContextKey struct{}

// AccessMiddleware derives the caller's account access from the verified
// identity and route namespace before invoking next. Refused requests are
// answered by refuse and never reach next.
func AccessMiddleware(policy AccessPolicy, refuse func(http.ResponseWriter), observe func(context.Context, Decision, *Caller), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		info, _ := types.AuthInfoFrom(request.Context())
		var namespace string
		if route := resource.RouteRequestInfoFrom(request.Context()); route != nil {
			namespace = route.Namespace
		}
		caller, decision := policy.Decide(info, namespace)
		if decision == DecisionRefused {
			observe(request.Context(), decision, nil)
			refuse(w)
			return
		}
		observe(request.Context(), decision, &caller)
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), callerContextKey{}, caller)))
	})
}

// CallerFromContext returns the verified caller.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok
}
