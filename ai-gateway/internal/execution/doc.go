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
//   - tool-history reconstruction: [Runtime.Call] rebuilds the prompt's
//     [ToolHistory] before anything else and fails inconsistent history with
//     [ErrInvalidToolHistory];
//   - selection, validated and contained, under the request context;
//   - invocation of DoGenerate or DoStream, with panics and missing results
//     failing as internal;
//   - stream waits, the idle duration and one drain bounded by the drain
//     duration, with exactly one cleanup owner for every returned stream;
//   - output lifecycle validation, seeded from the tool history: every
//     stream part through a [StreamValidator] in [Stream.Next], including its
//     response segments, and every unary result through
//     [ValidateGenerateResult] in [Call.Generate]. A violation fails with
//     [ErrInvalidOutput], or [WaitInvalidOutput] for a stream;
//   - how a stream ends: a provider close after a finish is [WaitFinished],
//     a terminal success, a close after an error part with no lifecycle part
//     since is [WaitClosedAfterError], and any other close is [WaitClosed];
//   - the closed failure categories and reasons;
//   - request-local attempt capture, projected into an overview from the
//     expected attempts of [ConfiguredSelection] and [AccountSelection].
//
// Adapters own request mapping, representation, wire encoding, framing, their
// error documents, response commitment and their own protocol state machine,
// such as how their protocol sequences chunks. They keep the checks their own
// representation needs, such as valid UTF-8 or a numeric range, but evaluate
// no provider output lifecycle rule: they act on the outcome the runtime
// returns. The runtime never writes HTTP responses.
//
// An adapter that maps its API semantics to native options passes a
// [Translator] to [Runtime.Call] with [WithTranslator]. The host wraps every
// physical candidate once with [Translate], so the order around each one is:
// fallback, then translation, then native option validation, then the native
// model. Each attempt translates its own copy of the call's options for its
// [Target], and host policy re-checks the result. A panicking translator or a
// refused result fails with [ErrTranslation], which [FallbackDecider] does not
// retry.
//
// Selection and invocation run on the caller's goroutine. The request
// context's deadline, which the host sets, bounds them: when the context ends,
// its error decides the outcome and a result returned later is discarded. A
// selector or model that ignores cancellation holds its own request until it
// returns.
package execution
