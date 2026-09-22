## Context

WP5 leaves the service with strict, bounded ProviderWire V4 unary/streaming text, a static canonical catalog, named Anthropic configuration, and one direct backend per public route. The Apache `fallback.Model` already performs ordered unary selection and peeks one stream part, but its archived contract predates the Gateway commitment model: it promotes a leading `PartError` into a retryable Go error, returns an empty stream as success on premature EOF, relays through an uncancelable goroutine, and launches unbounded drains. Its observer reports raw candidate identity/error at a setup-or-first-part boundary but does not state the selection or retry decision.

The strict adapter commits HTTP SSE when a wrapped model returns a valid stream. Therefore a fallback model can safely delay that return until it has either buffered one provider part or exhausted pre-commit candidates. Once any part is buffered—including `PartError`—returning the wrapper stream establishes the candidate and the existing adapter emits that part using its privacy-safe mapping. This is compatible with `retry`'s established-stream rule and avoids replaying visible output or future effects.

The change spans the Apache fallback primitive and AGPL Gateway composition. Planning baseline is integration merge `c055bec`, combining WP8 `a2177d9` and WP7 `e0e6c01`; both prerequisites are present locally. Reconcile with their accepted heads before implementation if they change. WP8 owns `service/observation.go`, where `observationFromContext` returns an immutable `requestObservation` snapshot. WP9 adds a tiny service-local correlation-only helper over that accessor; it does not read context keys or authentication state directly. `NewModelObservabilityFactory` owns the single logical chain: enrichment → Agent Observability → structured logging/Prometheus → canonical identity. WP9 places fallback beneath that chain and owns the private physical record type, projection, bounded sink/queue, and drop accounting.

## Goals / Non-Goals

**Goals:**

- Select configured candidates in deterministic order only after eligible pre-commit failures.
- Make any received provider part—including an error part—the irrevocable stream commitment and replay it exactly once in order.
- Classify premature EOF as a pre-commit candidate failure rather than successful empty output.
- Ensure cancellation releases every owned bridge and bounded cleanup goroutine; a synchronous provider DoStream that never returns can retain its setup-call goroutine, as in the strict adapter's documented residual risk.
- Emit one closed, stable attempt-decision record per invoked physical candidate with enough data for private attribution and winner/retry analysis.
- Build public routes once from strict, direct, non-recursive configuration while keeping backend identity and topology private.

**Non-Goals:**

- Fallback after provider output, unary success, a provider error part, or any effect.
- Effectful tools, tool replay/idempotency, structured output, files, reasoning, or later capability packages.
- Request-controlled provider/fallback selection, health-based reordering, hedging, retrying the same candidate, sticky winners, or dynamic configuration reload.
- Changing ProviderWire request/response/SSE/discovery schemas, the Go Gateway client, provider retry policy, image capacity, or production rollout assets.
- Recording full physical stream lifecycles in the fallback hook; WP8 retains logical lifecycle/usage/error recording.

## Decisions

### 1. Treat receipt of any provider part as commitment

`fallback.Model.DoStream` waits for one of: a synchronous setup result, request cancellation, a first part, or channel close. A non-nil result with a nil stream is an invalid pre-commit setup result. A first part of any registered or unrecognized type is buffered and returned through a wrapper stream; `PartError` is not inspected by the fallback decider. The transport adapter remains responsible for validating and safely mapping the part.

This deliberately reverses the archived `fallback-stream-error` behavior. A provider error part is a value in the LanguageModelV4 stream protocol, not a failed `DoStream` call. Treating it as retryable risks dropping ordered error information today and duplicating effects when later capabilities land.

Alternative: let the Gateway provide a part-acceptance predicate. Rejected because it couples the generic Apache primitive to one adapter/capability subset and creates different commitment semantics for different hosts.

### 2. Represent premature EOF as an explicit pre-commit error

If the candidate channel closes before yielding a part, fallback creates a stable sentinel error (for example `ErrPrematureStreamEnd`), records the attempt decision, and applies the existing decider. The default decider treats it as eligible while request context is live. On the last candidate or a rejecting decider, normal aggregate-error behavior returns a Go error before the strict adapter commits SSE.

Alternative: preserve empty success. Rejected because strict text streams require lifecycle output and Nara's acceptance contract explicitly classifies premature EOF before the first part as fallback-eligible.

### 3. Give every candidate its own cancelable context and bound only owned goroutines

