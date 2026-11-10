# Proposal

## Why

Gateway composition lives entirely under `internal/`, so no other Go module can build a Gateway distribution: `pkg/main.go` wires the plugin runtime, plugin ID, authenticator and lifecycle itself. Observers are also baked into every logical model by whoever builds it (catalog at startup, routed and BYOK per request), so there is no single place where later work (stack catalogs, extensions) can apply per-call behavior to every selection.

## What Changes

- A public, experimental (v0) Go package in the Gateway module exposes one entrypoint that runs the plugin backend: it owns the plugin runtime, the fixed plugin ID, the platform authenticator, the process logger, environment and metrics registerer, the app provider callback that receives the kubeconfig, the lifecycle events and the final flush.
- The shipped backend's `main` becomes a thin caller of that entrypoint and keeps no composition of its own.
- The entrypoint takes an options value that is empty in this change. There is no global or `init()` registration.
- Selectors return identity-wrapped, unobserved models. The configured catalog keeps its identity-wrapped models from startup.
- Logical observers are applied at one seam in the execution runtime, after the request selector returns and before the model is invoked, for operator-catalog, routed and BYOK selections alike.
- Each selection carries its source (`operator-catalog` or `request-byok`), which picks the observer configuration at the seam. A selection without a known source fails as internal before any provider call.
- No externally observable behavior changes: identities, logs, metrics, Agent Observability, overviews, documents and lifecycle events stay the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-host`: adds the public composition entrypoint and the thin shipped `main`.
- `gateway-request-routing`: configured and routed chains are observed at the selection seam instead of where they are built.
- `gateway-text-observability`: observer chains are constructed once at startup but applied once per call at the selection seam, chosen by selection source, instead of being composed into each catalog entry at startup.

## Impact

- **Code:** new public package under `ai-gateway/pkg/`; `ai-gateway/pkg/main.go`; `ai-gateway/pkg/internal/service` (observer chains and the source-based seam, identity-only factories); `ai-gateway/pkg/internal/gateway` (production wiring); `ai-gateway/internal/execution` (selection source and the runtime seam).
- **APIs:** a new public Go API, documented as v0 and experimental; it may break in minor releases.
- **Tests:** existing tests pass unchanged. New tests: compile from an external module, identical observed identities across operator-catalog, routed and BYOK calls, and a per-request selection double through the seam.
- **Docs:** the Gateway architecture walkthrough and glossary.
- **Not parity-sensitive:** no stream, chunk, provider or wire change.
