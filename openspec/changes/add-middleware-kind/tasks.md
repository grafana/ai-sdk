# Tasks

## 1. Kind, manifest and roles

- [x] 1.1 Add `kinds/middleware.cue`, reference it from `v1alpha1.kinds`, add the two roles and their bindings, and run `pnpm --filter ./ai-gateway generate:kinds`; verify the generated Go and TypeScript types and manifest are committed, `spec.config` is `{type: object, additionalProperties: true}`, and regeneration leaves no diff.
- [x] 1.2 Update the manifest tests: two kinds; `Middleware` not folder-scoped, without secure values, validated on `CREATE` and `UPDATE` only, with `type` and `enabled` required and the open `config`; unchanged route paths; four roles with permission sets, bindings and `fixed:` IDs, the catalog roles pinned as literals; verify they pass.
- [x] 1.3 Verify the frontend typecheck, lint and tests pass with the generated TypeScript types (`mise run test-ai-gateway-frontend`).

## 2. Shared schema check

- [x] 2.1 Export `extensionhost.CheckConfig` from the operator plan's private check and make `NewPlan` wrap it; verify every existing `extensionhost` and `gateway` startup message and test is unchanged, and new tests cover a schema-less extension, a value mismatch, a nested location and an undeclared key without values.

## 3. Middleware validator

- [x] 3.1 Add `pkg/internal/tenantconfig` with `NewValidator`, `MaxConfigBytes` and the admission algorithm; verify table tests for valid creates (enabled and disabled, absent config), name mismatch, unregistered and operator types with the same reason, schema mismatch, non-empty config without schema, oversized config, update of a known type, disable-only updates for an unregistered type and a tightened schema, refused enable and config changes of an unregistered type, wrong object types, and allowed other actions.
- [x] 3.2 Add the sentinel test: refused creates and updates with sentinel values and undeclared sentinel keys leave no sentinel in the errors or in the plugin log handler output; verify it passes.

## 4. App wiring

- [x] 4.1 Build the managed kinds from `Options.Extensions` in `gateway.New` for idle and running Gateways and pass them in `Provider()`; verify `TestProvider_ManagesTheManifestKinds` expects both kinds with validators, running and idle, and existing gateway tests pass unchanged.
- [x] 4.2 Add the harness option to serve a Gateway against an existing test host, and gateway tests through the test host and the real admission adapter with injected tenant, schema-less tenant and operator definitions: create, get with unchanged `config`, update, refusals with reasons and no sentinel in bodies or logs, the shipped build refusing every create, an idle Gateway refusing `Middleware` reviews, and a delete without review while the plugin no longer answers reviews; verify they pass.
- [x] 4.3 Add the rollback test (two Gateways, one store: disable-only accepted, enable and config changes refused) and the no-runtime-effect test (stored `enabled: true` for a default-off counting extension and `enabled: false` for a default-on one leave their phases unchanged on a language-model call); verify they pass.

## 5. End-to-end tests against Grafana

- [x] 5.1 Add `ai-gateway/tests/middleware.spec.ts` with the service account fixture: permissions per role, Viewer and Editor `200` reads and `403` writes, Admin creates refused with `400` for an unregistered type and a name mismatch, Admin delete of a missing resource `404`, and sentinel values and keys absent from every response; add the middleware sentinel to the plugin-log check in `scripts/e2e.sh`; verify the suite lists, type-checks and lints (it runs in CI).

## 6. Documentation

- [x] 6.1 Add the tenant configuration section to `docs/guides/gateway-extensions.md`, update the `ScopeTenant` godoc, add the glossary entry and update the architecture walkthrough's kind sentence; verify `mise run lint-docs` passes.

## 7. Integration checks

- [ ] 7.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-integration`, `verify-ai-gateway-boundary`, `build-ai-gateway-plugin` and `validate-ai-gateway-plugin`; verify all pass, and record the checks that only CI can run.
- [ ] 7.2 Run `openspec validate add-middleware-kind --strict`; verify it passes.

## Workflow follow-up

- The end-to-end suite runs only in CI, at the pinned nightly image; the pull request records the result.
- A maintainer opens the upstream app-sdk issue from the draft in design.md; link it from the guide once it exists.
- #766 picks up the end-to-end checks listed under design.md Follow-ups.
