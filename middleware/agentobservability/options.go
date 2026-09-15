package agentobservability

import (
	"context"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
)

// ClientResolver resolves the *agento11y.Client to use for a given request
// context. Most consumers route per-tenant; some return a single process-wide
// client.
//
// A nil return value SHALL make the middleware a no-op for that request: the
// inner model is invoked unchanged, no Generation is started, no EvaluateHook
// is called.
type ClientResolver = func(ctx context.Context) *agento11y.Client

// ContextProvider returns the per-request ContextInfo. The middleware
// tolerates a zero return value; every field falls back to the corresponding
// agento11y context helper or remains unset.
type ContextProvider = func(ctx context.Context) ContextInfo

// RecordErrorHandler receives a local validation or enqueue failure after a
// generation recorder is finalized. It must not return an error because
// observation failures never change the model call result.
type RecordErrorHandler = func(err error)

// RecordCompleteHandler is called once after a generation recorder has ended,
// including local validation or enqueue failure. Handler panics are recovered.
type RecordCompleteHandler = func()

// GenerationFilterInput carries the mapped generation plus the normalized
// provider finish reason observed for the call.
type GenerationFilterInput struct {
	Generation   agento11y.Generation
	FinishReason provider.FinishReason
}

// GenerationFilter applies a consumer-owned allowlist immediately before a
// mapped generation is handed to the Agent Observability client. It must not
// mutate values retained by the provider result.
type GenerationFilter = func(GenerationFilterInput) agento11y.Generation

// ContextSource selects whether recording may inherit Agent Observability
// values from the request context.
type ContextSource string

const (
	// ContextAmbientFallback preserves direct-consumer compatibility by using
	// explicit ContextInfo first and Agent Observability context values second.
	ContextAmbientFallback ContextSource = "ambient_fallback"
	// ContextProvidedOnly allows only ContextInfo plus the active OpenTelemetry
	// parent span into recording. The original context values and cancellation
	// still reach the wrapped model.
	ContextProvidedOnly ContextSource = "provided_only"
)

// ContextInfo carries consumer-derived fields that get attached to every
// Generation. None are required.
type ContextInfo struct {
	// UserID is attached to GenerationStart.UserID when non-empty. When empty,
	// agento11y.UserIDFromContext is consulted.
	UserID string
	// Metadata is merged into GenerationStart.Metadata. ContextInfo entries
	// win on key conflict with metadata derived from ProviderOptions / params.
	Metadata map[string]any
	// Tags is merged into GenerationStart.Tags. ContextInfo entries win on
	// key conflict with the client's configured tags.
	Tags map[string]string
	// AgentName overrides agento11y.AgentNameFromContext when non-empty.
	AgentName string
	// AgentVersion overrides agento11y.AgentVersionFromContext when non-empty.
	AgentVersion string
}

// WrapOptions is the top-level options struct for Wrap and Stack.
//
// ClientResolver and ContextProvider supply the per-request *agento11y.Client
// and ContextInfo for both Recording and Hooks middlewares. Each nested
// options struct MAY override these with its own resolver/provider; the
// override wins when both are set.
type WrapOptions struct {
	// ClientResolver supplies a per-request *agento11y.Client. If nil, the
	// nested Recording / Hooks ClientResolver fields are consulted; if both
	// are nil, every request passes through unchanged.
	ClientResolver ClientResolver
	// ContextProvider supplies per-request ContextInfo. If nil, the nested
	// Recording / Hooks ContextProvider fields are consulted; if both are
	// nil, the middleware logs a warning once per process and continues with
	// agento11y context defaults.
	ContextProvider ContextProvider
	// Recording configures the RecordingMiddleware.
	Recording RecordingOptions
	// Hooks configures the HooksMiddleware. Hooks are omitted from the stack
	// only when both top-level and nested ClientResolver are nil. Enabled gates
	// hook evaluation per request.
	Hooks HooksOptions
}

// IdentitySource selects the model identity recorded for a generation.
type IdentitySource string

const (
	// IdentityPreferResponse prefers complete response identity over requested identity.
	IdentityPreferResponse IdentitySource = "prefer_response"
	// IdentityRequested keeps requested identity and omits mapped response and transport
	// identity. Caller-supplied metadata and tags are not filtered.
	IdentityRequested IdentitySource = "requested"
)

// RecordingOptions configures RecordingMiddleware.
type RecordingOptions struct {
	// IdentitySource defaults to IdentityPreferResponse.
	IdentitySource IdentitySource
	// StreamDrainTimeout enables bounded asynchronous cancellation cleanup when
	// positive. Zero preserves the default behavior of not draining after cancellation.
	StreamDrainTimeout time.Duration
	// ClientResolver, when set, overrides WrapOptions.ClientResolver for the
	// Recording middleware only.
	ClientResolver ClientResolver
	// ContextProvider, when set, overrides WrapOptions.ContextProvider for
	// the Recording middleware only.
	ContextProvider ContextProvider
	// OnRecordError receives recorder-local validation and queue failures after
	// End. Handler panics are recovered so telemetry remains fail-open.
	OnRecordError RecordErrorHandler
	// OnRecordComplete runs once after End, regardless of recorder outcome. It
	// can be used by process owners to bound shutdown around active recordings.
	OnRecordComplete RecordCompleteHandler
	// GenerationFilter applies a final consumer-owned observation policy to
	// mapped unary and stream generations. Nil preserves existing behavior.
	GenerationFilter GenerationFilter
	// ContextSource defaults to ContextAmbientFallback.
	ContextSource ContextSource
}

// HooksOptions configures HooksMiddleware.
type HooksOptions struct {
	// Enabled, when non-nil, is called per request to gate the Hooks
	// preflight. Returning false causes the middleware to pass through
	// without contacting Agent Observability. A nil Enabled means "always run hooks
	// when a client is available".
	Enabled func(ctx context.Context) bool
	// MaxLatency, when greater than zero, bounds the EvaluateHook call via
	// context.WithTimeout on a derived context. The deadline does NOT
	// propagate to the inner model call; only the hook RPC is cancelled.
	// Zero (default) inherits the request context unchanged.
	MaxLatency time.Duration
	// ClientResolver, when set, overrides WrapOptions.ClientResolver for the
	// Hooks middleware only.
	ClientResolver ClientResolver
	// ContextProvider, when set, overrides WrapOptions.ContextProvider for
	// the Hooks middleware only.
	ContextProvider ContextProvider
}

// resolveRecordingOptions composes WrapOptions defaults into a concrete
// RecordingOptions usable by RecordingMiddleware.
func resolveRecordingOptions(top WrapOptions) RecordingOptions {
	out := top.Recording
	if out.ClientResolver == nil {
		out.ClientResolver = top.ClientResolver
	}
	if out.ContextProvider == nil {
		out.ContextProvider = top.ContextProvider
	}
	return out
}

// resolveHooksOptions composes WrapOptions defaults into a concrete
// HooksOptions usable by HooksMiddleware.
func resolveHooksOptions(top WrapOptions) HooksOptions {
	out := top.Hooks
	if out.ClientResolver == nil {
		out.ClientResolver = top.ClientResolver
	}
	if out.ContextProvider == nil {
		out.ContextProvider = top.ContextProvider
	}
	return out
}
