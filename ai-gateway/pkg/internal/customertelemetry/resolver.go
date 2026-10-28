package customertelemetry

import (
	"context"
	"time"
)

// DestinationResolver discovers validated destination snapshots for an authorized
// stack. Unavailable signals are absent; lookup failure is an error. Implementations
// must not activate products or infer recording permission from an endpoint.
// Implementations must honor context cancellation and bound transport work.
// Verified authoritative denial is ErrResolutionDenied; raw error text must not
// determine that classification. Cache supplies a context without caller values.
type DestinationResolver interface {
	ResolveDestinations(context.Context, Stack, time.Time) (Destinations, error)
}

// PolicyResolver resolves validated effective recording policy independently of
// destination and credential resolution. Failure must not become preference
// absence. Callers must check the returned stack and validity before use.
// Implementations must honor context cancellation and bound transport work.
// Verified authoritative denial is ErrResolutionDenied, not a transient failure.
type PolicyResolver interface {
	ResolvePolicy(context.Context, Stack, time.Time) (Policy, error)
}
