// Package execution is the protocol-independent model execution runtime that
// every Gateway API adapter shares.
//
// A [Runtime] selects the model for a request, invokes it once, and hands the
// adapter a typed outcome: a result, an established [Stream], or an error the
// adapter classifies with [ClassifySelection], [ClassifyProvider] or
// [ClassifyStreamPart]. It also captures the call's fallback attempts and
// projects the execution [Overview] and native failure summaries.
//
// The runtime owns:
//   - selection, validated and contained, under the request context;
//   - invocation of DoGenerate or DoStream, with panics and missing results
//     failing as internal;
//   - stream waits, the idle duration and one drain bounded by the drain
//     duration, with exactly one cleanup owner for every returned stream;
//   - the closed failure categories and reasons;
//   - request-local attempt capture, projected into an overview for
//     configured selections and for [AccountSelection] request accounts.
//
// Adapters own request mapping, wire encoding, their error documents,
// response commitment and their protocol state machine. The runtime never
// writes HTTP responses.
//
// Selection and invocation run on the caller's goroutine. The request
// context's deadline, which the host sets, bounds them: when the context ends,
// its error decides the outcome and a result returned later is discarded. A
// selector or model that ignores cancellation holds its own request until it
// returns.
package execution
