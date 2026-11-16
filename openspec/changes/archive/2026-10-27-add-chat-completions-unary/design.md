## Context

Restacked on 2026-10-28 on the #581 host-contract head, including the #555
execution runtime and #579 attempt visibility. The October 27 draft used a
resolver and private worker; those are superseded by this design.

Registered upstream: b7f6255acaae7724adc0fd37aac3dae3f3466578, ai 7.0.143,
provider 4.0.29, OpenAI 4.0.96, Anthropic 4.0.82, compatible 3.0.71.
The matching native source has been inspected: Responses defaults store and
strict JSON schema to true, so Chat supplies its own false defaults per native
attempt. Parallel settings use backend-specific names. Native Chat is a
Gateway extension, not ProviderWire/UI conformance or a Go SDK Chat provider.

## Goals / Non-Goals

Support the existing unary text/function/structured/reasoning subset through
the shared runtime. Do not implement a second execution engine, BYOK controls,
server tools, authentication or production routing in this unary change.
Streaming and route/client integration remain separate changes.

## Decisions

### Runtime-owned selection and execution

New accepts a non-nil execution.Runtime. The handler validates Chat input,
obtains Runtime.Call, defers Call.Close and invokes Call.Generate. Runtime owns
selection, host policy, native execution, context handling and attempt capture.
Chat has no local selector, worker or raw error classifier. It maps
ClassifySelection/ClassifyProvider categories to fixed OpenAI envelopes.
HostErrorWriter implements host.ErrorWriter for the shared host categories.

The current runtime calls selection and provider setup synchronously with
context and panic containment; it does not promise to interrupt functions that
ignore cancellation. Do not resurrect the prototype's worker to claim such a
bound. The host owns body size and total request duration. No Chat byte cap or
model-duration setting is added.

### Per-attempt translation

chatTranslator captures the validated request settings and is passed through
execution.WithTranslator. It receives execution.Target and maps store,
strictness and parallelism for that native backend/namespace while copying
options. The host's generic Translate wrapper applies it separately to each
configured or BYOK attempt, inside fallback, outside native validation, and
re-checks protected headers/namespaces. Chat installs no wrapper in catalog.
No translator means native options are unchanged for other protocols.

### Closed mapping and unary output

The explicit schema and typed mapping keep duplicate-last semantics, exact
keys, function history correlation and restricted JSON-schema vocabulary.
Output has one choice, optional truthful usage, represented finish reason,
canonical native identity/fallback and successful grafana.warnings. Raw native
bodies/headers are not exposed. Chat retains schema, tool-choice, parallelism
and representability checks; provider-wide validation has not yet moved to
#483. That migration must preserve Chat-specific refusals.

## Risks / Trade-offs

#483 remains a prerequisite to readiness. The #394 track is not complete merely
because runtime and host contract are now used. The handler buffers unary
input/output and relies on host resource ownership. Synthetic tests establish
mapping/client consumption, not live providers, deployed Grafana or future
lifecycle behavior. #579's shared attempt visibility does not introduce a new
Chat attempt-overview field or a Chat BYOK credential surface.

## Migration Plan

Preserve the protocol subset and tests while replacing prototype execution
ownership. Integrate streaming and host registration in their own changes.
Revalidate after the shared lifecycle change; rollback removes this adapter
without changing ProviderWire or the generic host contracts.
