# Proposal

## Why

ProviderWire V4 owns model execution, failure classification, attempt capture and host request policy, as well as its wire protocol. A second adapter has to copy all of it, and #248 did, with weaker guarantees (#394). This change moves the protocol-independent parts into one runtime that every adapter shares. It also removes runtime work and request policy that protect nothing the platform does not already protect (#481, part of #394).

## What Changes

- **Shared runtime.** `ai-gateway/internal/execution` gains a `Runtime` → `Call` → `Stream` API that ProviderWire V4 uses as a codec:
  - the runtime selects the model, invokes it, classifies failures, captures attempts and runs the stream loop: wait outcomes, idle timer and one bounded drain;
  - it never writes HTTP responses. Adapters own commitment, framing, their error documents and their state machine.
- **Moved types.** `Selection`, `RequestSelector`, `CatalogSelector`, the host sentinels and `Limits` (idle and drain durations, validated) move from `providerwire/v4` to `execution`. `v4.New` takes a constructed runtime.
- **Synchronous execution.** Selection, `DoGenerate` and `DoStream` run on the request goroutine. One recover at the runtime boundary contains panics and `nil, nil` results:
  - a result that arrives after the request context ends is discarded, and the context outcome is reported;
  - a stream returned after the context ends is canceled and drained once.
  - **Removed:** the selection, unary and stream-setup worker goroutines. A selector or model that ignores cancellation now holds its own request until it returns. The platform's route deadline still ends the caller's request.
- **Closed failure classification.** The runtime classifies every outcome into a closed set of categories and reasons. ProviderWire keeps only the mapping from category to its fixed document.
- **Attempt capture for every selection.** Captures are request-local. BYOK attempts are captured in order but not published (#420). The configured overview projection is unchanged.
- **Host request policy, reduced to what protects something:**
  - the runtime extracts call-level `gateway` controls for the selector;
  - it refuses credential-bearing call headers with a closed reason;
  - ProviderWire keeps the protocol rules: duplicate header spellings fail, and `ai-language-model-*` and `ai-o11y-*` names are not forwarded.
  - **BREAKING (caller):** the `grafana` and `grafana-ai-sdk` namespaces, and `gateway` below call level, are no longer reserved. No Gateway code consumes them, so they reach native models like any other namespace.
  - **BREAKING (operator):** an openai-compatible `providerName` may now be `grafana` and may no longer be `gateway`, which the host owns at call level.
- **Settings.** `Settings.ProviderWire` is removed. Production uses the runtime's fixed default limits, and config validation no longer constructs a handler.
- **Test-only adapter.** A minimal adapter over the runtime shows the same outcomes as ProviderWire for identical fake models.
- **Docs.** Package godoc for `execution` and `providerwire/v4`, the architecture walkthrough and the glossary. The operator limit ledger in `ai-gateway/README.md` stays as it is: its idle and drain rows still say "Gateway".

### Revised #394 and #481 decisions

- "ProviderWire's observable behavior changes only where size limits are removed": it also changes where the policies above are removed. All of those requests used to fail with a fixed 400.
- #481's selection, unary and stream-setup workers are dropped. Their only benefit was that the handler returned before a model that ignored cancellation did, and the platform already ends the caller's request at its route deadline.
- #481's runtime limits in settings and the runtime owner rows in the ledger are dropped. The limits are not operator settings, and the ledger is operator-facing.
- #481's refusal of the reserved `grafana` and `grafana-ai-sdk` namespaces is dropped.

## Capabilities

### New Capabilities

- `gateway-execution-runtime`: protocol-independent selection, invocation, stream consumption, failure classification, attempt capture and host request policy shared by every Gateway API adapter.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: the handler is constructed from a runtime. Invocation is synchronous, without bounded latency for work that ignores cancellation. Host namespaces other than call-level `gateway` are no longer reserved.
- `providerwire-v4-streaming-runtime`: the limits are the runtime's. Stream setup is synchronous, with one owner for each returned stream, and late streams are drained once.
- `gateway-request-byok`: BYOK attempts are captured, not published.
- `gateway-file-inputs`: file and message scopes no longer refuse host namespaces.
- `gateway-custom-content`: a custom part's `gateway` namespace is ordinary.
- `gateway-provider-tools`: tool-part options no longer refuse host namespaces.
- `gateway-provider-configuration`: the reserved `providerName` is `gateway` instead of `grafana`.

## Impact

- **Code:**
  - `ai-gateway/internal/execution`: new runtime, limits, selection, classification, capture and policy;
  - `ai-gateway/providerwire/v4`: handler, stream, selection, errors, capture and request mapping;
  - `cmd/grafana-ai-gateway/internal/{config,gateway,service}`;
  - `test/providerwire-v4/testserver`.
- **Go API:** `v4.New`, `v4.Config`, `v4.Limits`, `v4.Selection`, `v4.RequestSelector`, `v4.CatalogSelector`, `v4.Err*` and `v4.ReservedProviderOptionNamespace` change or move to an internal package. No consumer exists outside the `ai-gateway` module. The PR title carries `!`.
- **Wire:** no document changes. Requests that used the removed namespaces now succeed. The precedence between a protected-header refusal and a later mapping failure changes; both are fixed 400 documents.
- **Tests:** behavior is the gate. Tests may change imports, construction and the package that owns a moved internal. Tests for work that ignores cancellation are replaced by tests for discarded late results.
- **Parity:** Gateway execution and host policy are Go extensions with no `@ai-sdk/gateway@4.0.116` counterpart. The pinned client sends provider options and call headers in the body as opaque data, unchanged. `knownGaps`, recorded fixtures and conformance expectations are unchanged.
- **Delivery:** branch `nrbrd/gateway-execution-runtime`, based on `nrbrd/gateway-platform-limits` (#521). The issue names `nrbrd/gateway-resource-policy`, but #521 was inserted into the stack after it.
- **Out of scope:**
  - one selector and one handler, the route table, host errors, discovery and per-attempt translation (#482);
  - lifecycle validation (#483);
  - a public runtime package;
  - publishing BYOK overviews (#420).
