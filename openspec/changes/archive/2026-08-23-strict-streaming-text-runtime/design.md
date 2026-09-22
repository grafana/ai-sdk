## Context

Phase 3 added one strict `ai-gateway/providerwire/v4` handler for unary text. It validates the exact ProviderWire envelope, bounds the body, applies standard Go JSON semantics and the complete request schema, maps the supported text subset through typed DTOs, resolves a canonical model, invokes `DoGenerate`, preflights a minimal private response DTO, encodes it with standard Go JSON, and commits only after the final byte bound succeeds. Its envelope currently rejects `ai-language-model-streaming: true`.

The registered authority remains upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, `@ai-sdk/gateway@4.0.52`, and `@ai-sdk/provider@4.0.7`. The Gateway client posts the same body for unary and streaming calls, parses streaming responses with a permissive event-source handler, filters raw parts unless requested, converts response-metadata timestamp strings, tolerates `[DONE]`, and accepts clean EOF. Upstream types define event shapes but not a complete server lifecycle, and stream errors are `unknown`; local private DTOs, schemas, and state tests must therefore remain response authority.

The Go provider boundary returns a `StreamResult` containing a receive-only channel of `provider.StreamPart`. Providers may emit start, metadata, content, error, and finish parts. Most built-in providers emit an initial start. Anthropic currently attaches conversion warnings to finish instead, while the pinned upstream Anthropic provider emits them on start; preserving warnings in a server-owned initial event requires correcting that provider behavior rather than buffering the entire stream.

This change is parity-sensitive at the ProviderWire, provider-contract, Anthropic provider, and cross-language interop layers. Provider events are untrusted output: they can contain invalid state, private metadata, hostile errors or warnings, very large strings, continuously ready floods of small parts, or channels that do not close after cancellation.

## Goals / Non-Goals

**Goals:**
- Preserve the one-way `ai-gateway -> SDK` module and license boundary by publishing reusable Anthropic behavior as an immutable Apache prerequisite and keeping all ProviderWire runtime and integration code under `ai-gateway/`.

- Execute the phase 3 text request subset through `DoStream` with the same strict pre-model sequence.
- Define an irrevocable setup/commit boundary and normalized, closed, text-only SSE state machine.
- Preserve safe start warnings, response metadata, empty text deltas, usage, finish reasons, and ordered non-terminal provider errors.
- Keep backend identity, transport data, provider errors and warning prose, raw values, and metadata private.
- Bound part cardinality, retained ID state, complete frame bytes, model duration, stream idleness, handler latency, and handler-owned draining.
- Ensure cancellation, timeout, finish, writer failure, and provider-event races produce safe bounded outcomes.
- Prove compatibility through the exact pinned Gateway client while retaining raw Go authority.

**Non-Goals:**

- Reasoning, tools, approvals, files, generated media, sources, custom parts, structured output, provider options, body-carried headers, or raw output.
- Authentication, `/config`, service mount prefixes, process lifecycle, provider construction, or deployment.
- The reusable Go V4 client or `providers/grafana`.
- Reusing provider-domain JSON methods as protocol authority.
- Compatibility with Vercel's private Gateway service.
- Forcibly terminating provider code that ignores context forever.

## Decisions

### Extend one handler and preserve the existing request pipeline

Envelope validation will return an execution mode after accepting only exact `false` or `true`. Both modes will share the bounded body, standard JSON, complete schema, typed mapper, and resolver path. Only the final model invocation and response path diverge. The mapper will continue to execute the same text/scalar subset in both modes; later mode-specific capabilities can branch explicitly when they land.

This prevents a second streaming decoder from drifting from unary validation without exposing internal stage hooks or a protocol-local policy abstraction. It is a parity-preserving Go adaptation: HTTP mode dispatch differs structurally from TypeScript but preserves the registered request semantics.

Alternative: add a separate streaming handler. Rejected because it would duplicate security-sensitive validation and make golden replay and host mounting ambiguous.

### Add explicit streaming construction limits

`Limits` will add a positive provider stream-part count, complete SSE frame bytes, stream idle duration, and drain duration. Construction will validate positivity and prove that the frame limit contains the canonical empty start and every terminal error frame. The existing total model duration will cover setup and consumption.

The runtime increments one request-scoped part count for every provider-channel receive before interpreting the value, including start, errors, finish, and values consumed by terminal drain. It accepts at most `StreamParts`; receiving the first excess part causes a terminal adapter error before finish, or ends drain after an authoritative terminal event, before that part affects state or output. This bounds cumulative receive work and the global text-ID set even when a provider channel is continuously ready. Start-warning cardinality is checked against the maximum count that can fit the remaining frame budget before allocating a mapped warning slice.

