package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"sync/atomic"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

const (
	requestCorrelationBytes = 16
	maxObservationValueLen  = 128
)

var correlationFallback atomic.Uint64

// requestObservation is the immutable, closed view shared by Gateway-owned
// logical observers. Values are copied into a new snapshot after successful
// authentication; a published snapshot is never mutated.
type requestObservation struct {
	correlationID string
	callerService string
	namespace     string
	region        string
	application   string
}

type modelObservationKey struct{}

func newRequestCorrelationID() string {
	value := make([]byte, requestCorrelationBytes)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return "fallback-" + strconv.FormatUint(correlationFallback.Add(1), 10)
}

func boundedObservationValue(value string) string {
	if value == "" || len(value) > maxObservationValueLen {
		return ""
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return ""
		}
	}
	return value
}

func observationFromContext(ctx context.Context) requestObservation {
	if ctx == nil {
		return requestObservation{}
	}
	if observation, ok := ctx.Value(modelObservationKey{}).(requestObservation); ok {
		return observation
	}
	state, ok := ctx.Value(telemetryStateKey{}).(*telemetryState)
	if !ok || state == nil {
		return requestObservation{}
	}
	observation := state.observation.Load()
	if observation == nil {
		return requestObservation{}
	}
	return *observation
}

// observationBridgeMiddleware snapshots the approved request observation at
// the logical model boundary. Gateway observers read this value copy instead
// of the authentication state; the provider's original context propagation is
// otherwise preserved.
func observationBridgeMiddleware() middleware.Middleware {
	withObservation := func(ctx context.Context) context.Context {
		return context.WithValue(ctx, modelObservationKey{}, observationFromContext(ctx))
	}
	return middleware.Middleware{
		WrapGenerate: func(ctx context.Context, params middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			return params.DoGenerate(withObservation(ctx))
		},
		WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
			return params.DoStream(withObservation(ctx))
		},
	}
}

func observationLogAttrs(ctx context.Context) []slog.Attr {
	observation := observationFromContext(ctx)
	attrs := make([]slog.Attr, 0, 5)
	for _, field := range []struct {
		key   string
		value string
	}{
		{key: "correlation_id", value: observation.correlationID},
		{key: "caller_service", value: observation.callerService},
		{key: "namespace", value: observation.namespace},
		{key: "region", value: observation.region},
		{key: "application", value: observation.application},
	} {
		if field.value != "" {
			attrs = append(attrs, slog.String(field.key, field.value))
		}
	}
	return attrs
}

// observationMetadata returns a fresh fixed-key map for the logical model
// observation bridge. Callers may mutate the result without changing context.
func observationMetadata(ctx context.Context) map[string]any {
	observation := observationFromContext(ctx)
	metadata := make(map[string]any, 5)
	for key, value := range map[string]string{
		"gateway.correlation_id": observation.correlationID,
		"gateway.caller_service": observation.callerService,
		"gateway.namespace":      observation.namespace,
		"gateway.region":         observation.region,
		"gateway.application":    observation.application,
	} {
		if value != "" {
			metadata[key] = value
		}
	}
	return metadata
}
