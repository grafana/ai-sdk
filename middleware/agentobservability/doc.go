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
//   - [HooksMiddleware] evaluates a preflight hook and can allow or deny a
//     provider call. Non-empty transforms fail with [ErrHookTransformFailed]
//     because agento11y v0.19 discards malformed response parts before validation.
//
// [Stack] returns the canonical hooks-then-recording order. [Wrap] applies that
// stack to one model.
//
// [ClientResolver] selects the *agento11y.Client for each request. Returning nil
// makes both middleware paths no-ops. [ContextProvider] supplies approved user,
// metadata, tag, and agent fields.
//
// [RecordingOptions] can preserve requested model identity and opt into finite
// cancellation cleanup. Requested identity does not filter caller metadata or
// tags. Metadata-only client capture still retains non-content fields such as
// raw stop reasons and ambient Agent Observability identity/context; consumers
// that require an allowlist must establish their own observation boundary.
//
// Context helpers express generation relationships:
//
//   - [WithGenerationID], [GenerationIDFromContext], [NewGenerationID]
//   - [WithParentGenerationIDs], [ParentGenerationIDsFromContext]
//   - [WithLinkedGenerationID]
//
// # OpenTelemetry
//
// The resolved agento11y client selects the generation export protocol. With
// gRPC or HTTP it emits a generateText or streamText metadata span. With
// GenerationExportProtocolOTel and EnableExperimentalFeatures it emits one
// chat <model> generation span, marked with agento11y.record="true". Without
// EnableExperimentalFeatures the client exports no generation. Recording does
// not add a second span.
//
// Applications configure the client's TracerProvider, sampling, destination,
// content capture, and Flusher outside the middleware. Recording does not
// flush per request. See docs/middleware/agent-observability.md for flush and
// shutdown order.
//
// This module opens only [SpanNameHooksPreflight] for hook evaluation. Its
// aisdk.hooks.* keys belong to ai-sdk; the client neither produces nor reads
// them. Its gen_ai.provider.name values match the OTel generation span, and
// gen_ai.request.model is the wrapped model ID.
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
