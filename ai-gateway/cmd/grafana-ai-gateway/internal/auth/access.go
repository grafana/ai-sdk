package auth

import (
	"context"
	"errors"
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

// AccountAccess is the closed outcome of the access check: the account
// population a request may use, or refusal.
type AccountAccess string

const (
	// ConfiguredAccounts allows configured accounts and the catalog.
	ConfiguredAccounts AccountAccess = "configured"
	// RequestBYOK allows only request-supplied provider credentials.
	RequestBYOK AccountAccess = "byok"
	// Refused allows no account access.
	Refused AccountAccess = "refused"
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
	for entry := range strings.SplitSeq(internalServices, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return AccessPolicy{}, errors.New("config: internal service entries must not be empty")
		}
		if slices.Contains(policy.InternalServices, entry) {
			return AccessPolicy{}, errors.New("config: internal service entries must be unique")
		}
		policy.InternalServices = append(policy.InternalServices, entry)
	}
	return policy, nil
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

// Decide maps the verified caller and route namespace to account access. A
// refused caller carries no other values.
func (policy AccessPolicy) Decide(info types.AuthInfo, namespace string) Caller {
	refused := Caller{access: Refused}
	if info == nil || namespace == "" {
		return refused
	}
	caller := Caller{Namespace: namespace, Service: single(info.GetExtra()[DirectCallerServiceKey]), Identity: identityActor(info)}
	switch {
	case policy.Mode == AccessModeCloud && caller.Service != "" && slices.Contains(policy.InternalServices, caller.Service):
		caller.access, caller.attributed = ConfiguredAccounts, true
	case caller.Identity == "":
		return refused
	case policy.Mode == AccessModeCloud:
		caller.access = RequestBYOK
	case policy.Mode == AccessModeSelfHosted:
		caller.access = ConfiguredAccounts
	default:
		return refused
	}
	return caller
}

// identityActor returns the typed UID of the innermost actor when it is a
// user or service account.
func identityActor(info types.AuthInfo) string {
	if !types.IsIdentityType(info.GetIdentityType(), types.TypeUser, types.TypeServiceAccount) || info.GetIdentifier() == "" {
		return ""
	}
	return info.GetUID()
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
func AccessMiddleware(policy AccessPolicy, refuse func(http.ResponseWriter), observe func(context.Context, Caller), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		info, _ := types.AuthInfoFrom(request.Context())
		var namespace string
		if route := resource.RouteRequestInfoFrom(request.Context()); route != nil {
			namespace = route.Namespace
		}
		caller := policy.Decide(info, namespace)
		observe(request.Context(), caller)
		if caller.AccountAccess() == Refused {
			refuse(w)
			return
		}
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), callerContextKey{}, caller)))
	})
}

// CallerFromContext returns the verified caller.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok
}
