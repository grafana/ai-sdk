# Proposal

## Why

Teams that build Gateway distributions need to add their own per-call behavior, such as eligibility, quota, default settings, input and output security checks and audit. Today the only per-call layers are the fixed logical observers, and `middleware.Middleware` lets a wrapper skip the model, rewrite output or override identity, so it is not a safe contract for code from other teams. #761 added one composition seam after selection and a public entrypoint with an empty `Options`; this change adds the extension contract that uses them.

## What Changes

- A new public, experimental (v0) package `ai-gateway/pkg/extension` with typed phase interfaces: `Admit`, `TransformRequest`, `InspectRequest`, `InspectResponse` and `Observe`. The host owns control flow: extensions never receive `next`, never wrap the model and cannot change identity.
- An extension `Definition`: name, scope (`operator` or `tenant`), key ownership, transform priority, failure policy (`fail-closed` or `skip`), config schema, declared secrets, default state and a factory with dependencies (logger, Prometheus registerer, clock, resolved secrets). Instances may implement `Close(ctx)`.
- The selection source vocabulary (`operator-catalog`, `request-byok`, `stack-catalog`) moves to the public package as an informational label that may grow. Key ownership (`operator` or `customer`) derives from it and is the only applicability filter.
- A host pipeline runs the phases around every model call, for every selection source, between the logical observers and the selected model, above fallback. It owns ordering, per-call state, the host policy re-check after each transform, normalized response parts for unary and stream calls, response aborts, one `Observe` record per call, failure policies and panic containment.
- `gateway.Options` gains `Extensions` and `Mandatory`, and `gateway.DefaultExtensions()` returns the shipped extensions, which are none. There is no global or `init()` registry; invalid definitions and duplicate names fail startup.
- A new `extensions` plugin setting (`GF_PLUGIN_EXTENSIONS`) configures operator extensions: strict YAML, at most 64 KiB, with `*Env` secret indirection whose values join the summary redactor. Mandatory extensions that are disabled, unconfigured or invalid fail startup with secret-safe reasons.
- Two new runtime outcomes: `ErrDenied` maps to the permission category and document, and `ErrUnavailable` to a new unavailable category and document. Neither is retried by fallback, and a refusal before the model is entered records no attempt.
- The logical observers record extension denials as a distinct `denied` outcome, excluded from error rates, through the root logger and Prometheus middleware denial option added by the `record-middleware-denials` change, which this change builds on. The Gateway counts extension denials and failures by extension and phase.
- `Observe` runs off the response's critical path: dispatched after the unary result is returned or the stream is closed, bounded per call, and joined by the final flush.
- In-flight observations are awaited and extension instances closed within the existing shared 1 second final flush.
- The shipped `main` passes `DefaultExtensions()`; the public build registers no mandatory extension. Behavior without extensions is unchanged.

## Capabilities

### New Capabilities

- `gateway-extensions`: the extension contract (definitions, phases, ordering, per-call state, normalized parts, ownership filtering, failure policies) and the host pipeline that runs it around every model call.

### Modified Capabilities

- `grafana-gateway-client`: the Go client accepts the extension unavailable document and in-band extension events.

- `gateway-plugin-host`: distribution extension options on the public entrypoint, the `extensions` operator setting, mandatory extensions, and extension shutdown in the final flush.
- `gateway-execution-runtime`: extension denial and unavailable categories, and model entry recorded only when the selected model is invoked.
- `providerwire-v4-unary-runtime`: fixed ProviderWire documents for extension denial and unavailability, unary and in-band, with client classification evidence. Documents live here rather than in `providerwire-v4-http-contract`, which only owns client-contract evidence.
- `gateway-plugin-telemetry`: the `denied` model-call outcome and bounded extension event metrics and logs.
- `gateway-execution-overview`: no overview or synthesized attempt for a call refused before the model is entered.

## Impact

- **Code:** new `ai-gateway/pkg/extension`; new `ai-gateway/pkg/internal/extensionhost` (registry, operator config, pipeline); `ai-gateway/pkg/gateway` (`Options`, `DefaultExtensions`); `ai-gateway/pkg/main.go`; `ai-gateway/pkg/internal/gateway` (wiring, flush); `ai-gateway/pkg/internal/config` (setting); `ai-gateway/pkg/internal/auth` (caller type); `ai-gateway/pkg/internal/service` (observer denial options, request info, log allowlist); `ai-gateway/internal/execution` (source alias, entry, classification, policy check); `ai-gateway/providerwire/v4` (documents).
- **APIs:** new public Go API, v0 and experimental. `providers/grafana`, a separately released module, accepts the new unavailable document (minor release).
- **Docs:** new `docs/guides/gateway-extensions.md`; glossary terms (extension, phase, scope, key ownership, mandatory extension); architecture walkthrough; README settings table.
- **Parity:** not parity-sensitive. The extension contract and the host pipeline are Grafana-owned; `test/conformance/PARITY.md` lists the Gateway as a Go extension surface, and this change adds no upstream surface. No stream part, UI chunk, provider request or provider-option change; ProviderWire bytes change only for the new extension documents.
