# Design

## Context

See proposal.md (Why) for the motivation. Reference: the registered baseline in `test/conformance/upstream.yaml` (`@ai-sdk/gateway` 4.0.116, `@ai-sdk/provider` 4.0.29, `@ai-sdk/openai` 4.0.96, `@ai-sdk/anthropic` 4.0.82), read at those tags in a local checkout.

**Server today:**

- `ai-gateway/internal/execution/classify.go:114-145` (`ClassifyStreamPart`). A status-0 `APICallError` is classified as cancellation, timeout or transport (`classifyTransport`, `:166-181`), and otherwise falls through to `return CategoryUpstream` (`:139`).
  - `ClassifyProvider` (`:79-110`) covers unary and setup. It sends every `APICallError` through `categoryFromStatus`, so status 0 becomes upstream.
  - The callers of `ClassifyStreamPart` are `providerwire/v4/errors.go:67-69` and `internal/testadapter/adapter.go:74`.
- `providerwire/v4/error_response.go` maps categories to fixed documents (`publicErrors`, `:44-139`). `streamErrorResponse` (`:209-220`) allowlists the stream categories and always sets `StatusCode`. `publicError.StatusCode` already has `omitempty` (`:152`), so a definition with status 0 produces no `statusCode` member. `encodeHTTPError` (`:198-207`) writes `definition.status` as the HTTP status, so a status-0 definition must never reach it.
- `providerwire/v4/stream.go`:
  - `PartError` is handled at `:433-442`: it classifies the error, writes the frame and continues.
  - `PartRaw` is handled at `:484-497`: `projectRawValue` (`raw.go:38-42`) rejects an empty value, and the part is then dropped.
  - `validStreamEvent` (`:188-189`) requires a non-empty raw value, and `streamRawEvent.RawValue` (`:70`) has no `omitempty`.
  - Every non-part wait goes to `writeStreamTerminalForWait` (`:322-333`, `:547-549`). `WaitClosed` maps to `errInternal` (`internal/execution/runtime.go:208-220`), which is the synthetic `500`.
- `providerwire/v4/schema/stream_event.json:274-325` requires `rawValue` on `raw` and `statusCode` on `error`. This is the authoritative stream-event schema, used by `schema_test.go` and the TS `validateStreamEvent`. The request goldens and `schema-cases.ts` describe requests only and do not change.

**Go client today (`providers/grafana`):**

- `errors.go:75-98`: `wireStreamError.StatusCode` is an `int`. `decodeStreamError` requires `retryable` and checks it against the status-derived value.
- `errors.go:100-117`: `registeredError` is a closed (type, code, status) table.
- Stream errors surface only as `*provider.APICallError` (`errors.go:97`), not as `*GatewayError`.
- `stream.go:376` rejects a raw event without `rawValue`, and `stream_test.go:367` asserts that.
- `stream.go:94-96` already ends cleanly on EOF without finish.

**Upstream and direct-provider facts:**

- `@ai-sdk/provider` 4.0.29 declares the raw part as `{type:'raw'; rawValue: unknown}` (`language-model-v4-stream-part.ts:110-113`). `APICallError` defaults `isRetryable` to false when `statusCode` is absent (`api-call-error.ts:28-32`).
- `@ai-sdk/openai` 4.0.96 enqueues `{type:'raw', rawValue: chunk.rawValue}` for every chunk, including unparseable ones where the value is `undefined`, and then a status-less error (`openai-responses-language-model.ts:1682-1700`).
- `@ai-sdk/gateway` 4.0.116 enqueues each decoded part unchanged and filters raw parts only when they were not requested (`gateway-language-model.ts:156-180`).
- Direct Go OpenAI:
  - It emits a nil raw value for invalid JSON (`stream_preflight.go:73-75`).
  - A frame that fails to parse becomes a status-0 `PartError` with `IsRetryable=false` (`convert_stream.go:47-55`).
- Direct Go Anthropic: event-handling failures become a status-0 `PartError`, after which the stream closes (`model.go:262-270`). That is the `spliced-message-start` row.

