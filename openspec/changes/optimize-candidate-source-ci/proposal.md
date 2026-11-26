# Proposal

## Why

Candidate CI repeats the same Gateway contract builds and writes overlapping Go caches that compete for retention. Reviewed Gateway prose edits also miss the documentation fast path.

## What Changes

- Classify only reviewed narrative Gateway Markdown as docs-only, without weakening fail-closed path and mode checks or skipping docs lint and Gateway documentation-link validation.
- Make the official OpenAI TypeScript command contract explicitly required in CI and give the Go and TypeScript contracts one execution owner and one race-enabled build of the Gateway and test host per invocation. Retain separate short, race and full Gateway coverage outside that contract package.
- Seed reusable Go caches with explicit producers and bounded build-cache writes. Eligible tag validation must not restore or publish CI cache entries.
- Defer browser worker-count and shard benchmarking outside this change. Keep the existing browser configuration, E2E coverage and plugin validation.
- Keep check identities, candidate-source inputs, assertions and release/publication gates. Do not change provider behavior, dependency pins, Renovate schedules, runner capacity or branch protection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `candidate-source-ci`: Define the documentation fast-path boundary, required command-contract ownership and cache trust boundary for tag artifact validation.

## Impact

Build & Test workflow, Go cache action, docs classifier, Gateway command-test harness, contributor documentation and structural CI tests. No changes to Playwright configuration, SDK APIs, protocol, deployment or release authorization.
