# Tasks

## 1. Kind, manifest and roles

- [x] 1.1 Add `kinds/middleware.cue`, reference it from `v1alpha1.kinds`, add the two roles and their bindings, and run `pnpm --filter ./ai-gateway generate:kinds`; verify the generated Go and TypeScript types and manifest are committed and regeneration leaves no diff.
- [x] 1.2 Update the manifest tests: two kinds; `Middleware` not folder-scoped, without secure values, validated on `CREATE` and `UPDATE` only; unchanged route paths; four roles with permission sets, bindings and `fixed:` IDs, the catalog roles pinned as literals; verify they pass.
- [x] 1.3 Verify the frontend typecheck, lint and tests pass with the generated TypeScript types (`mise run test-ai-gateway-frontend`).

## 2. Shared schema check

- [x] 2.1 Export `extensionhost.CheckConfig` from the operator plan's private check and make `NewPlan` wrap it; verify every existing `extensionhost` and `gateway` startup message and test is unchanged, and new tests cover a schema-less extension, a value mismatch, a nested location and an undeclared key without values.

## 3. Middleware validator

- [x] 3.1 Add `pkg/internal/tenantconfig` with `NewValidator`, the 64 KiB bound and the admission algorithm; verify table tests.
- [x] 3.2 Add the sentinel test: refused creates and updates with sentinel values and undeclared sentinel keys leave no sentinel in the errors or in the plugin log handler output; verify it passes.

## 4. App wiring

- [x] 4.1 Build the managed kinds in `gateway.New` for idle and running Gateways and pass them in `Provider()`; verify `TestProvider_ManagesTheManifestKinds` expects both kinds with validators, running and idle.
- [x] 4.2 Add the harness option to serve a Gateway against an existing test host, and protocol tests through the test host and the real admission adapter; verify they pass.
- [x] 4.3 Add the rollback test (two Gateways, one store) and the no-runtime-effect test; verify they pass.

## 5. End-to-end tests against Grafana

- [x] 5.1 Add `ai-gateway/tests/middleware.spec.ts` with the service account fixture: permissions per role, Viewer and Editor reads and refused creates with nothing stored, Admin creates refused with `400` for an unregistered type and a name mismatch, Admin delete of a missing resource `404`, and sentinel values and keys absent from every response; add the middleware sentinel to the plugin-log check in `scripts/e2e.sh`; verify the suite lists, type-checks and lints.

## 6. Documentation

- [x] 6.1 Add the tenant configuration section to `docs/guides/gateway-extensions.md`, update the `ScopeTenant` godoc, add the glossary entry and update the architecture walkthrough's kind sentence; verify `mise run lint-docs` passes.

## 7. Public tenant type registry and binding

- [x] 7.1 Add `pkg/extension/tenanttype` (`Type`, `Member`, `All`, `Implementation`, `Bind`, package doc), with an empty registry; verify tests for `Member` (hyphens, digits), `Bind` (name, tenant scope, schema, default and implementation fields) and that `All` returns a copy.
- [x] 7.2 Add the types to the composition `Options` and the public `Run`, and extend `extensionhost.ValidateDefinitions` with the type checks and the binding check; verify table tests for invalid, repeated and reserved names and members, a schema without a valid example, an example without a schema, a tenant definition without a type or differing in schema or default, and an operator definition with a type's name, plus a `gateway.New` startup test on an idle host; update existing tests that register tenant definitions to pass matching types.

## 8. Projection and generator

- [x] 8.1 Add `pkg/internal/tenantschema` with the projection, the structural and prune round-trip check and the CUE rendering; verify table tests for each keep, drop and refuse rule, nested objects, arrays of objects, descriptions, and that a projected example survives pruning.
- [x] 8.2 Add the generator command, `kinds/middleware_members.cue`, the `middlewareMembers` import in `kinds/middleware.cue`, and the `generate-ai-gateway-kinds` mise task; regenerate; verify the drift test passes, the empty registry leaves the Middleware `spec` as `{type, enabled}`, and `generate:kinds` leaves no diff.
- [x] 8.3 In a scratch copy, generate members for the test types and the manifest with the pinned app-sdk, and verify the manifest test's Grafana-path conversion keeps each example; commit the generated schema as a test fixture and record the result in the pull request.

## 9. Typed union admission

- [x] 9.1 Add `tenantconfig.DecodeSpec`, the raw-spec `Middleware` object and codec, and `Kind()`; verify tests for metadata, type, enabled and canonical raw members, and fixed messages for undecodable specs.
- [x] 9.2 Rework `NewValidator` and the admission algorithm for members; verify table tests for one member only and only the type's, disable-only updates with the member equal, reformatted or absent, the member required when enabled, no member for a type without configuration, unimplemented, unregistered and operator types with one reason, the member size bound, schema failures with nested locations, and no sentinel in any reason.
- [x] 9.3 Update the protocol tests to test types and members: nested member round-trip, refusals, rollback with the member kept and removed, shipped types refusing every create, no runtime effect; verify they pass with `-race`.
- [x] 9.4 Update the manifest test for members (properties equal the registry's members, no enum or union, and the Grafana-path prune round-trip per type); verify it passes.

## 10. Documentation for typed members

- [x] 10.1 Rework the guide's "Configure tenant extensions", the `ScopeTenant` godoc, the glossary entry and the architecture sentence for typed members, public tenant types and the private-extension policy; verify `mise run lint-docs` passes.

## 11. Simplified configuration validation

- [x] 11.1 Refuse declared integer bounds outside ±(2^53−1) in the projection, validate with the standard schema check, and remove exact-number decoding; verify projection refusals for out-of-range bounds, accepted unbounded integers, and the validator and protocol tests.
- [x] 11.2 Refuse typed maps and `$ref` in the projection and remove location redaction; verify projection refusals and `CheckConfig` locations.
- [x] 11.3 Move the registry checks into a `tenanttype` test over `All()` and keep the binding check at startup; verify the registry and binding tests.

## 12. Integration checks

- [x] 12.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test`, `test-ai-gateway`, `test-integration`, `test-providerwire-v4`, `test-ai-gateway-frontend`, `verify-ai-gateway-boundary`, `verify-module-requirements`, `release-check`, `lint-docs`, `generate-ai-gateway-kinds` with a clean tree, and the end-to-end type checks; verify all pass, and record the checks only CI can run.
- [x] 12.2 Run `openspec validate --strict` on `gateway-extensions`, `gateway-plugin-host` and `gateway-caller-access`; verify they pass.

## Workflow follow-up

- The end-to-end suite runs only in CI, at the pinned nightly image; the pull request records the result.
- #766 picks up the end-to-end checks listed under design.md Follow-ups.
