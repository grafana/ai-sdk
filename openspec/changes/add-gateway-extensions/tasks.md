# Tasks

## 1. Denial classification in the root middlewares

- [ ] 1.1 Add `IsDenied func(error) bool` to `middleware/prometheus.Options`, applied to call errors and stream error parts before other rules (`status="denied"`, `error_type="none"`); verify with table tests for generate, stream setup, error part and nil classifier in `middleware/prometheus`.
- [ ] 1.2 Add `IsDenied func(error) bool` to `middleware/logger.Options` (outcome and class `denied`, success level); verify with generate and stream tests in `middleware/logger`, including unchanged pass-through.
- [ ] 1.3 Update both packages' godoc and `docs/middleware` pages; verify `mise run lint-docs` and both modules' `go test ./...`.

## 2. Public contract and source vocabulary

- [ ] 2.1 Create `ai-gateway/pkg/extension` with the types, phase interfaces, sentinels and `Source.Keys()` from the design, and a v0 `doc.go`; verify with compile-time interface checks and a `Source.Keys` table test.
- [ ] 2.2 Make `execution.Source` an alias of `extension.Source`, with the two existing constants re-declared from it; verify existing `execution` and `service` suites pass unchanged.
- [ ] 2.3 Add `Options.Extensions`, `Options.Mandatory` and `DefaultExtensions()` to `pkg/gateway`; pass `DefaultExtensions()` from `pkg/main.go`; extend the external-module build test with a distribution that defines an extension; verify `go test ./pkg/gateway/...` without `-short`.

## 3. Runtime: entry, classification, policy check

- [ ] 3.1 Build the attempt capture before composition and mark entry in an `entered` model under the composed layers; verify runtime tests for a pre-model refusal (no attempt, no overview) and an unchanged single-candidate view without a composer.
- [ ] 3.2 Treat `ErrDenied` and `ErrUnavailable` after entry as a selected synthesized attempt; verify with a single-candidate output-denial test.
- [ ] 3.3 Add `CategoryUnavailable` at the end of the list, classify both sentinels first in `ClassifySelection`, `ClassifyProvider` and `ClassifyStreamPart` (including a status-less API error cause); verify `classify_test.go` tables.
- [ ] 3.4 Export the host policy check shared by translation and extensions; verify existing translation policy tests and new direct tests.
- [ ] 3.5 Add `safeUnavailable` and map both categories in ProviderWire, with no native summary on in-band extension errors; verify unary, setup and in-band document byte tests, the unchanged permission document, and the client-consumption tests in `test/providerwire-v4`.

## 4. Extension host: registration, ordering, lifecycle

- [ ] 4.1 Create `pkg/internal/extensionhost` registration checks (name rule, duplicates, enums, factory, schema compile, secret names, keys and sources consistency); verify table tests per invalid field with secret-safe messages.
- [ ] 4.2 Construct enabled instances with scoped dependencies, refuse instances with no phase, contain factory panics, and build the per-source ordered lists; verify ordering and filtering tests over mixed scopes, priorities, names, ownerships and sources, including `SourceStackCatalog`.
- [ ] 4.3 Close `Closer` instances concurrently in `sinks.close` under the shared budget; verify with a blocking, failing and panicking closer that `Close` returns within the budget and other sinks flush.
- [ ] 4.4 Add `Caller` type to `auth.Caller` and a service request-info accessor (namespace, correlation ID, caller type); verify `auth` and `service` table tests per access mode.

## 5. Extension host: the pipeline

- [ ] 5.1 Implement request phases: admit, transforms with container cloning and per-transform policy check, request inspection, failure policies, panic containment and per-call state; verify the phase-order, deny-position, policy-injection, per-call-state (`-race`) and failure-policy tests.
- [ ] 5.2 Implement unary response phases on normalized parts with lifecycle validation and abort; verify unary inspection, abort, invalid-output and skip tests.
- [ ] 5.3 Implement stream forwarding with normalized inspection, in-band abort, cancellation, bounded drain and validation; verify stream abort, consumer stop, cancellation and idle tests under `-race` with no leaked goroutine.
- [ ] 5.4 Implement Observe once per call with the detached bounded context, usage, timing and outcome precedence; verify one record per outcome (success, denied with extension and phase, error, cancelled, invalid output) and contained observer failures.
- [ ] 5.5 Add the extension event counter and failure log records; verify counter labels and log fields carry no error text or values.

## 6. Wiring and observation

- [ ] 6.1 Compose `observers(pipeline(...))` in `newRouteHandler`, and set `IsDenied` in `NewModelObservers` with `denied` added to the log allowlist; verify a production-composition test for configured, alias, routed and BYOK calls, unary and stream, with phases once per call, `status="denied"` metrics and `denied` logs.
- [ ] 6.2 Add Gateway handler tests for denial before the model on a two-candidate model (zero provider calls, no overview) and output denial after fallback (two calls, overview); verify `go test ./...` in `ai-gateway`.

## 7. Operator configuration and mandatory extensions

- [ ] 7.1 Parse the `extensions` setting in `config` (bound, strict shape, entries, `<secret>Env`), validate configs against schemas, and resolve secrets after other validation; verify table tests for every refusal with secret-safe messages.
- [ ] 7.2 Validate definitions and `Mandatory` before the idle check, and enforce mandatory entries; verify `gateway.New` tests for idle-with-invalid, disabled, unconfigured, invalid, missing secret, unknown and tenant mandatory.
- [ ] 7.3 Add extension secrets to `configuredSecrets`; verify a summary redaction test.

## 8. Documentation

- [ ] 8.1 Write `docs/guides/gateway-extensions.md` (contract, phases, ordering, ownership, failure policies, operator setting, mandatory, privacy obligations, v0) with index and footer links; add glossary terms extension, phase, scope, key ownership and mandatory extension; update the architecture walkthrough and the README settings table; verify `mise run lint-docs`.

## 9. Integration checks

- [ ] 9.1 Run `mise run fmt`, `vet`, `lint`, `test`, `build`, `test-ai-gateway` and `validate-parity-baseline`, and `openspec validate add-gateway-extensions --strict`; verify all pass, with CI providing e2e.

## Workflow follow-up

- Archive the change after review, and verify the archived specs.
