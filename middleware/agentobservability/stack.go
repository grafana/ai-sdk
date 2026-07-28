package agentobservability

import (
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

// Stack returns the canonical Agent Observability middleware ordering for the given
// WrapOptions:
//
//   - HooksMiddleware (outer) — denies first, transforms second.
//   - RecordingMiddleware (inner) — records the call as actually executed.
//
// Hooks is omitted from the returned slice only when neither
// WrapOptions.ClientResolver nor opts.Hooks.ClientResolver is set — without a
// resolver the middleware can never produce an Agent Observability client and is
// purely overhead.
// Whenever a resolver is present, Hooks stays in the stack so opts.Hooks.Enabled
// can gate per request using the live request context. Callers that want to
// statically disable hooks can omit them from a custom stack instead.
// RecordingMiddleware is always included so callers wrapping their base model
// with Stack always get a generation row when a client resolves.
func Stack(opts WrapOptions) []middleware.Middleware {
	hookOpts := resolveHooksOptions(opts)
	recOpts := resolveRecordingOptions(opts)

	var out []middleware.Middleware
	if hooksEnabledAtConstruction(hookOpts) {
		out = append(out, HooksMiddleware(hookOpts))
	}
	out = append(out, RecordingMiddleware(recOpts))
	return out
}

// hooksEnabledAtConstruction decides whether Stack should include the Hooks
// middleware at all. The only construction-time signal we can trust is whether
// a ClientResolver is configured — without one the middleware can never reach
// an Agent Observability client. opts.Enabled is intentionally not consulted here: the
// documented usage is a per-request, context-dependent feature-flag check
// (see doc.go), which would typically return false against a bare
// context.Background() and accidentally remove Hooks from the stack for every
// request. The per-request gate lives in evaluateHook.
func hooksEnabledAtConstruction(opts HooksOptions) bool {
	return opts.ClientResolver != nil
}

// Wrap composes Stack(opts) around base using middleware.Wrap. It is the
// one-call convenience equivalent to:
//
//	middleware.Wrap(middleware.WrapOptions{
//	    Model:      base,
//	    Middleware: agentobservability.Stack(opts),
//	})
//
// Returning the wrapped model unchanged when no middlewares activate keeps
// the cost-of-doing-nothing zero for consumers without Agent Observability.
func Wrap(base provider.LanguageModel, opts WrapOptions) provider.LanguageModel {
	return middleware.Wrap(middleware.WrapOptions{
		Model:      base,
		Middleware: Stack(opts),
	})
}
