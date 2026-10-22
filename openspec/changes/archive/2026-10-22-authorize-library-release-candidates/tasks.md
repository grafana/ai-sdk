# Tasks

## 1. Retained readiness evidence

- [x] 1.1 Define the versioned readiness record and emit it only after successful final candidate validation; test head/base/tree and metadata binding, no record on failure, and unchanged ordinary-PR behavior.
- [x] 1.2 Upload the record from the successful readiness workflow using a pinned action and explicit retention; add workflow regression checks for upload gating, no credentials in evidence and unchanged publication guards, and document missing/expired artifact behavior.

## 2. Frozen read-only inventory

- [x] 2.1 Add merged-candidate discovery around pinned `Manifest.buildReleases()` with a complete paginated SCM iterator and stable-main/config snapshot; use the actual pinned API with mocked reads to test a candidate beyond the upstream 200-PR cap, discovery failures, empty inventory and movement during discovery.
- [x] 2.2 Freeze canonical candidate identity/config/metadata and add explicit library selection, duplicate component/tag rejection and Gateway disposition; test injected late candidates cannot enter the frozen set and document discovery versus authorization output.

## 3. Exact squash and check provenance

- [x] 3.1 Factor state-independent release identity/generated-file checks and add a separate merged-candidate path without weakening open-PR checks; local Git fixtures must reject spoofed App identity, unsigned heads, edited metadata, extra files, unsupported topology and unreachable merges.
- [x] 3.2 Retrieve readiness artifacts only from the expected canonical workflow/run/attempt, parse bounded records safely and bind head/base/tree/metadata to the reconstructed squash candidate; test wrong workflow/PR/repository, artifact path injection, unsafe redirects/credential forwarding, stale base, tree mismatch, absent/expired evidence and a valid equivalent-tree squash. Document the supported squash-only contract.
- [x] 3.3 Verify canonical push-main CI at the exact squash revision and all seven source jobs; test pagination, run attempts, wrong SHA/event/workflow, name spoofing, skipped/cancelled/failed/missing jobs and a maintainer-triggered successful run. Validate metadata field assumptions against read-only GitHub PR/run responses and document their provenance.

## 4. Selected merged-library validation

- [x] 4.1 Run shared selected-library checks in a clean detached candidate checkout using trusted tool/harness paths and a restricted child environment; test candidate-side tool substitution and dirty checkout rejection while existing 24 readiness regressions remain green.
- [x] 4.2 Bind prerequisite freshness and ancestry to the immutable squash parent and rerun standalone/consumer validation; use local Git/proxy fixtures to reject stale tags, replacements, pseudo-versions, wrong origins and proxy failures, while later unrelated main commits do not invalidate the old candidate. Document delayed-run semantics.
- [x] 4.3 Prove independent outcomes with ready root and blocked downstream candidates: run the real selected-root validation path against deliberately unready downstream source, assert downstream standalone commands are never invoked for root, and distinguish actual library validation from mocked GitHub provenance evidence.

## 5. Final decisions and integration

- [x] 5.1 Emit per-candidate approved/blocked reports with frozen identity, tooling revision and exact evidence IDs; test independent outcomes, nonzero status for blocked selections, no approved subset on inventory failure, and empty-inventory behavior. Document that the report is not persistent publication permission.
- [x] 5.2 Add final metadata/run-attempt/tag/ancestry rechecks; test mid-run edits, reruns and tag appearance, block both same-target and conflicting existing tags, and assert every attempted GitHub mutation is rejected by the read-only transport. Document retry versus deferred recovery.
- [x] 5.3 Run integrated release-tooling, Python readiness, Go release-configuration, workflow and docs checks plus strict OpenSpec validation; verify source CI tests the new tooling, normal release guards remain intact, no publication path is invoked, and every delta scenario maps to executable evidence.

## Workflow follow-up

- Review and implement this change in its isolated worktree, separately from #487 and the live App exercise. Keep #245 and #468 open and inspect future PR closing-issue references.
- Sync only implemented deltas and archive this change after verification, preserving the historical partial archive and deferred tasks.
- Publication API integration, immutable recovery, bootstrap policy, protection migration, Renovate, Gateway and activation require later work; this change does not authorize them.
