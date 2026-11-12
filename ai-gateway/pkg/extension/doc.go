// Package extension is the contract for Gateway extensions: Go code that a
// distribution compiles into the Gateway to add per-call behavior, such as
// eligibility, quota, default settings, input and output checks, usage
// accounting and audit.
//
// A distribution passes [Definition] values to the Gateway's composition
// entrypoint. There is no global or import-time registry. At startup the
// Gateway validates every definition, reads operator configuration, and runs
// each enabled extension's factory once with its validated configuration and
// [Dependencies]. A factory may run more than once per process, once per
// configuration it is given, so it must not assume it is the only instance.
//
// # Phases
//
// An instance takes part in a phase only by implementing that phase's
// interface, and must implement at least one. The host owns control flow:
// extensions never receive the model or a continuation, never wrap it and
// cannot change identity, selection or credentials. For every call the host
// runs, in order:
//
//  1. [Admitter] across extensions in name order, with request information
//     only. The first [ErrDenied] ends the call.
//  2. [RequestTransformer] with tenant-scope extensions first, then
//     operator-scope extensions, each by ascending [Definition.Priority] and
//     then by name. Each transform receives the parameters the previous one
//     returned; the host refuses parameters that add a protected call header
//     or the call-level "gateway" provider-option namespace.
//  3. [RequestInspector] in name order, on exactly the parameters the model
//     receives. The first [ErrDenied] ends the call.
//  4. The selected model, once, with its fallback chain.
//  5. [ResponseInspector] in name order on each normalized [Part]. The first
//     [ErrDenied] aborts the response: a unary call fails, and a stream ends
//     with one error part.
//  6. [Observer] in name order, once per call, with the call's [Record], off
//     the response's critical path.
//
// An extension applies to a call only when its declared key ownerships include
// the call's; otherwise none of its phases run and it gets no record. The
// call's source is informational and does not decide where an extension
// applies.
//
// # Failures
//
// A phase that returns an error other than [ErrDenied], panics, or returns
// parameters the host refuses fails. Under [FailClosed] the call fails with
// [ErrUnavailable]; under [FailSkip] the phase's effect is discarded and the
// call continues. The host never surfaces an extension's error text to
// callers, logs or metrics; an extension logs its own details through
// [Dependencies.Logger] and is responsible for keeping them free of
// credentials and content.
//
// # Concurrency
//
// Instances are shared by concurrent calls and must be safe for concurrent
// use. The phases of one call never run concurrently, and each extension gets
// its own [Call] for each call, whose State it alone reads and writes.
//
// # Stability
//
// This API is v0 and experimental. It may change or break in any minor
// release, and is not a stable extension contract.
package extension
