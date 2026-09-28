## Context

The registered reference (`test/conformance/upstream.yaml`) is Vercel `4e8c387622ee1bb0d55841664416d38754d5c9a3`, `ai` 7.0.109 and `@ai-sdk/react` 4.0.112 (the issue cites older 7.0.107/4.0.110). Pinned `use-chat.ts` forwards `regenerate`, `resumeStream`, `onData`, `onFinish`, and `onError` into `AbstractChat`. Pinned `chat.ts` truncates messages before regeneration; reconnect GET uses `/chatId/stream`, returns `null` on 204, and does not enter `submitted` until a stream exists. Pinned `use-completion.ts` gates completion writes with `requestIdRef`, but passes loading, error and callback operations into `callCompletionApi`; do not assume those are all gated. Pinned `use-object.ts` retains partial object on stop, ignores abort errors, invokes `onError` on transport errors and `onFinish` on normal close (including schema failure). Relevant pinned upstream UI hook tests are `use-chat.ui.test.tsx`, `use-completion.ui.test.tsx`, `use-object.ui.test.tsx`; their mocked transports are behavior references, not Go/HTTP evidence.

`test/integration/react-hooks.test.tsx` uses actual hooks and the scenario server; chat exposes only send/stop/status/text/error, completion only single requests, and object only value/onFinish. Existing Go controlled streams and error scenarios are request-scoped, while `test/integration/chat-overlap.test.ts` exercises AbstractChat without React/Go HTTP. The `integration-testing` spec owns this test coverage and `test/conformance/PARITY.md` labels React hooks and UI/SSE mixed.

## Goals / Non-Goals

**Goals:** Make the missing #214 behaviors observable through real React hooks and Go HTTP, with reliable state/identity/callback assertions and schema-parsed SSE checks where new wire events are introduced.

**Non-Goals:** Public Go APIs, a persistent resumable-stream service, real provider recordings, changes to the registered versions, claims of exhaustive parity, or fixing unrelated upstream hook internals.

## Decisions

1. Extend focused hook probes in `react-hooks.test.tsx` to expose immutable message snapshots (including IDs, metadata, and data parts), status/loading histories, callback call records, and stop/regenerate/resume actions. Compare exact observable outcomes to the pinned hook behavior, with bounded synchronization around the first partial and terminal phases instead of arbitrary sleeps. Alternative: only parse SSE or call AbstractChat directly; neither establishes React hook behavior.
2. Use distinct, deterministic request-scoped Go scenarios. For regeneration inspect the submitted `trigger`/message history where relevant, produce distinguishable first/second response text and stable explicit IDs where needed, then check retained user identity and replacement assistant identity. For reconnect use a test-only GET route (via `prepareReconnectToStreamRequest` if convenient) returning deterministic UI SSE, 204 for no stream, and an HTTP error case. Seed a fixed chat ID and initial messages or produce a known response locally; test what the hook actually does with stream message IDs. The GET response is a test transport fixture, not replay of stored server events. Alternative: build a durable stream registry; not authorized or necessary for these hook-state tests. If a requested assertion truly needs persistence beyond fixture behavior, stop for owner decision.
3. Produce metadata/data chunks with existing Go UI chunk/SSE writer functions where possible; assert `onData` receives both retained and transient data, while transient data never enters message parts; also check assistant metadata updates, ordered retained parts, and final callbacks. For new SSE cases additionally parse with `parseJsonEventStream` + `uiMessageChunkSchema`, assemble via `readUIMessageStream`, and check fields. Do not label hand-authored provider events as recorded conformance input. Alternative: only assert raw text; it cannot prove structured updates.
4. For completion overlap, use two prompts with distinguishable response phases and bounded request-local synchronization; exercise old request completion/error while the new stream remains pending and check the pinned ownership semantics: an older request's `onFinish` still receives its prompt/result or its distinguishable error still reaches `onError`, but its settlement cannot replace the newer displayed completion/error or clear newer active loading. `requestIdRef` gates completion writes and `callCompletionApi` guards settlement's error/loading cleanup by current abort controller identity; it does not suppress older callbacks. For object, reuse HTTP-error and abortable partial-stream cases, add a Go transport-failure scenario that errors the body read *after* valid partial JSON (e.g. truncated declared response body), and assert the difference from a normally ended but schema-invalid JSON stream. Alternative: malformed JSON with ordinary EOF only tests validation/onFinish, not a read failure.

## Risks / Trade-offs

- [Truncated-body behavior varies by fetch runtime] → Validate the transport-error case against pinned Node/Vitest runtime; use a deterministic HTTP close after flushed partial bytes, not a fabricated provider fixture. If it cannot reliably produce a body read error, report coverage gap rather than passing a schema-error test as stream failure.
- [Overlapping hooks have non-intuitive shared state] → Record per-request inputs/callback counts and compare to pinned implementation/tests; explicitly distinguish latest rendered value from callback ownership; do not assert idealized cancellation behavior.
- [Reconnect fixture does not prove durable resume] → State the test-only boundary in assertions and coverage report, keep PARITY mixed.
- [Concurrent test scenarios leak cross-request state] → Keep data per request or use independent, bounded synchronization; cancellation releases blocked handlers.

## Migration Plan

Tests/scenarios only; no deployment or data migration. Revert new integration files to roll back. Run `mise run test-integration`, and `mise run parity-check` if committed parity behavior/fixtures change; retain `mixed` unless evidence materially changes.

## Open Questions

None requiring a product/API decision at planning time. If testing requires server-side persisted replay, stop and seek explicit approval rather than implementing it as part of #214.
