## 1. Handler Mode and Limits

- [x] 1.1 Add failing construction tests for positive provider stream-part count, complete-frame, idle, and drain limits, including safe `limit+1` arithmetic and canonical start/error frame fit.
- [x] 1.2 Extend `v4.Limits`, embedded schema construction, and validation for `StreamParts` and the other streaming limits while preserving unary construction behavior.
- [x] 1.3 Add failing envelope and sequencing tests for exact `true`/`false` mode selection, invalid streaming values, shared pre-model stages, and `DoGenerate` versus `DoStream` dispatch.
- [x] 1.4 Refactor request validation and handler dispatch to return execution mode and route supported text requests through the correct model method without duplicating the strict pipeline.

## 2. Shared Safe Warnings and Anthropic Timing

- [x] 2.1 Add unary warning privacy tests covering hostile credentials, URLs, bodies, headers, provider/backend identity, arbitrary prose, empty values, unknown discriminators, canonical public identity, and cardinality rejection before mapped-slice allocation.
- [x] 2.2 Implement one value-safe warning mapper shared by unary and streaming output that emits only closed identifiers, fixed approved prose, and canonical public identity where required.
- [x] 2.3 Update unary response tests and raw expectations for normalized warning values without weakening response schemas, byte bounds, or client-overwrite evidence.
- [x] 2.4 Add Anthropic tests proving initial API failures remain `DoStream` setup errors and successful preflight emits one `PartStreamStart` before handling/emitting the pre-read event, with warnings absent from `PartFinish`.
- [x] 2.5 Move Anthropic streaming warnings to the post-preflight initial provider start while preserving all later event order, initial-error promotion, and finish values.
- [x] 2.6 Run `mise run test-anthropic`; regenerate derived conformance expectations only if unchanged provenance-valid inputs are affected, and verify no `test/conformance/**/input*.chunks.txt` file changed.

## 3. Private Stream DTOs and Bounded Framing

- [x] 3.1 Add an embedded closed draft 2020-12 stream-event schema and positive/negative tests for normalized start warnings, canonical metadata, text events, finish, and the closed safe error object.
- [x] 3.2 Add private stream DTO mapping and incremental complete-frame encoding tests covering required empty values, invalid UTF-8, exact below/at/above frame boundaries, warning cardinality prechecks, and canonical fallback frames.
- [x] 3.3 Implement bounded `data: <json>\n\n` encoding that counts the full frame, schema-validates before write, and never first accumulates an oversized provider value.
- [x] 3.4 Add response-writer tests for full and short writes, commitment-header and per-frame `http.NewResponseController(w).Flush()`, direct and wrapped `http.ErrNotSupported`, separate header/frame flush failures, panic recovery, exact SSE headers, no `event:` field, and no `[DONE]`.
- [x] 3.5 Implement identical header/frame flush classification with `errors.Is(err, http.ErrNotSupported)` so unsupported flushing is tolerated and every other observable writer failure cancels provider work without a second write.

## 4. Setup Ownership and Commitment

- [x] 4.1 Add setup tests for provider errors, panic, `nil, nil`, nil channel, result-plus-error with a channel, private `StreamResult` metadata, valid commitment, and result-plus-error JSON latency independent of drain duration.
- [x] 4.2 Add deterministic ownership tests that make outcome/cancellation/timeout signals ready before invoking one precedence snapshot, plus post-snapshot conditions, already-ready outcomes, late outcomes, handler claim, worker abandonment, and exactly-one asynchronous drain.
- [x] 4.3 Implement an atomic pending/handler-owned/abandoned setup handoff with readiness and ownership-decision signals so buffered notification cannot transfer cleanup implicitly.
- [x] 4.4 Implement the ownership precedence snapshot as the linearization point, claimed-valid logical commitment, post-snapshot SSE handling, and asynchronous single-owner cleanup for every invalid, result-plus-error, late, or abandoned non-nil channel.
- [x] 4.5 Add and implement no-first-part behavior so established streams remain SSE and attempt empty start plus the applicable terminal error after cancellation or timeout.

## 5. Start, Cardinality, Metadata, and Text State

- [x] 5.1 Add start tests for provider start present/absent, normalized known and generic warning variants, hostile warning privacy, duplicate/late start, oversized warnings, and warning-count rejection before allocation.
- [x] 5.2 Implement first-part pre-read and exactly-one public start normalization through the shared safe warning mapper, including empty start plus terminal fallback when warnings cannot be represented.
- [x] 5.3 Add below/at/above `StreamParts` tests counting start, metadata, errors, text, and finish, plus a continuously ready small-block flood that proves bounded retained ID state without timeout scheduling.
- [x] 5.4 Implement pre-interpretation provider-part counting so the first excess part cannot mutate lifecycle state, grow the ID set, or emit provider-derived output.
- [x] 5.5 Add table-driven transition tests for canonical metadata/privacy, metadata duplication/placement, sequential text blocks, empty deltas, active-ID matching, empty/invalid/reused IDs, and overlapping blocks.
- [x] 5.6 Implement the closed text state machine with canonical response metadata, globally unique non-empty UTF-8 text IDs bounded by `StreamParts`, one active block, ordered events, and required empty delta preservation.
- [x] 5.7 Add unsupported-family and hostile provider-metadata tests proving reasoning, tools, approvals, files, sources, custom, raw, and provider metadata terminate safely without provider-domain serialization.

