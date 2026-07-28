// Package agentobservability instruments AI SDK language models for Grafana
// Agent Observability. It records generations and evaluates preflight policy hooks
// while remaining provider-agnostic.
//
// The package is a nested Go module under middleware/agentobservability so the
// Agent Observability SDK, OpenTelemetry SDK, and gRPC dependencies remain opt-in.
// The upstream Go SDK is github.com/grafana/agento11y/go/agento11y. At the
// version pinned in this module's go.mod, that SDK emits agento11y.* telemetry
// attributes and reads AGENTO11Y_* environment variables.
//
// # Public API
//
// Two middleware values can be composed independently:
//
//   - [RecordingMiddleware] records unary and streaming generations.
//   - [HooksMiddleware] evaluates a preflight hook and can allow, deny, or
//     transform a provider call.
//
// [Stack] returns the canonical hooks-then-recording order. [Wrap] applies that
// stack to one model.
//
// [ClientResolver] selects the *agento11y.Client for each request. Returning nil
// makes both middleware paths no-ops. [ContextProvider] supplies approved user,
// metadata, tag, and agent fields.
//
// Context helpers express generation relationships:
//
//   - [WithGenerationID], [GenerationIDFromContext], [NewGenerationID]
//   - [WithParentGenerationIDs], [ParentGenerationIDsFromContext]
//   - [WithLinkedGenerationID]
//
// # OpenTelemetry
//
// The underlying SDK owns the canonical generation span, including its
// agento11y.generation.id attribute. This module opens only
// [SpanNameHooksPreflight] for hook evaluation. That span's aisdk.hooks.*
// attribute keys are ai-sdk's own; the SDK neither produces nor reads them. It
// also carries the gen_ai.provider.name and gen_ai.request.model
// semantic-convention attributes, which the SDK sets on its generation span too.
//
// # Example
//
//	import (
//		"github.com/grafana/ai-sdk/middleware/agentobservability"
//		"github.com/grafana/agento11y/go/agento11y"
//	)
//
//	model := agentobservability.Wrap(base, agentobservability.WrapOptions{
//		ClientResolver: func(ctx context.Context) *agento11y.Client {
//			return clientForRequest(ctx)
//		},
//		ContextProvider: contextInfoForRequest,
//		Hooks: agentobservability.HooksOptions{
//			Enabled:    hooksEnabled,
//			MaxLatency: 5 * time.Second,
//		},
//	})
package agentobservability
