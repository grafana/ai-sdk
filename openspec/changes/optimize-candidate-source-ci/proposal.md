# Proposal

## Why

Candidate CI repeats the same Gateway contract builds and writes overlapping Go caches that compete for retention. Reviewed Gateway prose edits also miss the documentation fast path, while browser testing is a substantial part of plugin validation time.

## What Changes

- Classify only reviewed narrative Gateway Markdown as docs-only, without weakening fail-closed path and mode checks or skipping docs lint and Gateway documentation-link validation.
- Make the official OpenAI TypeScript command contract explicitly required in CI and give the Go and TypeScript contracts one execution owner and one race-enabled build of the Gateway and test host per invocation. Retain separate short, race and full Gateway coverage outside that contract package.
- Seed reusable Go caches with explicit producers and bounded build-cache writes. Eligible tag validation must not restore or publish CI cache entries.
- Measure browser parallelism against the current suite with independent resources. Retain a worker or shard change only if it passes isolation, coverage and timing checks.
- Keep check identities, candidate-source inputs, assertions and release/publication gates. Do not change provider behavior, dependency pins, Renovate schedules, runner capacity or branch protection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `candidate-source-ci`: Define the documentation fast-path boundary, required command-contract ownership and cache trust boundary for tag artifact validation.

## Impact

Build & Test workflow, Go cache action, docs classifier, Gateway command-test harness, contributor documentation and structural CI tests. Playwright configuration may change if benchmark results warrant it. No SDK API, protocol, deployment or release-authorization change.
