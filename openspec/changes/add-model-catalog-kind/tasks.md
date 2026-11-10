# Tasks

## 1. Route baseline on the routes-only build

- [ ] 1.1 Add the fake Anthropic SSE server under `ai-gateway/tests/`, its `e2e`-profile compose service, the extra provisioned provider and model, and make `scripts/e2e.sh` start the profile; verify `mise run test-ai-gateway-e2e` still passes.
- [ ] 1.2 Add the namespace route spec (config, unary and streaming language-model, unknown-model error document, undeclared method) and run it against the current routes-only build; verify it passes and record the run.

## 2. Validation split

- [ ] 2.1 Split `config.File.Validate` into a shape walk with a per-provider key hook, keep `Validate` as shape plus `apiKeyEnv`, and export `ValidateShape`; verify every existing `config` test passes unchanged and new tests show `ValidateShape` accepts a file without `apiKeyEnv` and refuses each shape error.

## 3. Kind, manifest and roles

- [ ] 3.1 Add `kinds/modelcatalog.cue`, reference it from `v1alpha1`, add the `roles` and `roleBindings` blocks, and run `pnpm --filter ./ai-gateway generate:kinds`; verify the generated Go and TypeScript types and manifest are committed and regeneration leaves no diff.
- [ ] 3.2 Update the manifest tests: one kind, not folder-scoped, `secure` `*`, validation on `CREATE` and `UPDATE` only, unchanged route paths, and the two roles with their permission sets and bindings; verify they pass.
- [ ] 3.3 Verify the frontend typecheck, lint and tests pass with the generated TypeScript types (`mise run test-ai-gateway-frontend`).

## 4. Stack catalog checks and validator

- [ ] 4.1 Add `pkg/internal/stackcatalog` with `Parse` (policy, conversion, shape, key check); verify table tests cover each policy refusal, conversion of every field, a shape error from the spec, and missing and extra keys.
- [ ] 4.2 Add the secure key merge and `Validator` returning `400` errors; verify table tests for create, update without `secure`, `create` replacement, `remove` of a used and an unused key, stored-name echo, a new name on create and update, the stored name of another key (swap), conflicting and empty entries, and a wrong object type.
- [ ] 4.3 Add the sentinel test: refused and accepted reviews with plaintext `create` values leave no sentinel in errors or in the plugin log handler output; verify it passes.

## 5. App wiring and test host

- [ ] 5.1 Pass `ManagedKinds` with the catalog validator in `Gateway.Provider()`; verify a test that `simple.App.ValidateManifest` accepts the committed manifest for running and idle Gateways, and that existing gateway tests pass unchanged.
- [ ] 5.2 Add manifest kinds to the test host (config manifest, in-memory store, admission reviews, inline secure values) and the manifest path to the `plugin-host` command; verify host tests for create, get, update and delete, a denial, and names-only reads.
- [ ] 5.3 Add a gateway test that writes catalogs through the test host and the real admission adapter, valid and invalid, and an idle Gateway that refuses admission; verify it passes.

## 6. End-to-end tests against Grafana

- [ ] 6.1 Add the service account fixture (Viewer, Editor, Admin tokens) and the catalog specs for roles, folderless saves, secure storage and names-only reads, updates, refusals and secret-safe responses; add the container-log sentinel check to `scripts/e2e.sh`; verify the suite passes against the CI-pinned Grafana image.
- [ ] 6.2 Run the namespace route spec from 1.2 on this build; verify it passes unchanged and record both runs.
- [ ] 6.3 Run the whole suite with `GRAFANA_VERSION=13.3.0`; verify it passes, or report the version that does.

## 7. Documentation

- [ ] 7.1 Add the glossary entry and the README requirements (minimum version, toggles, Cloud limitation, writes fail closed while the plugin is unavailable), and update the architecture walkthrough if its manifest description drifted; verify `mise run lint-docs` passes.

## 8. Integration checks

- [ ] 8.1 Run `mise run fmt-check`, `vet`, `lint`, `build`, `test-ai-gateway`, `test-integration`, `verify-ai-gateway-boundary`, `build-ai-gateway-plugin` and `validate-ai-gateway-plugin`; verify all pass.
- [ ] 8.2 Run `openspec validate add-model-catalog-kind --strict`; verify it passes.
