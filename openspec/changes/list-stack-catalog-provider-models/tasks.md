# Tasks

## 1. Shared provider listing and inventory

- [ ] 1.1 Extract #836's traversal and single-use probe behind both projections and share the admission gate. Keep the health tests passing; add white-box Go tests with testify for health/picker contention, native request path and auth, pagination and cursor bounds, deadlines, cancellation, decoded byte bounds and redirect/replay refusal.
- [ ] 1.2 Implement the single-provider inventory projection with typed outcomes. Test exact IDs, deduplication, sorting, output limits, empty and partial lists, refusals, malformed pages, key echoes in IDs/cursors, and that no upstream text enters Gateway telemetry.
- [ ] 1.3 Add the short revision-scoped cache. With deterministic time and provider call counts, test hits, expiry, bounded eviction, isolation across namespace, catalog UID, revision and provider, missing metadata, uncached partial/failure results and no caching for health.

## 2. Catalog route and permissions

- [ ] 2.1 Declare and register the bodyless `available-models` POST, regenerate the manifests, and pass verified parent metadata to inventory. Verify the manifest's method, paths, declared permission and writer-only grant in manifest and route table tests. Update the architecture and glossary where they enumerate catalog routes; run docs lint.
- [ ] 2.2 Exercise the production route through the Gateway host harness. Test selector/body rejection, unknown provider, wrong method, parent or caller failure, missing key, custom-endpoint refusal and no outbound traffic on rejection. Verify correct stored account selection, validation before cache use, fixed route errors, `no-store`, and no credentials or upstream details in the response.

## 3. Provider card picker

- [ ] 3.1 Add the typed listing client and permission check, then wire **Browse models** into the saved provider card's existing quick-add form. Jest: assert a bodyless request for the selected provider, local search and exact selection, no write before confirmation, existing defaults and validation, and no listing in create flow or without permission.
- [ ] 3.2 Add progress, empty, no-match, partial, failure and retry states, keeping manual IDs usable. Jest: verify custom adds in every state, fixed messages, and that closed, navigated or updated views ignore late responses. Document saved-only browsing and manual IDs in the model catalog guide and run docs lint.

## 4. End-to-end integration

- [ ] 4.1 Extend Playwright against the pinned Grafana and #836's fake Anthropic `GET /v1/models`. As Admin, create with a manual ID (no listing request), browse using the stored key, search and add a listed ID, and add an unlisted ID. Reopen after replacing the key with the fake's rejected key to prove the old result is not reused. Verify manual add still works, and scan rendered content, console and responses for key and upstream-error sentinels.
- [ ] 4.2 On an existing catalog with a warm listing, prove Editor and Viewer cannot browse and receive `403` on a direct, otherwise-valid POST from their browser sessions. Use CI for the Playwright evidence; unit and host harness tests do not substitute for Grafana's permission check.
- [ ] 4.3 Run the Gateway Go checks (including race coverage of the touched packages), frontend Jest suite and strict OpenSpec validation. Record local results separately from CI.