The frame limit counts `data: `, JSON, and `\n\n`. A normal warning or content frame need not fit at construction: oversized provider-controlled values fail at runtime and transition to the guaranteed terminal fallback.

Alternative: rely on total timeout to bound event count. Rejected because a fast provider can perform unbounded work and grow retained ID state before a wall-clock timeout. Alternative: reuse unary response bytes. Rejected because unary document and per-event limits protect different resources and have different fallback requirements.

### Transfer setup ownership before logical commitment

`DoStream` setup runs concurrently with a request-derived timed context and panic recovery so handler latency remains bounded even if a provider ignores cancellation. The setup worker sends its outcome over an unbuffered channel. A successful receive transfers ownership to the handler; if the model context ends first, the worker retains ownership and asynchronously drains any stream returned later. This channel handoff provides one cleanup owner without a protocol-specific atomic state machine. When setup completion and termination race, either safe ownership outcome may win.

A setup error, panic, `nil, nil`, nil result, nil channel, or result-plus-error remains a pre-commit JSON failure. The owner cancels and asynchronously drains every non-nil invalid or late channel without waiting for cleanup. A valid transferred result and channel is the logical SSE commitment boundary.

At commitment the handler sets HTTP 200, `Content-Type: text/event-stream`, and `Cache-Control: no-cache, no-transform`, and applies the same response-controller flush classification used for frames. It then pre-reads the first provider part before choosing the normalized start. Any cancellation, timeout, invalid first part, or premature EOF after setup remains an SSE outcome; it cannot revert to JSON. If no first part arrives, the handler emits empty start plus the applicable terminal error when writable.

Alternative: use a buffered result channel. Rejected because a late send can succeed after the handler returns and leave an unread stream. Alternative: delay commitment until a valid first part. Rejected because it would turn post-setup provider lifecycle failures into non-2xx responses and make the commitment boundary depend on provider timing.

### Normalize start through one value-safe warning mapper

The public stream always begins with exactly one server-owned start. If the first provider part is `PartStreamStart`, the runtime consumes it and maps all registered warning variants through a streaming-specific mapper. Unary output remains minimal and omits provider warnings. The stream mapper never copies arbitrary `Feature`, `Setting`, `Message`, or `Details` strings. Phase 4 uses this closed table and includes no model identity in warning prose:

| Type | Public fields |
| --- | --- |
| `unsupported` | `feature: "model capability"`, `details: "a requested model capability is unsupported"` |
| `compatibility` | `feature: "model compatibility"`, `details: "a requested setting was adjusted for model compatibility"` |
| `deprecated` | `setting: "model setting"`, `message: "a requested model setting is deprecated"` |
| `other` | `message: "the model reported a warning"` |

Unknown warning discriminators fail safely. Warning cardinality is frame-budget checked before allocation.

If the first part is not start, the runtime writes `warnings: []` and processes that part next. Late or duplicate provider starts fail the lifecycle. If a warning start is invalid or too large, the guaranteed empty start is written before one synthetic terminal error. This does not silently downgrade to empty warnings: the stream fails safely.

Anthropic retains its current upstream-event preflight so an initial API failure remains a pre-commit setup error. After that preflight succeeds, its provider goroutine emits `PartStreamStart` with conversion warnings before handling or emitting the already pre-read first event and removes warnings from finish. This is an implementation bug fix against the matching pinned provider source, not a new provider feature. Existing provider input fixtures remain unchanged; only derived expectations are regenerated if affected.

Alternative: treat DTO shape as a warning allowlist. Rejected because provider-controlled strings can contain credentials and backend model identity. Alternative: buffer the complete Anthropic response so finish warnings can be moved backward. Rejected because it destroys streaming and makes memory and latency proportional to the full response.

### Use a closed text state machine

After start, state tracks whether metadata was emitted, all used text IDs, the active text ID, and whether finish was written. Response metadata is optional, at most once, and must precede text. It preserves only an optional response ID and UTC timestamp, replaces model identity with the canonical catalog ID, and drops provider identity, response headers, backend model IDs, and provider metadata.

Text blocks are sequential. Start IDs must be non-empty valid UTF-8 and globally unique; a delta or end must match the one active ID. Empty deltas remain required values. Provider errors are allowed before metadata, between blocks, and inside an active block and do not change state. Unsupported part families and invalid transitions become terminal adapter failures.

The non-empty, unique ID and placement rules are part of the documented strict server dialect. Upstream types constrain shape but do not define a complete producer state machine; the pinned client remains permissive.

Alternative: forward any schema-shaped part. Rejected because shape-only validation cannot prevent ambiguous blocks, metadata drift, backend leakage, or invalid finish placement.