Each physical attempt receives a child context. Abandoning a candidate cancels it immediately. Cleanup uses a package-owned positive fixed upper bound and exits on channel close, the bound, or a fixed part budget; it never extends caller-visible selection latency. A successful bridge sends the buffered part and later parts with selects on the request/candidate context, so a blocked downstream consumer cannot retain the bridge. The bridge closes only its own output channel and never closes a provider-owned channel.

The guarantee is intentionally about fallback/Gateway-owned goroutines. An uncooperative provider producer may leak its own goroutine after ignoring cancellation; that remains a provider lifecycle defect, matching the existing strict adapter contract.

Alternative: drain until provider closure. Rejected because a silent or continuously ready hostile channel can live indefinitely. Alternative: no drain after cancellation. Rejected because a cooperative producer may be blocked on a send and unable to observe cancellation.

### 4. Observe one physical decision, not the whole stream lifecycle

Extend `Attempt` additively with `Outcome AttemptOutcome` and `WillFallback bool` while preserving its existing index, provider, backend model ID, timestamps, and error. `AttemptOutcome` is a string enum with exactly three exported values: selected, failed, and canceled. Retry is not a fourth outcome: `Outcome=failed` together with `WillFallback=true` records the intent to advance while the request is live and another candidate exists at decision time; selected is the winner fact. Later cancellation, including during the synchronous observer, may prevent that next invocation without rewriting the event. Subsequent attempt records establish which candidates were actually invoked. Emit exactly once after a candidate reaches a selection decision:

- selected: unary returned successfully or a stream produced its first part;
- failed: setup failed, returned an invalid result, or ended prematurely, with `WillFallback` reflecting the live decision-time intent to try the next candidate;
- canceled: the request ended before selection, including cancellation observable in the context snapshot immediately after the decider returns. That snapshot normalizes the event error and returned error to the context cause and sets `WillFallback=false`, irrespective of the decider's answer.

The decider still runs exactly once for each pre-commit failure whose request is live on entry, including the final candidate. A synchronous decider must return before fallback can finish the decision; cancellation does not interrupt arbitrary callback code. Cancellation after a failed decision to advance but before the next invocation stops progression and remains in the returned error chain alongside earlier failures, in both unary and streaming modes.

For a selected stream, `FinishedAt` means decision/commit time, not channel completion. Post-commit stream errors remain ordered data and belong to WP8's logical lifecycle. The generic callback stays synchronous and documented as required to return promptly; the Gateway callback reads correlation through WP8's narrow accessor, projects the event, and performs only a non-blocking enqueue into the WP9-owned physical sink. It recovers from observer panics so telemetry cannot affect model selection.

Alternative: keep the observer alive through stream completion. Rejected because it would duplicate WP8 lifecycle semantics and complicate ownership/cancellation. Alternative: invoke arbitrary observers in timeout goroutines. Rejected because an observer that ignores cancellation would leak one owned goroutine per attempt.

### 5. Keep route references direct and make recursion unrepresentable

Retain the current `primary: {provider, model}` object and add `fallback` as an ordered list of the same direct reference shape. Every element directly names a configured provider instance and opaque backend model ID; public-route references and nested fallback objects are not schema arms and strict YAML rejects them. Validation rejects empty fields, unknown providers, duplicate `(provider, model)` tuples across primary/fallback, and an empty effective route. This structurally rejects self-reference and longer route recursion without a graph walker.

Alternative: allow fallback entries to name other public routes and detect graph cycles. Rejected for WP9 because it adds no required expressiveness, obscures physical attribution, and turns startup into recursive composition. Later route-composition work can propose that separately.

### 6. Construct candidates once, then wrap once at the logical boundary

At startup, construct every direct candidate from named provider configuration, build `fallback.Model` only for routes with fallback entries, attach one private attempt observer, apply the WP8 canonical identity override, and then the one logical middleware chain. Aliases resolve to that same immutable final model instance. Candidate models are never wrapped in logical Agent Observability/logger/Prometheus middleware.

The private observer obtains correlation only through WP8's narrow accessor and projects an allowlisted WP9-owned physical record: logical correlation ID, candidate index, configured provider-instance name, backend model ID, start/decision timestamps or duration, one of the three closed outcomes, decision-time `WillFallback` intent, and selected winner. WP9 defines and owns the bounded physical sink/queue, non-blocking enqueue behavior, lifecycle, and bounded-cardinality drop diagnostics; WP8 supplies no physical sink or physical telemetry type. Because multiple configured instances all report provider `anthropic`, the Gateway closes over the immutable ordered route descriptors and maps the hook's validated one-based index back to the configured instance/backend tuple; it does not infer instance identity from `LanguageModel.Provider()`. It never records raw error text/data, credentials, request/response content, headers, URLs, or caller identity beyond the approved correlation key. Missing correlation or an impossible observer index must not fail the call; invalid records are dropped with bounded-cardinality diagnostics and identity is never guessed.

