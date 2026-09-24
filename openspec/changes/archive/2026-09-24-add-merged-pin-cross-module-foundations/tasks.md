## 1. Published module inventory and merged-pin checker

- [x] 1.1 Share tracked nested `go.mod` discovery and published/local-only module classification in the Bash policy entry point; test root, Gateway, providers, middleware, examples, tests, and unknown selections.
- [x] 1.2 Implement independently callable merged-pin checking for direct requirements and selected internal module graphs with `GOWORK=off`, readonly manifests, immutable version-to-commit resolution, and an explicitly fetched canonical `main` anchor.
- [x] 1.3 Add deterministic Bash Git/module-fixture tests for merged tags and pseudo-versions, branch-only commits/tags, missing revisions, shallow/fork checkout, and synthetic merge-ref false positives; verify failure is closed and diagnostic.

## 2. Standalone and structural validation

- [x] 2.1 Move standalone public-proxy, clean-cache, readonly download/verify/build/test and replacement checks into a shared all-or-one-published-module entry point; retain `mise run verify-module-resolution` and test selector/error behavior.
- [x] 2.2 Separate license/module/import/root-graph policy checks from standalone execution in `scripts/module-policy.sh`; use tracked module inventory and check reverse references in source and nested module manifests.
- [x] 2.3 Keep the existing all-module `GOWORK=off` SDK and Grafana standalone build/test proof, check their module graphs for Gateway, and independently build/test both in a temporary copy without Gateway source through the same Bash entry point.

## 3. Gateway source integration

- [x] 3.1 Add the explicit Gateway-inclusive `.work` file and dedicated `mise` source-integration commands; assert local module resolution and preserve Gateway-free root `go.work` and production `GOWORK=off` behavior.
- [x] 3.2 Audit `gateway-command.test.ts`, `runtime-integration.test.ts`, `go-client-capture.ts`, and `test/integration/global-setup.ts`; wire Gateway-owned builds to explicit integration-vs-isolated selection without altering standalone Go client or SDK-only testserver evidence. Add a controlled source-change test that differs across modes.

## 4. Migration and activation

- [x] 4.1 Inventory actual declared and selected published internal pins; for each unmerged pin, record the module path, old/new versions and full revisions, behavior justification, and standalone test evidence in the implementation PR, or stop and escalate if equivalent merged behavior is unavailable.
- [x] 4.2 Update approved pins and sums; run scoped and all-module public-proxy standalone validation with readonly manifests and `GOWORK=off` before activating the guard.
- [x] 4.3 Add the merged-pin guard to the existing required `module-resolution` CI job without removing standalone, boundary, parity, integration, image, publication, or deployment checks.
- [x] 4.4 Update `AGENTS.md` and `CONTRIBUTING.md` to distinguish source integration from publication readiness and explain explicit workspace, merged pins, and selectable validation; keep #245's gate transition out of scope.
- [x] 4.5 Run deterministic tooling tests, `openspec validate add-merged-pin-cross-module-foundations --strict`, boundary checks, `mise run verify-module-resolution`, relevant Gateway source/isolated integration tests, and existing required validation; inspect resulting CI diff for unchanged gates.