### Make finish the final authoritative public event

Finish requires no active text block, empty finish warnings, valid non-nil usage, and a registered non-nil finish reason. Usage reuses the unary non-negative JavaScript-safe checks and omits raw usage and provider metadata. Once the bounded finish frame is written, it is authoritative and the final public event. The handler cancels provider work, starts bounded asynchronous drain, and returns clean EOF immediately without waiting for provider channel closure or emitting `[DONE]`.

Later provider parts are suppressed by drain rather than producing a second error and are classified as provider lifecycle defects for later operational reporting. Phase 4 proves suppression and bounded ownership; service-level reporting lands with observability. This resolves the plan's apparent tension between terminal authority and post-finish validation: public terminal state cannot be rewritten after finish.

Alternative: wait for channel close before writing finish so later parts can be rejected publicly. Rejected because it withholds a valid terminal event indefinitely from clients when a provider delays or forgets channel closure.

### Distinguish non-terminal provider errors from terminal adapter errors

Each pre-finish `PartError` is reduced independently through the existing closed safe-category mapping and emitted in place. The strict event payload is:

`{"type":"error","error":{"message":...,"type":...,"param":null,"code":...,"statusCode":...,"retryable":...}}`

The registered stream error is `unknown`, so this narrower object is a local strict dialect whose correctness comes from fixed safe fields, a test-only schema, and raw assertions. Provider errors do not close blocks or terminate the state machine; multiple errors and later valid content remain ordered. Nil or malformed provider errors reduce to canonical internal error values without exposing their original representation.

Adapter failures are different. Lifecycle violations, unsupported output, premature EOF, mapping errors, or frame overflow produce at most one synthetic terminal internal error before finish. Cancellation and timeout use their existing safe categories. No synthetic finish or block-end event is inserted.

Alternative: terminate on every provider error. Rejected because the registered V4 contract permits multiple error parts and the phase acceptance explicitly requires later content and finish to remain consumable.

### Encode, validate, write, and flush one bounded complete frame

Private DTOs cover start, response metadata, text start/delta/end, and finish. Synthetic terminal errors select fixed precomputed frames, while provider-originated errors select fixed safe category frames. A closed draft 2020-12 schema validates fixtures and raw output in tests rather than running on the production write path. Before standard Go JSON encoding, a lightweight preflight bounds dynamic string bytes and collection cardinality and validates UTF-8. The complete `data: <json>\n\n` frame is then checked against the configured limit before one write. JSON escaping may allocate a bounded constant multiple of the configured frame limit, matching the simpler unary response trade-off.

Commitment headers and each complete frame are flushed with `http.NewResponseController(w).Flush()`. Unsupported flushing is detected with `errors.Is(err, http.ErrNotSupported)` so direct and wrapped sentinel errors mean no flush capability and do not fail the stream. Every other header or frame flush error or panic is a writer failure. Short writes, write errors, and writer panics likewise mark the writer unusable, cancel provider work, and end without a second write. Production HTTP server write timeouts remain responsible for a kernel or network write that blocks without returning.

Alternative: stream JSON directly into the response. Rejected because partial invalid events cannot be recovered and would violate the complete-frame boundary.

### Use standard cancellation and timeout primitives

A request-derived `context.WithTimeout` begins immediately before `DoStream` and bounds setup plus consumption. After commitment, one resettable `time.Timer` bounds stream idleness and resets after every represented provider part, including a consumed start and a written provider error. Request cancellation and both timeout paths cancel provider work before writing their safe terminal outcome.

The runtime uses an ordinary `select` over provider output, model-context completion, and idle timeout. When several become ready concurrently, any applicable safe bounded outcome may win. The protocol does not expose scheduler-level ordering, deadline equality, or a test clock. A finish becomes authoritative once written, and writer failure ends immediately because the writer is no longer safe for another event.

Alternative: define exact precedence with a custom clock and arbiter. Rejected because it turns test mechanics into protocol behavior without a client or product requirement.

### Cancel and drain without extending handler latency

Every established-stream exit cancels the provider context and starts one bounded asynchronous drain owned by the outcome receiver. The drain retains the request's provider-part counter and stops on channel close, the first excess part, or the configured duration. It uses standard time and checks elapsed duration while consuming so a continuously ready channel cannot run indefinitely. Handler return does not wait for it.

If request cancellation or total timeout wins before setup transfer and `DoStream` later returns a non-nil stream, the setup worker owns the same bounded drain. If `DoStream` never returns, that provider-call goroutine may remain; if a producer remains blocked after drain expiry, that work belongs to the non-compliant provider. The Gateway bounds request latency and its own drain lifetime but does not claim it can reclaim arbitrary provider resources.

