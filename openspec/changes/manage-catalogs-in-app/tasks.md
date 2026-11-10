# Tasks

## 1. API and permissions

- [ ] 1.1 Add `src/api/modelCatalogs.ts` (list, get, create, update, delete, `errorMessage`) on `getBackendSrv` with `showErrorAlert: false` and the generated kind types; verify with Jest tests for URLs (`appSubUrl`, namespace), methods, bodies and `Status` message extraction.
- [ ] 1.2 Add `src/permissions.ts` with the four `modelcatalogs` action names and `useCatalogPermissions()` on `hasPermission`; verify with a Jest test that results follow the permission map, not `orgRole`.
- [ ] 1.3 Add the form model: spec-to-rows and rows-to-spec conversion and `secureChanges(storedKeys, providers)`; verify with a Jest table test covering kept, replaced, new, missing, removed, renamed and re-added providers, and a full-spec round trip.

## 2. Catalog pages and routing

- [ ] 2.1 Add `ROUTES.Catalogs`, the `catalogs`, `catalogs/new` and `catalogs/edit/:name` routes in `App.tsx`, and the "Model catalogs" include in `plugin.json` with the `modelcatalogs:get` action; verify with the `App` Jest test rendering each path.
- [ ] 2.2 Build the catalog list with delete confirmation, empty state, "New catalog" and error alert; verify with Jest tests for rows, permission-gated controls, confirm-before-delete and a refused delete's message.
- [ ] 2.3 Build the catalog form: providers with `SecretInput` keys, models with aliases, primary and fallbacks, read-only mode, duplicate checks and returned-error display; verify with Jest tests for create and edit bodies, configured key state, no typed key in the DOM after save, read-only matrix, a verbatim admission message and a kept form after an error.

## 3. Overview

- [ ] 3.1 Add the "Model catalogs" block with each catalog's base URL to `Overview.tsx`, shown only with `get`; verify with Jest tests for the URLs, no request without `get`, the empty state and an inline load error with the endpoint still shown.

## 4. End-to-end tests

- [ ] 4.1 Add the `users` and `pageAs(role)` fixture to `tests/fixtures.ts` (Viewer and Editor login users, deleted after the worker); verify by the specs in 4.2 passing in CI.
- [ ] 4.2 Add `tests/catalog-page.spec.ts` for Admin create, edit (keys kept, key replaced), delete, admission error, conflict, and Viewer and Editor read-only pages, with the sentinel never on the page; add an overview base URL test to `tests/overview.spec.ts`; verify with the CI Playwright job (`mise run test-ai-gateway-e2e`).

## 5. Documentation

- [ ] 5.1 Update `ai-gateway/README.md` (catalog page, overview catalog URLs, when edits, removed roles and revoked tokens take effect) and `ai-gateway/src/README.md`; cite cache durations only if confirmed at the Grafana `v13.3.0` tag; verify with `mise run lint-docs`.
- [ ] 5.2 Add the "Stack catalogs" section to `ai-gateway/docs/model-catalog.md`; verify with `mise run lint-docs`.
- [ ] 5.3 Add "Offer Grafana models and customer catalogs" to `docs/providers/grafana-gateway.md` and update the overview sentence in `docs/guides/gateway-authentication.md`, after checking in Grafana source what an on-behalf-of catalog call requires of the user; verify with `mise run lint-docs`.
- [ ] 5.4 Check the architecture walkthrough and glossary against the result and update them only where they drift; verify with `mise run lint-docs`.

## 6. Integration checks

- [ ] 6.1 Run `mise run test-ai-gateway-frontend` (typecheck, lint, Jest) and `mise run build-ai-gateway-plugin` plus `mise run validate-ai-gateway-plugin`, and confirm the include action passes the validator.
- [ ] 6.2 Confirm no file outside `ai-gateway/src/`, `ai-gateway/tests/`, docs and this change is modified (`git diff --stat` against the base branch), and run `openspec validate manage-catalogs-in-app --strict`.

## Workflow follow-up

- Open the draft PR stacked on `nrbrd/758-stack-catalog-routes` after review.
- Archive the change after the PR review requirements are satisfied.
