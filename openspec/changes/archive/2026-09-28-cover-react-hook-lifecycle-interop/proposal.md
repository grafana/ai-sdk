## Why

Issue grafana/ai-sdk#214 remains OPEN and valid: the actual React hook integration suite proves selected success, error, cancellation and approval flows, but not chat regeneration/reconnect, metadata/data and callback behavior, completion overlap, or object failure/stop lifecycle. Chunk snapshots and a separate non-React AbstractChat overlap test do not establish those behaviors across the Go HTTP boundary. The registered reference is now `ai` 7.0.109 / `@ai-sdk/react` 4.0.112 at `4e8c387622ee1bb0d55841664416d38754d5c9a3`, superseding the versions quoted in the issue; treat the remaining difference as a coverage gap, not a demonstrated implementation defect.

## What Changes

- Extend deterministic Go testserver scenarios and actual-hook Vitest probes to check useChat regeneration, reconnect, state/message identity, callbacks, and metadata/data updates.
- Cover overlapping useCompletion requests and observable completion, loading and callback ownership without assuming behavior contrary to the pinned hook implementation.
- Cover useObject HTTP and stream failures, cancellation, retained partial values, and onFinish/onError behavior.
- Preserve `mixed` React hook/UI coverage classification until sufficient breadth is proven; do not add public Go APIs, server-side durable stream storage, or provider fixture inputs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `integration-testing`: Strengthen the existing React hook lifecycle and failure integration coverage requirement with regression scenarios for the remaining #214 paths.

## Impact

Planning targets `test/integration/react-hooks.test.tsx` and request-scoped Go scenarios in `test/integration/testserver/`; any added UI SSE case should use existing `parseJsonEventStream` + `uiMessageChunkSchema` and message assembly checks. No runtime API or dependency upgrades. `test/conformance/PARITY.md` stays mixed unless the coverage boundary changes; run integration validation and applicable baseline checks during implementation.
