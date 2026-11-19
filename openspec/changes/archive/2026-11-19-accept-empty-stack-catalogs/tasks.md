# Tasks

## 1. Admission and runtime

- [x] 1.1 Relax the collection minimums only for stack validation. Add white-box Go tests with testify for both empty states on create and update, removing the last model, removing the last provider with its key, and refusal of a missing or orphan key or a missing primary or fallback provider. Keep the operator minimums, key checks and existing validation regression tests.
- [x] 1.2 Distinguish malformed stored parent specs from empty specs in shared runtime preflight. Test absent, null and ill-typed stored specs or maps, the fixed error on all four routes, and no provider traffic. Retain parent identity and permission checks.
- [x] 1.3 Test both valid no-model states through selection and discovery: existing unknown-model category and `404 model_not_found`, `200 {"models":[]}`, zero candidate constructions or provider requests, and no operator-catalog fallback.
- [x] 1.4 Test health and inventory through the existing service and route suites. Cover an empty result with no providers, unknown provider without egress, a single health listing page for an untargeted provider (success and key rejection), and inventory from a provider with no configured models. Retain bounds, fallback-target checks, input validation, and fixed secret-safe outcomes.

## 2. Catalog app

- [x] 2.1 Replace the draft form with a name-only create. Remove draft-only key and validation state. Jest: assert the POST has empty maps and no secure values, navigation follows success, and errors and permissions still behave as before.
- [x] 2.2 Use the saved-page editor for provider and model changes only. Keep provider controls visible when it has no models. After connecting a provider, support manual add and on-demand browse without relaunching creation. Hide health when there are no providers; retain existing permission and stale-result behavior. Cover these states in Jest.
- [x] 2.3 Remove last-model and last-model-owning-provider guards. Keep the inbound fallback block; confirm which models and key are removed. Jest: assert the resource version and exact secure changes when deleting the last model, deleting the last provider, or deleting a provider whose primary models fall back elsewhere. Ensure surviving models and keys stay untouched.

## 3. Docs and evidence

- [x] 3.1 Update **Store model catalogs in Grafana** in `ai-gateway/README.md` with name-only creation, empty discovery, and unknown-model behavior. Check the architecture and glossary for drift, and run docs lint.
- [x] 3.2 Add Playwright coverage for pinned Grafana CI to create and reopen with empty maps, connect with a key and no models, explicit health and browsing before the first model, add and remove the last model, and remove a provider and its key. Check empty discovery and the invocation error through the real catalog routes. Keep secret-safe reads, permission and conflict coverage.
- [x] 3.3 Run Gateway Go tests (including race checks for touched packages), Gateway frontend Jest, docs lint, and strict OpenSpec validation. Record local results separately from the Grafana end-to-end evidence. Sync the approved deltas and archive after implementation.


Local validation passed: `mise run check` (including docs lint), race tests for the four touched Gateway packages, frontend typecheck/lint/Jest (150 tests), and `mise run test-integration` (109 cross-language cases). Playwright discovered 26 tests including setup across the three catalog suites; execution against the pinned Grafana image is pending CI.
