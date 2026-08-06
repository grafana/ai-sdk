package runtime

import "context"

type invocationContext struct {
	protocol                Protocol
	requestID               string
	requestedModelID        string
	canonicalModelID        string
	authenticatedAttributes map[string]string
	policyMetadata          PolicyMetadata
}

type invocationContextKey struct{}

func withInvocationContext(ctx context.Context, call GatewayCall, identity Identity) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, invocationContext{
		protocol:                call.Protocol,
		requestID:               call.CallMetadata.RequestID,
		requestedModelID:        identity.RequestedModelID,
		canonicalModelID:        identity.CanonicalModelID,
		authenticatedAttributes: cloneStringMap(call.CallMetadata.AuthenticatedAttributes),
		policyMetadata:          cloneRawMap(call.PolicyMetadata),
	})
}

func contextData(ctx context.Context) (invocationContext, bool) {
	value, ok := ctx.Value(invocationContextKey{}).(invocationContext)
	return value, ok
}

// ProtocolFromContext returns the originating gateway protocol.
func ProtocolFromContext(ctx context.Context) (Protocol, bool) {
	value, ok := contextData(ctx)
	return value.protocol, ok
}

// RequestIDFromContext returns the trusted gateway request ID.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	value, ok := contextData(ctx)
	return value.requestID, ok
}

// RequestedModelIDFromContext returns the caller-supplied public model ID.
func RequestedModelIDFromContext(ctx context.Context) (string, bool) {
	value, ok := contextData(ctx)
	return value.requestedModelID, ok
}

// CanonicalModelIDFromContext returns the canonical catalog model ID.
func CanonicalModelIDFromContext(ctx context.Context) (string, bool) {
	value, ok := contextData(ctx)
	return value.canonicalModelID, ok
}

// AuthenticatedAttributesFromContext returns a defensive copy of trusted host
// attributes.
func AuthenticatedAttributesFromContext(ctx context.Context) map[string]string {
	value, ok := contextData(ctx)
	if !ok {
		return nil
	}
	return cloneStringMap(value.authenticatedAttributes)
}

// PolicyMetadataFromContext returns a defensive copy of policy annotations.
func PolicyMetadataFromContext(ctx context.Context) PolicyMetadata {
	value, ok := contextData(ctx)
	if !ok {
		return nil
	}
	return cloneRawMap(value.policyMetadata)
}
