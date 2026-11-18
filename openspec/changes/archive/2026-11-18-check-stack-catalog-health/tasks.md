# Tasks

## 1. Saved catalog health service

- [x] 1.1 Reuse stack selection's preflight and resolved provider inputs for the health service. Add white-box tests for saved keys, failed catalog policy or shape, and no fallback to operator/ambient credentials.
- [x] 1.2 Add bounded native listing, the typed result projection, with fake-provider tests for auth, fixed paths, pagination, every result class, exact primary/fallback matches, unused providers, and absence of upstream data or secrets in results and telemetry. If custom endpoints are available on the implementation base, prove the shared allowlist, prefix preservation, and compatible endpoint behavior with fake providers; otherwise assert the base's refusal without dialing.
- [x] 1.3 Enforce deadlines, unconditional decoded byte limits, redirect refusal, fan-out, and run admission limits. Verify partial results, no queued or retried run, concurrent callers, and cancellation with controlled request lifetimes and Gateway race tests.

## 2. Route and authorization

- [x] 2.1 Register the bodyless action with the existing catalog parent and access checks. Regenerate manifests with `mise run generate-ai-gateway-kinds`; assert method/path agreement, the declared `get` on `modelcatalogs/health`, and the writer-only default grant. Update the route walkthrough and glossary for the third catalog subresource.
- [x] 2.2 Use `test/platformhost` to verify response outcomes and fixed errors (`400`, `424`, `429`, caller or parent failure), no probe on refused or unmatched requests, and the correct decrypted key on the permitted path. Do not treat the harness as evidence of Grafana RBAC.

## 3. Catalog page

- [x] 3.1 Add the signed-in API request, explicit permission-gated check and fixed statuses on the saved page only. Verify permission, pending, error and save/navigation invalidation states in Jest (`mise run test-ai-gateway-frontend`). Document when to use the action in the catalog guide and what listing does and does not prove.
- [x] 3.2 Extend the Compose Anthropic fake for model listings. Add Playwright coverage against Grafana's saved catalog and real route: Admin gets fixed provider/model outcomes and no sensitive data; Editor and Viewer lack the action and their direct requests are denied.

## Workflow follow-up

- Sync and archive the completed change according to the repository workflow before merge. Do not push without approval. After an approved push, verify the candidate's `test-ai-gateway-e2e` run, including declared `get` authorization for POST.
