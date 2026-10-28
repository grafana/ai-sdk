# Tasks

## 1. Cache admission and configuration

- [x] 1.1 Add the internal cache API and validated resource/time limits, fixed outcomes, and authoritative-denial sentinel; test invalid options, zero attribution, canceled requests, and no resolver calls during construction.
- [x] 1.2 Add independent per-stack destination/policy entries and result validation; test cold then warm access, two-stack isolation, foreign results, and snapshots that expire while lookup is pending.
- [x] 1.3 Document cache ownership and attribution in godoc and the maintained Gateway architecture/glossary; verify the documentation distinguishes cache capability from production route integration.

## 2. Shared lookup and request budgets

- [x] 2.1 Deduplicate concurrent component lookups while keeping stacks independent; use channel-controlled tests to verify resolver call counts, no lock held across resolver waits, and cancellation of one waiter without affecting the others.
- [x] 2.2 Bound cold waiting and honor request deadline/headroom, shared lookup timeout, and cache closure; use deterministic timer/context tests to verify prompt budget/cancellation outcomes and that no model invocation or replay exists in the API.
- [x] 2.3 Document waiter versus shared-work cancellation and the operational lookup budget; verify defaults are not presented as measured deployment recommendations.

## 3. Refresh and negative state

- [x] 3.1 Implement independently scheduled component refresh and expiry checks with minimum attempt spacing; test valid reuse during transient failure, hard expiry, unequal component lifetimes, and repeated nearly expired responses without a refresh loop.
- [x] 3.2 Replace state immediately on observed disablement, removal, denial, or invalid results; test disablement followed by failure, individual signal removal, and rejection of stale/late completions without restoring permissive defaults.
- [x] 3.3 Add bounded retry suppression and fixed failure/non-recording classifications; test burst failures, later enablement without restart, no raw diagnostics, and independent recovery of destination and policy components.
- [x] 3.4 Document refresh/revocation limits and failure behavior in the Gateway guide; verify it does not claim revocation of already-issued recorder leases or knowledge of unspecified Cloud error formats.

## 4. Resource lifecycle

- [x] 4.1 Bound entry and resolver-call ownership and reclaim idle entries; test admission at capacity, background refresh not extending idle retention, later reclamation, and a canceled non-cooperative resolver remaining charged to capacity.
- [x] 4.2 Implement idempotent bounded close and late-completion rejection; test concurrent lookup/close, waiters waking, timers stopping, no post-close publication, and no replacement calls while a canceled resolver remains outstanding. Release non-cooperative fakes during test cleanup.
- [x] 4.3 Document shutdown ownership and cancellation requirements for future real adapters; verify no production composition root or exporter lifecycle has been wired in this slice.

## 5. Integration acceptance

- [x] 5.1 Run cache tests under the race detector and `mise run test-ai-gateway-source`; record results and verify existing contracts, auth, configuration, and operator recording behavior remain covered.
- [x] 5.2 Run Gateway vet, pinned lint, formatting, documentation checks, baseline validation, and strict OpenSpec validation; inspect the final stacked diff for scope and public-content safety and record before/after review evidence outside the repository.

## Workflow follow-up

- Review against the contracts branch in #590 so this PR contains only the cache slice; retarget after the dependency merges.
- Sync and archive the completed change after implementation and review, before publishing a merge-ready revision.
- Keep deployment limits, live discovery mappings, actual write authority, and exporter integration as separate validation gates.
