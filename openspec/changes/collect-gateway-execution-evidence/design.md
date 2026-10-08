## Context

PR #373 is the dormant foundation for #322, based on main after #372. The owner approved a compact overview rather than an execution trace, reusable fallback capture first, no selected index, no diagnostic allocation matrix, and preserving the primary response when native namespace relocation cannot fit. The SDK stage is implemented in `f8dffbfe`; its formerly separate OpenSpec scope is consolidated here so this PR carries one change.

The baseline remains ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at `5d12eaa6caa193d3901cbab98a734403eb6bf622`. The pinned APICallError source keeps native data separate from causes. There is no matching upstream fallback implementation; observation is a Go adaptation. The Gateway-owned overview is an extension, not private-service parity. PARITY.md classifies shared fallback and dormant Gateway projection separately.

## Goals / Non-Goals

**Goals:** Capture reusable request-scoped fallback decisions and safely project a compact explanation of actual observed attempts and selection, independent of operator capture.

**Non-Goals:** A trace/audit framework, model-global last execution, automatic SDK serialization, a new diagnostic accessor, new readers/executors/retries/cleanup owners, native transport capture, production activation or #370 restacking.

## Decisions

### Shared fallback observation

Package-level `fallback.WithAttemptObserver(ctx, fn)` observes requests without mutating a shared model. The nearest registration replaces inherited request observation; nil disables it. Request and model callbacks run synchronously at the existing decision boundary, request first, with the same snapshot and independent panic recovery. Callbacks must return promptly and synchronize shared accumulation.

`Attempt.SourceErr` preserves the setup/validation/first-part-wait error owned before decision-time cancellation normalization. Existing `Err`, decider inputs, outcomes, returned errors and cleanup are unchanged. Cancellation that wins setup ownership exposes its context error, not an unowned late native result. Accepted error parts remain selected stream content, not setup failures.

Repeated SDK steps, retries and nested fallbacks are callback events with invocation-local indices, not automatically grouped traces. The runtime integrator owns call-local accumulation, synchronization and finalization. Only one completed fallback invocation is projected into an overview; incomplete or mixed observation is omitted rather than presented as a complete history.

### Pure Gateway projection

Remove the Gateway collector and model wrappers. A pure projector takes route identity, observed decisions, configured candidate descriptors and actual protected sources. It emits one ordered array with provider/backend model, optional configured instance, outcome and optional error summary. Configuration supplies account identity; Apache fallback supplies decisions and candidate-local errors. A direct call is not wrapped in a one-candidate fallback; later runtime integration supplies its independently observed decision using existing lifecycle ownership.

Route identity appears once. Array order identifies candidates; no selected index is needed. A selected outcome means usable unary result or first accepted stream part, not successful completion. All prior outcomes must be failed and only the last may be selected or canceled. Unrun configured candidates are never emitted. Post-selection errors remain the existing error carrier and are not collected into finish history.

### Small native summaries

Retain only message, type, string/number code and status. No error details, headers, request bodies, arbitrary cause serialization, retryability or error classification is duplicated. Follow only a single error chain; do not assign an aggregate error member to a candidate.

Decode only the top-level and conventional nested error objects with standard Go JSON. Numbers are re-encoded without float conversion. Known protected scalar fields, actual configured/request credential values and credential-bearing URL values protect summary echoes. Ordinary endpoints, identifiers and token-looking application data are not DLP-filtered. Summaries with protected echoes are omitted field-by-field. No raw diagnostic data is republished or mutated.

The integrator supplies its existing source/transport read bound; there is no evidence-specific source quota. Malformed or oversized diagnostics degrade to available safe status rather than returning an assembly error. Complete response/frame limits remain the protocol adapter's existing protections.

### Optional metadata enrichment

The namespace is `gateway.execution` with requested/canonical identity and attempts; a native `gateway` value moves intact under `gateway.nativeMetadata` when enrichment fits. Other namespaces remain opaque and unchanged. The assembler never merges native fields into execution facts or mutates inputs.

The caller supplies a fit check including primary validation against the complete response or SSE frame, not a new overview byte budget. The original envelope must pass before enrichment can be accepted, so relocation cannot repair a primary-invalid namespace. If encoding or that complete-envelope fit check fails, return the original provider metadata without truncation or new errors. This also applies when relocation alone cannot fit: preserve the primary response and leave the native namespace opaque. Namespace presence alone is not provenance or authority; an omitted overview is not evidence of no attempts. Existing primary encoding failures remain primary protocol behavior.

### Schema and scope

The compact JSON schema documents/tests the Grafana extension, not a runtime validator or Apache client dependency. Package and schema tests do not establish handler delivery. Gateway/workspace and client minimums remain Go 1.26.3.

This owner-approved revision supersedes #321's evidence shape, quotas, dispositions, mandatory attribution allocation and completion/replay-risk claims. Standard Go diagnostic duplicate/string normalization and independent client raw HTTP/SSE retention remain unchanged.

## Migration Plan

Keep SDK and Gateway source changes in this single OpenSpec/PR; same-revision Gateway builds use `go.gateway.work` without unmerged module pins. Replace `internal/evidence` with `internal/execution`, update the namespace schema and focused tests, and remove unused machinery. #370 must be adapted separately to the request observer and pure projector, preserve direct-call lifecycle and existing error/content/finish ordering, and apply complete-envelope fit checks. Do not archive, merge or restack #370 here.

## Risks / Trade-offs

- Optional overview omission → callers must not infer no attempts or native/Gateway provenance from namespace presence alone.
- Unsafe native diagnostics → publish only shallow protected summaries, never whole Go errors or diagnostic trees.
- Mixed/incomplete callback histories → reject optional projection rather than invent attribution; finalization belongs to the runtime integrator.
- Blocking callbacks → preserve the documented synchronous contract rather than introducing queues/goroutines.
- No live boundary proof → retain dormant support status until separately reviewed production activation and both-client tests.

## Open Questions

None for this foundation. Production accumulation/finalization and complete-envelope integration remain explicit #370 work.
