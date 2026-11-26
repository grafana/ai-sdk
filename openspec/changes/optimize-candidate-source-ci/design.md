# Design

## Context

At the starting revision, three jobs run the same Go Chat Completions package, and each suite invokes two race-enabled builds per top-level test. The TypeScript counterpart is in the local combined task but is not explicitly scheduled in Build & Test. The push-only plugin host step runs the separate authenticated ProviderWire command task. The full Gateway job covers external-module compilation and plugin process tests that skip in short mode.

Go dependency caches are job-keyed; compiled Go and linter caches are also written per commit. The tag plugin build currently uses cache lookup-only, which prevents restore but does not disable the pinned cache action's post-job save. Existing release authorization checks the same-revision required job names and, for Gateway publication, also `Validate AI Gateway plugin`. The latter is not required by the live branch rules.

## Goals / Non-Goals

Preserve coverage, named gates and candidate module resolution with both empty and restored caches. Improve merge-required and whole-workflow latency where measured and report them separately. This is a harness change, not a new parity result, provider behavior change or release-policy change.

## Decisions

### Classify prose by reviewed path and file type

Keep the existing root/docs rules and fail-closed raw Git diff. Add the reviewed `ai-gateway/docs/` narrative files (`model-catalog.md`, `sources.md`, `text-observability.md`) by name. Allow only regular-file content modifications to `ai-gateway/README.md`: it is the fallback packaging input when `ai-gateway/src/README.md` is absent, so a creation, removal or rename out of it must exercise source and packaging. Do not classify all Gateway Markdown or future guides by extension alone. Keep docs lint unconditional. Run the focused Gateway overview/README link test on the fast path, including root docs changes; the general structural linter does not validate those GitHub blob URLs and anchors. Share the test logic without requiring the plugin build or full frontend install if practical.

### Assign command contracts to the existing required race job

Keep an explicit required command-contract invocation in the existing `race` job, retaining the existing checks by name and avoiding another runner/setup/archive path. Give only this package a narrow CI routing rule outside the contract task. The local broad test commands still exercise the Go suite. If the TS dependency setup and additional runtime materially regress the race critical path, measure and propose a separate parallel job gated by both existing aggregates rather than silently changing the required graph.

| Coverage | Source CI owner |
| --- | --- |
| Short candidate-source Go suites apart from the moved contract | `build-test`; `ci` continues to require it and lint |
| Full Gateway tests apart from the moved contract, including short-mode skips | `Test AI Gateway source`; plugin validation continues to require it |
| Race short tests for the whole existing stream-path module set apart from the moved contract | `race` |
| All five official Go top-level scenarios and all official TypeScript lanes (Responses, OpenAI-compatible, Anthropic), with both executables race-enabled | One explicit contract invocation under `race`, already checked by branch protection and release authorization |
| ProviderWire comparator, request mutants, root frontend integration and authenticated Gateway command | Existing parity-baseline and integration jobs; retain the canonical push-only plugin host suite |
| Generated manifest, frontend, both Linux architectures, artifact validator, browser E2E | Existing plugin build; retain fail-closed validation and same-revision artifact handoff |

Build each executable once in an invocation-owned temporary directory using `GOWORK=off`, local candidate replacements and `-mod=readonly`. Hand that pair explicitly to both client suites; direct local harness entry points should still build at most one pair per invocation when none was prepared. Keep build/setup errors fatal and require a complete pair. Run the dedicated Go suite with `-race -count=1` so results cannot mask skipped child execution. Reuse only files, not test hosts, configurations or synthetic server state. Do not share these executable files across jobs/revisions or consume arbitrary cached executables.

The pinned upstream context is the registered `ai@7.0.173` commit, including OpenAI Responses tool-argument SSE, compatible `/chat/completions` tool calls, and Anthropic `/messages` content deltas. These contextual checks do not replace the official client contract assertions; the adapter boundary and fixture bodies remain unchanged.

### Reuse dependency downloads from existing jobs

- On a cache miss, only the canonical `main` `build-test` job saves `GOMODCACHE`, after its ordinary checks succeed. The key includes runner and toolchain identity and the tracked Go manifest and sum inputs. A saved archive can be partial; ordinary checked module resolution supplies anything missing. Consumers do not wait for a producer, and there is no separate dependency fill step.
- Keep the existing canonical-main weekly compiled Go caches partitioned by native, race and plugin workloads, plus the separate lint analysis archive. Pull requests do not write these Go or lint caches. Skip cache exchange entirely on Gateway tags, including the combined caches and mise. Cache archives are not release artifacts.
- The race, module-resolution and parity-baseline jobs restore the existing lockfile-keyed pnpm download store before their first workspace install. Integration, conformance and plugin jobs remain the existing writers. Every job still runs its frozen install as needed; no `node_modules` or runtime artifacts are added to the cache.

### Defer browser parallelism benchmarks

Browser worker-count and shard comparisons are outside this change and are not acceptance criteria. The local suite did not complete authentication, so it yielded no controlled baseline. Keep the existing Playwright configuration, normal plugin E2E, and validation and publication gates unchanged. A separate performance change would need repeatable timings on the same suite and runner class with independent test resources.

## Risks / Trade-offs

- **Cache misses:** new keys or evicted archives can make jobs slower, and a partial module archive only saves downloads that overlap. A miss never waives a check or requires a producer job.
- **Silent drops:** structural tests must enforce one owner, fail-closed status/artifact gates and docs guards; local inventory and semantic negative controls must prove the intended Go/TS cases still execute and reject bad outcomes.
- **Benchmark confounds:** build cache eviction and hosted runner variation prevent attributing historical timing differences to this change.
- **Renovate:** leave scheduling and fixes separate. The repository rebases behind-base branches and uses weekly package rules; Renovate defaults allow some existing PR updates outside a schedule. Assess effective inherited rules and a deliberate update-before-merge path separately because current required statuses must be up to date.

## Migration Plan

Validate classifier, cache policy and contract routing in separable commits. New cache keys warm naturally on canonical main; no migration or deletion. Revert any component if it weakens coverage; never work around status gates or publish during testing.