Alternative: synchronously drain before returning. Rejected because a defective provider would extend request latency to the drain duration and complicate shutdown.

### Combine local authority with pinned-client and parity evidence

Go unit tests will own setup cleanup, state, errors, warning privacy, frame bytes, part-count bounds, cancellation and timeout outcomes, writer behavior, and drain lifetime. The phase 2 `streaming.json` and streaming `sequence.json` record will move from unary envelope rejection to supported `DoStream` execution. Pinned-client evidence is split: one abort test uses a model that returns a non-nil silent channel, waits until `doStream()` resolves after HTTP commitment, then aborts and proves server-side provider-context cancellation; un-aborted tests consume normal, provider-error, adapter-timeout, finish, and clean-EOF streams. Client behavior is compatibility evidence only because its event parser is permissive.

No provider `input*.chunks.txt` will be invented or edited. Anthropic conversion cases use focused unit tests and, where existing provenance-valid fixtures are affected, regenerate only expectations from the unchanged recorded inputs. `test/conformance/PARITY.md` will move strict streaming text from gap to the achieved mixed/automated scope while retaining later-family gaps.

Observed differences are classified as follows:

- channel/context structure and private DTOs: parity-preserving Go adaptations;
- Anthropic finish-warning placement: implementation bug corrected to the registered upstream provider;
- replacing streaming provider warning strings with fixed public prose: intentional privacy deviation from upstream passthrough, documented in `test/conformance/PARITY.md`;
- non-empty unique IDs and closed error DTO: documented strict server-dialect constraints within the permissive public-client surface;
- omitted `[DONE]`: compatible server choice explicitly accepted by the pinned client, not a deviation.

## Risks / Trade-offs

- [A fast provider floods small parts and IDs] → Enforce `StreamParts` before accepting each received part, bound warning allocation from the frame budget, and test continuously ready channels below, at, and above the limit.
- [Setup completion races cancellation] → Transfer ownership over an unbuffered result channel so either receiver owns cleanup without a separate atomic protocol.
- [Provider warning strings contain backend identity or credentials] → Never copy arbitrary warning values; use one closed mapper with fixed prose and no model/provider identity.
- [Provider start warnings exceed the frame limit] → Guarantee an empty start and terminal fallback, fail rather than drop warnings, and test exact frame boundaries.
- [A provider emits valid data in a lifecycle order the strict dialect rejects] → Keep the transition table explicit, compare built-in providers and the pinned type surface, and add focused provider tests before widening rules.
- [Timer and channel races produce different safe outcomes] → Treat scheduler-level precedence as unspecified and assert only bounded, valid terminal behavior.
- [A provider ignores cancellation] → Bound handler latency and drain ownership; report the remaining provider goroutine or blocked producer as a provider lifecycle defect.
- [A response write blocks forever] → Rely on production server write deadlines; after an observable write/flush failure, never attempt another frame.
- [Safe provider warnings still contain sensitive prose] → Emit only registered warning fields and no metadata; future host policy may further restrict warning content without widening the wire DTO.
- [Anthropic warning movement changes many derived snapshots] → Keep recorded inputs byte-identical, regenerate expectations only, and inspect every changed snapshot for the expected start/finish movement.
- [Strict error objects are accepted but not validated by the client] → Make fixed documents, test-only schemas, and raw HTTP assertions authoritative.

## Migration Plan

1. Extend handler construction with stream-part cardinality, mode parsing, streaming-specific safe warning mapping, and test helpers while preserving phase 3's minimal unary behavior.
2. Correct Anthropic stream-start warning timing after initial-event preflight and update focused tests and derived expectations from unchanged inputs.
3. Add private stream DTOs, a test-only event schema, fixed stream errors, standard JSON frame encoding with bounded preflight, response-controller flushing, and raw encoder tests.
4. Add unbuffered setup ownership transfer, start normalization, bounded text state, provider errors, finish, and premature-EOF handling.
5. Add standard context/timer cancellation, writer failure handling, late-setup cleanup, bounded drain duration, and continuously-ready flood tests.
6. Replay committed request goldens, add split pinned-client streaming/cancellation integration, and update parity coverage.
7. Run ProviderWire, integration, provider, parity, and full module validation.

Rollback removes the streaming dispatch, stream schemas/runtime, and new limits and restores Anthropic warning placement in the same change. The unary handler and phase 2 contract workspace remain independently usable, and no deployed phase 5 service depends on this package yet.

## Open Questions

None. Later content families, host policy controls, service authentication, Go-client parsing, and production server write-timeout configuration remain assigned to later work packages.
