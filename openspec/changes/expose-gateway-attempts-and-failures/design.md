## Context

This rewrite keeps #370 open but starts its implementation from #373 at ea668bef. The historical 4d0f9bfc implementation remains backed up and is reference material, not code to mechanically migrate. The consolidated foundation is archived and synced; this branch owns only expose-gateway-attempts-and-failures.

Reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18, Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. The registered TypeScript Gateway spreads unary metadata, passes stream parts through and maps setup errors with their original API-call cause. The Go client retains bounded original HTTP/SSE error data. There is no upstream shared fallback implementation or private Gateway overview contract. PARITY classifies runtime, provider contract and frontend evidence separately.

## Goals / Non-Goals

**Goals:** Deliver actual configured destination outcomes and useful protected failures independently of operator capture, without changing execution policy or invalidating primary output for optional attribution.

**Non-Goals:** Trace/history frameworks, SDK serialization, mutable shared-model execution state, raw transport diagnostics, retry/classification changes, readers/executors/cleanup owners, selected indexes, completion/replay claims, native identity duplication or client acceptance redesign.

## Decisions

### Immutable route context, not instrumentation wrappers

The resolver currently returns only canonical ID and model; configured descriptors live in catalog metadata and actual credentials in service configuration. Carry copied configured descriptors and private protected-source values in the resolved invocation context. Static construction keeps sources out of public ModelInfo/discovery and excludes them from JSON. Registry/custom resolution can omit unavailable configuration rather than invent account identity. No request value selects destinations.

Keep configured candidates and the existing model observer unchanged. Request observation uses fallback.WithAttemptObserver; its independent panic recovery and delivery order already belong to the foundation. Do not reinstall candidate wrappers or replace the model's operator callback to collect consumer output.

### Call-local decisions and existing ownership

A small synchronized handler-owned accumulator stores only one invocation's fallback decisions and seals when publication is finalized. No global state, diagnostic history cap or per-stream error history. Mixed/incomplete observation is omitted as the foundation requires; cancellation after intent does not invent another invocation. Late callbacks cannot rewrite a published snapshot. Unary worker outcomes rejected by the existing cancellation/timeout boundary discard unverifiable fallback history and seal capture there, before response construction; configured direct cancellation retains its existing locally owned outcome. This does not change fallback SourceErr preservation or result ownership.

For a configured direct route, observe actual call entry and the handler-owned setup/result/first-part boundary. Selection means a non-invalid unary result or first received provider part, including error. Preserve direct HTTP commitment timing, cancellation ownership, setup select behavior and bounded drain. Never turn a direct route into one-candidate fallback. Do not attribute an unowned late native failure to cancellation.

### Optional carriers and current failures

Use execution.Project and execution.Metadata. Unary success and finish carry providerMetadata.gateway.execution. Unary/setup errors add top-level providerMetadata; committed errors add error.data.providerMetadata. Current committed native summaries remain event-local in error.data.nativeError, using the same small protected fields as attempt summaries; they are not added to selected-attempt/finish history. Current classification, status and client retryability remain unchanged.

Use existing complete-response or complete-SSE-frame validation and limits, including original-byte metadata preflight, before accepting enrichment. If optional summary/overview encoding cannot fit, preserve the existing primary document/frame rather than emit a new failure. Native gateway collision relocates intact only when fitting; otherwise native metadata remains opaque. Existing invalid primary output remains an adaptation failure, without fallback replay. No essential/detail quotas, truncation, dispositions or error-detail collapse.

Summary extraction uses the caller's existing response/source bound and actual credential/tenant sources, not a new diagnostic allocation. Request credential sources stay in the invocation context and never become output or discovery data. Explicit Anthropic MCP authorization tokens, OpenAI MCP authorization/recognized credential headers, and known credential query/userinfo values in their named destination URLs join the actual configured keys and inbound/body-carried protected headers; this is not an application-data scan or new request admission policy. Complete HTTP errors additionally stay within the independent Go client's existing 64 KiB default read bound, not a per-diagnostic quota. Unknown internal/panic/aggregate errors do not become arbitrarily attributed native error trees.

### Evidence and client access

Rewrite old assertions instead of preserving their shape. Retain actual request-count, ordering, direct/preselection, late-result, namespace, credential, shared-model and saturated physical-sink setups. Both registered TypeScript and independent Go clients must exercise success, noneligible/exhausted/setup/committed paths and consumer middleware; Go GenerateText is a streaming invocation, not unary DoGenerate proof. Frontend scenarios parse UI SSE with registered schema and assert assembled error/content/finish behavior without automatic diagnostic text exposure.

Synthetic endpoints remain scoped wire/behavior witnesses, not recorded provider inputs or live/private-service parity. Full checks and limitations are recorded afresh; historical review/test counts do not approve this rewrite.

## Risks / Trade-offs

- Optional overview omission → document that absence does not prove no attempts and namespace presence does not establish provenance.
- Cancellation/late callback races → seal under synchronization; only existing ownership decides results and stream parts.
- Configured credential exposure → separate private invocation sources from public catalog/discovery and JSON; test cloning and output protection.
- Primary adaptation failures → preserve strict validation and no replay, without pretending selection proves completion.
- HTTP retry repetition → preserve status-derived client policy and document caller retry controls; no exactly-once claim.

## Migration Plan

Rebuild on the archived #373 foundation, rewrite this one OpenSpec change, implement with failing runtime regressions first, update schema/client/frontend and command evidence, then validate/commit/push with an explicit lease against 4d0f9bfc. Rollback removes runtime enrichment while leaving the shared SDK and pure projection foundation. No module pins/minimums change; candidate-source builds use go.gateway.work. Do not sync/archive or merge #370 without further approval.

## Open Questions

None for the approved scope. Stop if ownership, carrier or protection work requires a material departure.