## 6. Provider Errors and Finish Authority

- [x] 6.1 Add tests for one and multiple provider errors before/between/within text blocks, later content and finish, nil/malformed errors, closed status/retryability fields, and hostile-detail privacy.
- [x] 6.2 Implement independent non-terminal `PartError` reduction through the existing safe-category table without changing stream lifecycle state.
- [x] 6.3 Add finish tests for finish-only and text streams, normalized usage and reasons, invalid/unsafe usage, active blocks, finish warnings, premature EOF, duplicate finish, and post-finish provider floods.
- [x] 6.4 Implement finish validation as the authoritative final public event, immediate clean EOF without waiting for channel close, premature-EOF synthetic failure, and post-finish suppression during asynchronous drain.
- [x] 6.5 Add tests proving lifecycle, mapping, unsupported-output, part-limit, and frame failures emit at most one synthetic terminal adapter error without synthetic block ends or finish.

## 7. Controllable Precedence and Deadline-Authoritative Drain

- [x] 7.1 Add one protocol-local controllable clock/timer test seam and pure precedence-arbiter tests for caller cancellation over total timeout, total over idle timeout, deadline equality, and deadlines over newly observed provider error/finish.
- [x] 7.2 Implement explicit total/idle timers from that clock for both public arbitration and model-context cancellation; do not retain an independent real-time streaming `context.WithTimeout`.
- [x] 7.3 Add fake-clock tests proving total and idle expiry cancel provider context before timeout encoding/writing, plus pre-deadline finish authority and idle reset after consumed start, metadata, text, and provider errors.
- [x] 7.4 Add drain tests for normal close, delayed close, result-plus-error streams, late setup, post-finish values, exactly-one drain, and pre-commit JSON/handler latency independent of drain duration.
- [x] 7.5 Add permanently ready flood-channel drain tests proving termination at deadline equality or the first excess request-scoped provider part even when receives are continuously selectable.
- [x] 7.6 Implement one asynchronous drain per ownership winner with the shared provider-part counter and an absolute deadline checked before and after every receive, using a timer only as wake-up.

## 8. Contract Replay and Cross-Language Evidence

- [x] 8.1 Update Go golden replay tests so `streaming.json` and the streaming `sequence.json` record execute `DoStream` once with exact mapped options while all unary records retain their intended outcomes.
- [x] 8.2 Add raw production-handler SSE tests for headers, exact frames, safe warning values, canonical metadata, empty/non-empty deltas, ordered provider errors, finish, immediate clean EOF, privacy, schemas, part limits, and frame limits.
- [x] 8.3 Extend `test/providerwire-v4` with un-aborted pinned-client scenarios for normal streams, multiple ordered provider errors followed by content, adapter timeout, finish, and clean EOF without `[DONE]`.
- [x] 8.4 Add a separate established-stream abort scenario: return a non-nil silent channel, await pinned-client `doStream()` resolution and HTTP commitment, then abort and assert server-side recording-provider context cancellation.
- [x] 8.5 Add or extend deterministic `test/integration/testserver` and matching Vitest coverage for ProviderWire SSE behavior crossing the frontend wire boundary; do not create synthetic provider conformance input.
- [x] 8.6 Update `test/conformance/PARITY.md` to classify fixed-prose unary and streaming warning normalization as an intentional privacy deviation, classify strict streaming text evidence accurately, and retain explicit gaps for every deferred stream family and permissive-client evidence boundary.

## 9. Validation

- [x] 9.1 Run `mise run fmt`, `go test ./gateway/providerwire/v4`, and `mise run test-anthropic`; fix all focused failures.
- [x] 9.2 Run `go test -race ./gateway/providerwire/v4` to validate setup ownership, shared clocks and timers, part counters, and asynchronous drain cleanup.
- [x] 9.3 Run `mise run test-providerwire-v4` and `mise run test-integration`; verify normal ProviderWire checks do not rewrite tracked goldens.
- [x] 9.4 Run `mise run parity-check` and inspect every conformance expectation diff for matching registered-baseline behavior and valid input provenance.
- [x] 9.5 Run `mise run build` and `mise run test` across all modules and examples.
- [x] 9.6 Run `mise run vet`, `mise run lint`, and `mise run test-short`; remove debug code, unused imports, and temporary files.
- [x] 9.7 Run `openspec validate strict-streaming-text-runtime --strict`, review the final diff against the revised phase 4 authority and upstream commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`, and document residual provider lifecycle risks.
