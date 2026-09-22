## 1. Reusable middleware controls

- [x] 1.1 Add a typed identity-source option to `middleware/logger` with response-preferred zero-value behavior and requested-mode suppression of response/transport identity attributes; unit-test unary, stream, incomplete metadata, and unchanged pass-through values.
- [x] 1.2 Add a typed identity-source option to `middleware/agentobservability` with response-preferred zero-value behavior and requested-mode suppression of response model, response ID, and transport metadata; unit-test unary and stream recording while proving source values pass through unchanged.
- [x] 1.3 Add optional bounded stream-drain durations to logger, Prometheus, and Agent Observability recording options, using an absolute deadline checked around receives; test closed, silent, continuously-ready, and cancellation-race channels plus zero-value compatibility.
- [x] 1.4 Run focused module format, test, vet, and dependency checks for all three middleware modules and confirm the root module graph remains unchanged.
- [x] 1.5 Add provided-only recording context, a final consumer-owned generation filter, fail-open recorder-error reporting, and exact-once post-End completion signaling required by the Gateway privacy/export/shutdown boundary, with unary, stream, nil-result, ambient-context, panic, queue/validation, abandoned-stream, and callback-backpressure tests while preserving nil/default behavior.

2026-09-14 long-block checkpoint: reusable controls are complete in local Apache source checkpoints `9ac4c4a` (logger), `ca4a128` (Prometheus), and `01aeee9` (Agent Observability). All three modules pass isolated vet, tidy-diff and build; logger/Prometheus pass race count=5 and Agent Observability passes race count=50. Root `GOWORK=off` test/build and registered upstream baseline validation pass; all root/module dependency files and Gateway source remain unchanged. Agent Observability zero retains the reconciled main behavior of not draining after cancellation. Metadata-only privacy characterization establishes remaining Gateway context/mapping work and does not complete section 2 or 5. These local commits are not published module pins. Full evidence and handoff: `.codex/implementation-reports/2026-09-14-wp8-long-block.md`.

Independent-review follow-up `458775c`: added deterministic logger/Prometheus full-output, active-producer, already-canceled, deadline and concurrent close/cancel matrix cases, plus explicit logger requested-identity privacy-limit documentation. Both expanded modules pass race count=50 and vet/tidy/build. Runtime code/dependencies remain unchanged; no additional Gateway task is complete.

2026-09-15 signed hardening follow-up: Prometheus now normalizes API error
status labels to `100`–`599`, `none`, or `other`, preventing an invalid provider
integer from creating an arbitrary label. The focused module passes race
tests, vet, tidy-diff, and build in signed commit `3293cbf`. Agent Observability's
provided-only context, final generation filter, fail-open recorder diagnostics,
and exact-once completion boundary are in signed commit `895aa5e`; its race
tests, vet, tidy-diff, and build pass.

## 2. Gateway observation context and privacy

- [x] 2.1 Add a private immutable Gateway request-observation value carrying one generated bounded correlation ID and fixed optional fields for verified caller service, verified namespace, configured region, and configured application.
- [x] 2.2 Allocate correlation once in the outer HTTP telemetry context, project verified auth values only after successful authentication, and expose narrow readers for HTTP/model logging and Agent Observability without storing tokens or full authlib state.
- [x] 2.3 Add table-driven trust-boundary tests proving arbitrary headers/context values, acting-user details, raw claims, secrets, empty values, and over-limit values cannot enter the observation view or provider call options.
- [x] 2.4 Add a Gateway logger attribute allowlist composed with the reusable default secret redactor; test that raw finish reasons, warning strings, response metadata, payloads, provider details, and future unknown attributes are dropped while approved lifecycle fields remain.

## 3. Exporter and service configuration

- [x] 3.1 Add strict Gateway settings for trusted static region/application and Agent Observability enablement, protocol/endpoint/TLS, auth secret reference, finite queue/batch/payload/retry/backoff, and independent flush/shutdown duration; bind flags/environment explicitly and reject invalid combinations before secret resolution or construction.
- [x] 3.2 Resolve the Agent Observability credential exactly once from its environment-variable reference, construct one process-wide exact-pinned client with metadata-only capture and hooks disabled, and adapt asynchronous failures to fixed diagnostics plus bounded counters without exposing raw exporter errors.
- [x] 3.3 Add configuration tests for production TLS/credential requirements, development disablement, empty/unset secret references, non-positive or overflow-prone bounds, invalid enums/endpoints, ambient environment isolation, and secret-free failures.
- [x] 3.4 Expose the service-owned Prometheus registry through a narrow startup registerer and register model collectors once; make duplicate registration fail before listener binding and readiness.

## 4. Canonical catalog composition

- [x] 4.1 Introduce a service-owned model-observability factory that builds the fixed context bridge, Agent Observability recording, logger, and Prometheus chain with requested identity, metadata-only privacy, disabled logger capture/per-part events, and bounded drains.
- [x] 4.2 Update catalog construction to wrap each canonical entry's logical model once with provider `grafana` and its canonical public ID while preserving the direct inner model internally, returning one shared composed instance for canonical/alias resolution, and leaving an unchanged inner-model seam for WP9 fallback.
- [x] 4.3 Add catalog/composition tests proving exact middleware order, one traversal per logical unary/stream call, canonical identity for aliases, no provider-bound enrichment, one collector registration, and unchanged call options/results/stream parts.
- [x] 4.4 Keep `middleware/enrichment` in provider-output-disabled mode or omit it from the build entirely, documenting that WP8's Gateway context bridge is telemetry-only and that provider header/options enrichment requires a later explicit host-policy approval.

