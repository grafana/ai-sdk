package customertelemetry

import (
	"context"
	"time"
)

// DestinationResolver discovers validated destination snapshots for an authorized
// stack. Unavailable signals are absent; lookup failure is an error. Implementations
// must not activate products or infer recording permission from an endpoint.
type DestinationResolver interface {
	ResolveDestinations(context.Context, Stack, time.Time) (Destinations, error)
}

// PolicyResolver resolves validated effective recording policy independently of
// destination and credential resolution. Failure must not become preference
// absence. Callers must check the returned stack and validity before use.
type PolicyResolver interface {
	ResolvePolicy(context.Context, Stack, time.Time) (Policy, error)
}
