## Context

The existing fallback observer already reports one closed decision per candidate invocation. It is configured on the model, which is normally shared, and sees only the decision error after cancellation normalization. The registered upstream baseline is ai 7.0.118/provider 4.0.18 at `5d12eaa6caa193d3901cbab98a734403eb6bf622`; its APICallError keeps native data separate from the error cause. There is no matching upstream fallback implementation. These observation APIs are a Go adaptation, not a private Gateway parity claim.

## Goals / Non-Goals

**Goals:** Let a caller observe attempts through its request context; retain the candidate-local error already owned by fallback; preserve all fallback policy and lifecycle behavior.

**Non-Goals:** Automatic metadata serialization, a trace/report framework, mutable model-global execution history, new native transport capture, Gateway credentials or account identity, and changing errors returned to callers.

## Decisions

- Add package-level `WithAttemptObserver(ctx, fn)`, distinct from the existing model method. It creates a context value without mutating the model. The nearest registration replaces the inherited request observer; a nil callback disables it. Callers own their accumulation and synchronization.
- Invoke the request callback and existing model callback independently at the same decision boundary, request first. Recover each panic independently. Both receive the same decision snapshot, including timestamp. Neither callback can retroactively change the selection decision.
- Add `Attempt.SourceErr` alongside `Err`. Save the setup/validation/peek error before calling the decider. Keep `Err`, decider inputs, returned errors and outcomes unchanged. If cancellation owns setup before a native result is transferred, the source is that context error, not a later unowned native failure.
- Request observers see fallback invocations using that context, including repeated core steps and nested fallback models. Indices restart for each invocation. This API is an observation hook, not an automatically grouped execution trace. Independently captured calls use independently scoped contexts.
- Do not add a collector or serialize Go errors automatically. Gateway projection stays in the AGPL module and consumes the hook through request-local state.

## Risks / Trade-offs

- Blocking callbacks block the existing decision boundary → document prompt return; do not add goroutines or queues.
- Native errors can contain secrets → keep them in-process and require application-owned projection before publication.
- Concurrent callbacks into a caller's accumulator → document synchronization; test independent request scopes with one shared model under the race detector.
- Late provider setup results → preserve existing ownership and cleanup; do not publish unowned results.

## Migration Plan

Existing model observers continue to work without registration changes. Gateway source builds use the same-revision SDK through `go.gateway.work`; no unmerged published-module pin is introduced. Runtime activation and Gateway wire representation remain separately scoped.

## Open Questions

None for the shared observer API. Optional Gateway enrichment versus a native namespace collision at the complete-envelope limit remains an activation policy decision, not part of this SDK change.