**Rationale this change supersedes:**

- `2026-08-23-strict-streaming-text-runtime/design.md:108`: "premature EOF … produce at most one synthetic terminal internal error". This change narrows it.
- `2026-08-23-strict-unary-text-runtime/design.md:46-50`: "Use fixed safe errors", where provider status-less errors reduce to upstream. The rule is kept, with one more fixed document.
- `2026-10-07-gateway-raw-output/design.md:9`: "Nil or invalid values are dropped because a LanguageModelV4 raw part requires a value and the Go client rejects a raw event without one". This is removed for nil values. The privacy risk noted at `:19` is unchanged.

## Goals / Non-Goals

**Goals:**

- For the part sequences of `anthropic/upstream/spliced-message-start` and `openai/upstream/parallel-tool-call-wrapper`, both clients receive the shapes the direct goldens show: a status-less error, a value-less raw part, and a clean end after a final error. The provider-parts comparator (`test/conformance/provider_parts.go:148-156`) keeps an error's detail only when `statusCode > 0`, so these are the only facts that must match. On-`main` unit and contract tests pin them.
- Every existing status-bearing, transport, cancellation, timeout and malformed-status classification is byte-identical.
- No new exported Go API.

**Non-Goals:**

- Native error text in public base fields. Core stream-error semantics (#299) and native transport diagnostics (#323) are out of scope.
- Changing the drop rule for non-empty raw values that are invalid JSON, invalid UTF-8 or carry duplicate keys.
- Changing unary or setup errors, the HTTP error schema (`schema/error.json`), or `gateway-attempt-failure-evidence`.
- Root `test/integration` coverage. UI chunk types, serialization and SSE framing in the root module do not change, so no frontend wire behavior changes. A Go `StreamText` consumer over the Gateway sees one fewer trailing error chunk, which now matches the direct `expected.jsonl`. That difference sits at the ProviderWire boundary, and its cross-language evidence is the pinned-client contract suite under `ai-gateway/test/providerwire-v4`.

## Decisions

### D1. A stream-only execution category, `CategoryProviderStream`

`ClassifyStreamPart`'s status-0 fallback returns a new `execution.CategoryProviderStream` instead of `CategoryUpstream`. `ClassifyProvider` never returns it.

- **Alternative: map inside `providerwire/v4` only.** Rejected. gateway-execution-runtime requires adapters to pick their document "from the category and reason alone". v4 would have to inspect the error again, and the test adapter's `PartErrors` would keep reporting upstream for a fault that is not an upstream failure.
- **Enum order.** `CategoryProviderStream` is appended after `CategoryInternal` (`classify.go:18-32`), and `safeProviderStream` after `safeBYOKDiscovery` (`providerwire/v4/errors.go:7-22`), so no existing iota value changes.
- **Blast radius.** `execution.Category` is internal and used only by `providerwire/v4/errors.go` and `internal/testadapter`. The new category needs:
  - an entry in `safeCategories`;
  - a `safeProviderStream` document;
  - a row in `adapter_test.go`'s `unaryDocuments`;
  - a changed row in `classify_test.go:103` (`"zero status"`, stream column).
- **Unary stays upstream.** An HTTP response needs a status, and setup failures happen before any output. Keeping unary upstream preserves every unary document.

### D2. The fixed status-less document

The document is `{"message":"provider stream error","type":"internal_server_error","param":null,"code":"provider_error","retryable":false}`, with no `statusCode`.

- **Type.** `internal_server_error` is the type that every non-4xx Gateway document already uses: `overloaded`, `upstream_error`, `timeout`, `canceled` and `internal_error`. It maps to the existing `GatewayInternalServer`, so the Go client needs no new public constant.
- **Code.** `provider_error` is a new, distinct code: the provider reported a fault inside its stream. `upstream_error` means the Gateway could not reach the provider or got a 5xx from it.
- **Message.** `provider stream error` follows the lower-case noun-phrase style of the existing messages.
- **Implementation:**
  - the definition has `status: 0`;
  - `streamErrorResponse` allowlists it and leaves `StatusCode` at 0, so `omitempty` drops it;
  - `encodeHTTPError` (`error_response.go:198-207`) maps it to `safeInternal` before resolving the definition, so the HTTP and unary paths can never write status 0;
  - `definitionForError` stays a plain table lookup, because `streamErrorResponse` also calls it (`:215`). A remap there would turn every provider-stream frame into `internal_error`.
- **Alternative: reuse `upstream_error` without a status.** Rejected. The schema ties each code to one status and one retryability, and the Go registry is keyed on (type, code, status).

### D3. `retryable: false`, fixed

The value is false for four reasons:

- It matches upstream `APICallError`'s default when `statusCode` is absent.
- Direct Go OpenAI sets it explicitly for parse faults.
- The fault happened after output began, and the `IsRetryable` doc (`provider/api_call_error.go:35-37`) warns that a stream is not safe to replay.
- Go `NewAPICallError` also derives false for status 0 (`provider/api_call_error.go:71-83`), so the client's existing check that `retryable` matches the status-derived value still holds.

Alternative considered: copying the native `IsRetryable`. Rejected, because the value would no longer be fixed, it would leak a provider fact, and the schema could not pin it.

### D4. Value-less raw events

- **Behavior.** When raw output was requested and `bytes.TrimSpace(part.RawValue)` is empty, the handler writes `{"type":"raw"}`.
- **Implementation.** The event omits `rawValue` (`omitempty` on `streamRawEvent.RawValue`), and `validStreamEvent` accepts the empty case.
- **Unchanged.** Non-empty values still go through `projectRawValue`, and a value that fails projection is still dropped. The affected rows do not need this case: Go adapters never pass invalid JSON as a value. The duplicate-key and UTF-8 drops protect the credential projection.
- **Alternative: turn unusable non-empty values into value-less markers too.** This would follow upstream more closely, but it changes the projection contract and is outside #564.
- **Schema.** `raw.required` becomes `["type"]`. `{"type":"raw","rawValue":null}` keeps today's behavior: `null` is a valid JSON value.

### D5. Clean EOF after a provider error frame

- **State.** `streamState` gains `lastEventProviderError`. It is set after a `PartError` frame is fully written and cleared after any other frame is written.
- **Close.** On `WaitClosed` with the flag set, the handler returns without writing. The deferred `stream.Close()` (`stream.go:204`) still cancels and drains.
- **Any written error frame counts.** This includes a nil `APICallError` reduced to an in-place internal frame (`providerwire/v4/stream_test.go:132-160`); implementations must not narrow the flag to particular categories.
- **What does not count.** Parts that write nothing, such as unrequested or unusable raw parts, do not touch the flag. The handler's own synthesized `stream-start` is written before any provider error, so it cannot follow one.
- **Unchanged:**
  - canceled, deadline and idle waits;
  - adapter failures, including `streamrecover` panics, which are terminal and not forwarded errors;
  - a close before the first part.
- **Open blocks.** The flag is not tied to lifecycle state. In the spliced row the stream ends while a tool input is still open, and no closure is synthesized. This matches the direct stream and the "Adapter failure does not manufacture lifecycle completion" rule.
- **Alternative: only after status-less errors.** Rejected. "Error, then end" is the same native fact for any category, and a 429-then-close stream would still gain a `500` that the direct provider does not emit.
- **Alternative: after any earlier error.** Rejected. When content follows the error, the synthetic error is the only truncation signal.
- **Test adapter.** It keeps reporting `WaitClosed` as internal: clean EOF is a ProviderWire state-machine rule, not a runtime classification. The adapter cross-check (`adapter_test.go`) has no close-after-error scenario.
- **Existing Gateway tests.** Every service row whose stream ends with a provider error frame and then closes loses its trailing `internal_error`:
  - `cmd/grafana-ai-gateway/internal/service/fallback_acceptance_test.go:52` "tool input then error": `[stream-start, tool-input-start, error, error]` / `["overloaded","internal_error"]` becomes `[stream-start, tool-input-start, error]` / `["overloaded"]`;
  - `fallback_acceptance_test.go:286` "postcommit_error_close": `[stream-start, error, error]` becomes `[stream-start, error]`;
  - `generated_file_test.go:29` "file then error": `[stream-start, file, error, error]` becomes `[stream-start, file, error]`;
  - `providerwire/v4/stream_segments_replay_test.go` "overlapping-message-start" (from sequential response segments, #583): the written events now equal the direct golden, which ends with the provider error, instead of gaining a trailing `internal_error`.

  gateway-streaming-function-tools "Tool events commit without replay" (`openspec/specs/gateway-streaming-function-tools/spec.md:110-112`) still holds: it requires no replay, no duplicated tool events and no later candidate, not a trailing `internal_error`.
- **Race boundary.** `Stream.Next` (`internal/execution/runtime.go:238-260`) selects nondeterministically when a closed channel and `model.Done()` are both ready. A deadline or cancellation that races a provider close after an error may therefore end at clean EOF. This falls under the existing "Streaming terminal authority and scheduler race boundary" requirement; no precedence is added.

### D6. Go client registry

- **Presence.** `wireStreamError.StatusCode` becomes `*int`, using ordinary Go decoding. Under that decoding `"statusCode": null` decodes as absent (AGENTS.md "Canonical Request Casing"), so it is accepted for `provider_error` only and rejected for every other code. The server schema never emits null.
- **Status-less document.** `decodeStreamError` accepts a nil status only when the type is `internal_server_error` and the code is `provider_error`. It builds the cause with `StatusCode: 0` and an explicit `IsRetryable: &false`, and still requires `retryable` to be present and equal.
- **Every other code** still requires its registered status.
- **HTTP path.** It passes `resp.StatusCode`, which is never 0, so `provider_error` cannot match there.
- **Raw.** `stream.go:376` drops the `value.RawValue == nil` rejection, so the `PartRaw` carries a nil `RawValue`. Filtering at `:112` is unchanged.
- **API.** No `GatewayErrorCategory` is added and no exported symbol changes.

### D7. Privacy boundary (confirmed)

The issue scopes privacy to text "exposed by the new shapes", and `openspec/specs/gateway-attempt-failure-evidence/spec.md:56` already governs `nativeError` for every committed error. nara confirmed this reading.

The new shapes add no native exposure:

- The base fields are fixed.
- The value-less raw event carries nothing.
- Optional `error.data` keeps today's 502 behavior exactly: `providerMetadata` plus the event-local `nativeError` summary under gateway-attempt-failure-evidence, with configured-key redaction. `stream_test.go:416` already asserts `nativeError.message` for status-0 errors.

**Interpretation.** The issue says "no native error text reaches either client". This design reads that as no native text in base error fields or in any new member. Tests assert both:

- the native message is absent from `message`, `type`, `code`, `param` and `retryable` in both clients;
- `nativeError`, when present, equals the governed redacted summary.

If the stricter reading is intended, that would mean no `nativeError` for this category. It would change gateway-attempt-failure-evidence and is not planned here.

### D8. Delivery, evidence and parity records

- **On main.** This change is green on `main` by itself through:
  - Go unit tests (`internal/execution`, `providerwire/v4`, `providers/grafana`);
  - the stream-event schema tests;
  - the pinned-client contract suite:
    - a new testserver scenario in `runtime-integration.test.ts`, which exercises status-less errors, a value-less raw part and close after an error in both clients;
    - the raw-SSE fixture in `go-client-differential.test.ts:457-472`;
    - the updated raw expectations in `runtime-integration.test.ts:720-746`, because the existing testserver `raw` scenario (`testserver/providerwire_v4.go:282`) already emits a value-less part.
- **Acceptance.** The on-`main` tests above are the full acceptance evidence; no external conformance harness is a dependency. The scenarios in tasks 3.1, 6.1 and 6.3 reproduce the part sequences of the two affected rows as provider stream parts in unit and contract tests. No provider `input*.chunks.txt` fixtures are recorded or edited.
- **Parity records.** `knownGaps` and `PARITY.md` do not change. Neither mentions these rows, and the "AI Gateway" surface keeps its status and evidence location.
- **Divergence classification.** The three rewrites were Gateway spec rules (fixed safe errors, dropped value-less raw parts, synthetic close error), not `knownGaps` entries. Their effect on these rows is now a parity-preserving Go adaptation. Two intentional deviations remain:
  - the fixed message replaces native error text; the comparator ignores it for status-less errors;
  - the fixed `retryable:false`. It matches upstream `APICallError`'s default and direct OpenAI parse faults (`providers/openai/convert_stream.go:49-54`). It diverges from the direct Go OpenAI truncation path, which reports `IsRetryable=true` (`providers/openai/stream_preflight.go:233-241`). The comparator cannot see this difference.

## Risks / Trade-offs

- **Version skew.** `ai-gateway` and `providers/grafana` release independently (`release-please-config.json`), so shipping both in one PR does not control which reaches users first.
  - *Effect:* a released client rejects `provider_error`: status 0 matches no `registeredError` entry (`providers/grafana/errors.go:100-117`), and the stream ends at one terminal protocol error. On the OpenAI recoverable-parse path the direct stream continues after the error (`providers/openai/convert_stream.go:49-54`), so an old client that works today loses all later content and the finish. That is a functional regression, not only fail-closed. Raw-requesting old clients also stop at a value-less raw event. Nothing leaks.
  - *Mitigation:* client and Gateway ship in one PR. The Gateway plugin builds against same-revision client source through the `ai-gateway/go.mod` replacements, and the tolerant client still decodes old 502 documents. The PR description names the minimum `providers/grafana` version for the new stream shapes. The TS client is unaffected.
- **Retryability flip.** A status-0 error whose cause is neither `net.Error` nor `url.Error` now reports `retryable:false`. One example is a body-truncation error that OpenAI wraps with `IsRetryable=true` (`providers/openai/stream_preflight.go:233-241`). `net.Error` and `url.Error` causes keep upstream or timeout.
  - *Effect:* the Gateway's server fallback is unaffected: it decides on the native `IsRetryable` and never replays after the first part. But in-stream `retryable` is not purely informational. Root `fallback.WithBufferedStream` returns the first in-stream `PartError`'s `APICallError` (`fallback/buffered_stream.go:106-111`), and `DefaultDecider` returns `IsRetryable` (`fallback/fallback.go:337-349`); `test/integration/testserver/scenario_buffered_fallback.go` uses this. A buffered consumer of a Gateway-backed model used to fall back on these errors (502, retryable) and now stops.
  - *Mitigation:* classified as an intentional deviation in D8, and documented in the provider guide's retry guidance (task 2.4).
- **Metrics label.** Go-client consumers using `middleware/prometheus` see `status_code` change from `502` to `none` for these errors (`middleware/prometheus/errors.go:82-84`).
- **A stream may now end without finish and without a Gateway error.** Both clients already treat clean EOF without finish as completion, and core synthesizes an error finish, as the direct `expected.jsonl` shows.
  - *Mitigation:* the architecture walkthrough and provider guide describe the rule.

## Migration Plan

No data migration is needed. Client and Gateway ship in one PR; see Risks, Version skew. Roll back by reverting it. Gateway and client changes are independent of stored state. An updated client keeps decoding old 502 documents, so a Gateway rollback is safe for it.

## Confirmed decisions

nara confirmed:

1. **Delivery.** One PR carries the client and the Gateway changes (tasks group 1).
2. **Document names (D2).** `provider_error`, `provider stream error`, type `internal_server_error`.
3. **Privacy (D7).** Native text stays out of base fields and new members; `error.data.nativeError` stays under gateway-attempt-failure-evidence.