### 7. Preserve adapter and client privacy boundaries

Fallback aggregate errors remain internal values and flow through the existing fixed safe-error mapping. Discovery and canonical response metadata are generated from the catalog's public identity. No attempt record, candidate count/order, provider instance, backend ID, or raw failure is added to ProviderWire bytes, public errors, HTTP access logs, logical metrics labels, or logical Agent Observability. WP7 therefore requires no fallback-specific protocol or API.

## Risks / Trade-offs

- **[Behavioral break]** Consumers relying on leading `PartError` failover will instead receive the error stream. → Document prominently, update the archived capability through a delta, and test leading/multiple error-part ordering.
- **[Higher time-to-first-byte]** Fallback waits for the first candidate part before returning a stream. → This already occurs in the current fallback wrapper and is necessary for pre-commit selection; the request/model deadline bounds it.
- **[Observer backpressure]** A synchronous third-party observer can delay a call. → Document the prompt-return contract; Gateway uses non-blocking enqueue into its WP9-owned bounded physical sink and records dropped-event counters only with bounded labels.
- **[Incomplete provider cleanup]** A provider that ignores cancellation can leak its own producer. → Bound all fallback-owned work and classify provider non-cooperation as a private lifecycle defect.
- **[WP8 integration drift]** Exact WP8 and WP7 heads are integrated locally but may change before acceptance. → Reconcile the service-local helper and ModelFactory wiring against accepted heads; retain WP9 ownership of every physical telemetry type and sink.
- **[Retry amplification]** SDK retries can restart the full fallback chain. → Keep existing retry semantics, expose decision-time fallback intent and invoked-attempt records privately, and document provider retry × fallback × SDK retry multiplication.

## Migration Plan

1. Verify the integrated WP7/WP8 heads against accepted prerequisites; add the service-local correlation helper over `observationFromContext` without changing snapshot ownership or moving the physical sink into WP8.
2. Land the Apache fallback primitive, focused race/lifecycle tests, and documentation first; publish or immutable-pin that root module revision for the isolated Gateway module.
3. Land strict Gateway configuration, one-time route construction, fallback placement, and private projection against the immutable Apache pin with `GOWORK=off` validation.
4. Deploy no production configuration in WP9. WP10 adds staged fallback configuration, authenticated client smoke, operational evidence, and rollback to a direct-only route. Removing `fallback` entries restores direct-only behavior without changing public IDs.

## Open Questions

- No product decision blocks implementation. During rebase, WP9 must use the service-local correlation accessor over WP8's immutable observation snapshot rather than reading its private context directly or inventing a second context key. WP9 must independently define and own the physical record, bounded sink/queue, enqueue behavior, and drop accounting; this is not permission to merge logical and physical telemetry.

## Integration with WP11 and WP12

WP11 may enable function tools on direct-only routes. A fallback-configured route must reject any request containing non-empty tool definitions, tool choice, tool-call/result history, or later effectful content before invoking its first candidate. Put that guard in Gateway route composition beneath the logical chain; strict mapping still rejects unsupported families globally until their package lands. Do not silently select only the primary for a tool request. WP12 preserves the guard. A future separately specified replay/idempotency capability is required to lift it.

The private sink is a dedicated bounded asynchronous operator sink with a fixed worker and bounded writes/shutdown. It must be connected to an actual private operational output in service composition; an in-memory test sink alone does not deliver physical attribution. Keep it separate from the WP8 logical logger redactor and metric labels.

The selected output is the existing private operator stderr destination through
a separately owned socket descriptor or Linux nonblocking pipe handle. One
worker drains at most 256 queued records, each bounded to 4096 encoded bytes,
with a 100 ms write timeout and one-second shutdown budget. Unsupported stderr
types disable this physical output without failing model execution; fixed-class
drop counters report unavailable transport, queue saturation, invalid records,
worker defects, and shutdown drops. No new operator setting or logical logger
projection is introduced. Deployment smoke must verify the actual stderr
transport and private access policy before enabling fallback in production.

On non-Linux Unix platforms, a socket must already be nonblocking; blocking
sockets fail open without output. A macOS saturation test demonstrated that a
blocking Unix stream socket can block inside `sendmsg` despite `MSG_DONTWAIT`.
The sink must not change shared descriptor flags because WP8 also uses stderr.