## 5. Model lifecycle and privacy evidence

- [x] 5.1 Add deterministic unary tests for success and provider failure covering start/terminal counts, usage, finish, duration, approved correlation, fail-open export, canonical identity, and absence of credentials/payloads/backend IDs/provider errors across logs, metrics, and Agent Observability.
- [x] 5.2 Add deterministic streaming tests covering normal finish, split/strongest usage, first output timing, provider error followed by later content and finish, premature close, cancellation, timeout, silent upstream, and continuously-ready non-cooperative upstream.
- [x] 5.3 In every streaming test assert original part order and identity metadata reach ProviderWire unchanged, exactly one logical finalization occurs per observer, output channels close once, and all observer/drain goroutines finish within their configured deadlines.
- [x] 5.4 Add alias/canonical and hostile-data privacy matrix tests that search captured logical telemetry for access credentials, request/response text and bodies, headers, provider options/metadata, arbitrary error/warning text, response IDs, provider instance/backend model IDs, URLs, and topology.
- [x] 5.5 Add an explicit WP9 seam test fixture showing one logical chain can wrap a placeholder lower model without defining, emitting, or depending on physical-attempt hooks or records.

## 6. Process integration and dependency proof

- [x] 6.1 Wire observation components before listener binding/readiness and add bounded Agent Observability flush/shutdown after HTTP serving stops, preserving WP5 readiness-first/cancel-first/independent-deadline order.
- [x] 6.2 Extend the real-command fake-Anthropic integration with a fake/capturing Agent Observability destination and one shared `/metrics` scrape for authenticated unary, streaming, provider-error, abort, and process-shutdown scenarios.
- [x] 6.3 Prove real-command ProviderWire status, unary bytes, SSE events, clean EOF, cancellation, and provider invocation counts remain unchanged while each enabled logical surface records once and exporter outage does not fail traffic.
- [x] 6.4 Update Gateway operator/configuration docs with trusted metadata sources, metadata-only policy, metric labels, exporter bounds/failure behavior, and explicit WP6/WP7/WP9/WP10/WP27/later-capability boundaries.
- [x] 6.5 Publish or identify immutable Apache middleware refs before the dependent Gateway commit, pin `ai-gateway/go.mod` without committed `replace`, keep `ai-gateway` absent from root `go.work`, and verify the root module remains buildable/testable with the Gateway directory unavailable.

2026-09-14 private implementation checkpoint: sections 2–5 and 6.1–6.4 are implemented locally. The actual Gateway command passes a capturing fake-Anthropic/fake-Agent-Observability matrix for unary, stream, provider error, client abort, one shared metrics scrape, process shutdown, canonical identity, privacy, exact invocation count, and exporter-outage fail-open behavior. Root test/build also passes from a temporary checkout with `ai-gateway/` absent. Task 6.5 remains intentionally open: the Apache middleware commits are local only, so `ai-gateway/go.mod` has no fabricated versions or `replace` directives and the strict boundary check stops only at those missing immutable dependencies.

2026-09-14 shutdown-order hardening: repeated real-command execution exposed a race in which HTTP shutdown could finish while an abandoned stream's observer goroutine was still finalizing. Agent Observability now signals exact-once completion after recorder End without letting callbacks delay downstream EOF. The Gateway refuses new recorder acquisition after close begins and boundedly waits for active recorders before a fresh bounded flush/shutdown; `process_shutdown_completed` is emitted only after that finalizer. Diagnostic classification is also capped to the SDK-prefix window. Coordinated lifecycle and Gateway barrier regressions passed scheduler-diverse repetition, the composed abandoned-stream/process-close regression passed 100 race runs, the exact final Gateway tree passed a full 50-run shuffled race soak (including the spawned-command integration package) plus fresh 10-run integration gates, every reusable middleware suite passed 100, and the separately enumerated real-command matrix passed sixty post-fix executions (600 scenarios), including five runs each at `GOMAXPROCS=1` and `8`. This remains part of 6.1–6.3 and does not change the immutable-publication blocker in 6.5.

2026-09-15 immutable dependency and review closure: the rebased middleware
commit `0b83f45` publishes logger, Prometheus, and Agent Observability together
as `v0.0.0-20260915200037-0b83f45375ca`. `ai-gateway/go.mod` pins that exact
version for all three modules with no `replace`, and root `go.work` remains
unchanged. Unknown stream-part values use the single closed `other` metric
label, and cancellation/timeout wins consistently when context termination and
upstream closure are both observable at logical finalization. Focused
middleware race/vet/build, Gateway race/vet/build, real-command, integration,
root-isolation, boundary, full parity, docs, strict OpenSpec, and clean-cache
module-resolution gates pass against the corrected public version.

## 7. Verification

- [x] 7.1 Run `gofmt`, focused middleware and Gateway unit/race tests, module vet/lint/build checks, and dependency/license-boundary inspection.
- [x] 7.2 Run the ProviderWire V4 checks, real-command cross-language integration, `mise run validate-parity-baseline`, and root `GOWORK=off` build/test; record that no upstream baseline or conformance fixture provenance changed.
- [x] 7.3 Run `openspec validate add-gateway-text-observability --strict` and verify every issue #103 acceptance criterion is backed by a named automated scenario.
