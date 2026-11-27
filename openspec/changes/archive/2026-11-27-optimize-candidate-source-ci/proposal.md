# Proposal

## Why

Candidate CI repeats the same Gateway contract builds and writes overlapping Go caches that compete for retention. Reviewed Gateway prose edits also miss the documentation fast path.

## What Changes

- Classify only reviewed narrative Gateway Markdown as docs-only, without weakening fail-closed path and mode checks or skipping docs lint and Gateway documentation-link validation.
- Make the official OpenAI TypeScript command contract explicitly required in CI. The Go and TypeScript contracts share one owner and one ordinary race-enabled Gateway and host build per invocation; Go stack-catalog cases use a separate test-only Gateway build. Retain separate short, race and full Gateway coverage outside that contract package.
- Reuse the Go module downloads from a successful canonical-main build, retain rolling compiled Go and lint caches, and restore the existing pnpm store before more jobs run their frozen workspace install. Eligible tag validation must not restore or save CI caches.
- Defer browser worker-count and shard benchmarking outside this change. Keep the existing browser configuration, E2E coverage and plugin validation.
- Keep check identities, candidate-source inputs, assertions and release/publication gates. Do not change provider behavior, dependency pins, Renovate schedules, runner capacity or branch protection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `candidate-source-ci`: Define the documentation fast-path boundary, required command-contract ownership and cache trust boundary for tag artifact validation.

## Impact

Build & Test workflow, Go cache action, docs classifier, Gateway command-test harness, contributor documentation and structural CI tests. No changes to Playwright configuration, SDK APIs, protocol, deployment or release authorization.
