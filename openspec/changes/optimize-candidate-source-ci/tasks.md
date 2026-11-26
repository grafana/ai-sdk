# Tasks

## 1. Docs selection

- [x] 1.1 Extend the docs classifier and document the allowlist. Add cases for reviewed Gateway prose, README creation/removal/rename, unreviewed Markdown, mixed inputs, modes and classification errors. Keep the focused Gateway link validation in the docs lane and prove missing link targets or headings fail. Verify with the Python classifier tests and `mise run lint-docs`.

## 2. Cache ownership

- [x] 2.1 Save ordinary Go module downloads on a miss after successful `build-test` checks on canonical `main`, without a completeness requirement or separate fill step. Retain rolling compiled Go and lint caches.
- [x] 2.2 Restore the existing pnpm store before the frozen workspace install in race, module-resolution and parity-baseline without adding writers.
- [x] 2.3 Use workflow tests to verify producer ownership, cache key reuse, frozen installs and the docs and tag guards.

## 3. Command-contract ownership

- [x] 3.1 Build one ordinary race-enabled Gateway/host pair per combined contract invocation and a separate test-only Gateway for the Go stack-catalog cases. Preserve local entry points and per-test isolation. Verify both catalog topologies and the other Go and TypeScript scenarios with `mise run test-openai-chat-completions-adapter`.
- [x] 3.2 Route only the moved contract package away from the redundant CI invocations. Keep one explicit owner under the existing required race check; document coverage ownership. Test routing and fail-closed merge/publication evidence with `mise run test-ci-workflow` and the locked release tooling tests.
- [x] 3.3 Verify the new harness rejects known bad test outcomes and missing binaries rather than skipping execution. Run the retained ProviderWire semantic request mutants via `mise run test-providerwire-v4`; confirm the differential, not build/setup failure, rejects the copied mutant code.

## 4. Deferred: browser worker-count and shard benchmarking

No browser performance comparison is required for this change, and no controlled baseline was obtained. Keep the default Playwright configuration, normal browser E2E, plugin validation and publication gates unchanged.

## 5. Integrated validation

- [ ] 5.1 Run focused classifier and workflow tests and strict OpenSpec validation; verify ordinary hosted source and plugin checks at the pushed head. Record revision-tagged results and limitations.

## Workflow follow-up

- Create the draft pull request with focused commits and measured/unmeasured distinctions; verify effective required checks and plugin validation on the current pushed head. No merge, publication or protection changes.
