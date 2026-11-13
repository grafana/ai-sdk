package service

import (
	"context"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
)

// ExtensionRequest returns the request information extensions receive for a
// call: the route namespace and caller type of the verified caller, and the
// call's correlation ID.
func ExtensionRequest(ctx context.Context) extension.Call {
	call := extension.Call{CorrelationID: observationCorrelationID(ctx)}
	if caller, ok := gatewayauth.CallerFromContext(ctx); ok {
		call.Namespace, call.Caller = caller.Namespace, caller.Type()
	}
	return call
}
