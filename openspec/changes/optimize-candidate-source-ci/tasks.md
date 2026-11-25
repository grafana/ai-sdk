# Tasks

## 1. Baseline and docs selection

- [ ] 1.1 Record pinned tool versions, checkout and current Playwright test census. Capture cold/warm command and plugin timings with isolated cache directories and the baseline running in this checkout before altering its harness.
- [x] 1.2 Extend the docs classifier and document the allowlist. Add cases for reviewed Gateway prose, README creation/removal/rename, unreviewed Markdown, mixed inputs, modes and classification errors. Keep the focused Gateway link validation in the docs lane and prove missing link targets or headings fail. Verify with the Python classifier tests and `mise run lint-docs`.

## 2. Go cache producers

- [ ] 2.1 Prove complete local module population on an empty module cache using the tracked module inventory. Verify every tracked module can resolve its graph from the seed, candidate local replacements remain selected and no manifests or sums change; run `mise run test-module-policy`.
- [ ] 2.2 Separate restore-only cache consumers from explicit canonical main producers. Validate build cache compatibility and retention cadence with repeated same-environment cold/warm runs, restored-seed changed-source invalidation, projected retention including other cache families, and available hosted cache/step metadata. Document the policy and expected cold rollout.
- [x] 2.3 Add workflow tests for complete producer selection, flavor/key reuse, canonical-only writes and no restore or save on tag pushes. Prove the assertions reject fixture changes that reintroduce bad ownership or tag exchange.

## 3. Command-contract ownership

- [x] 3.1 Build one race-enabled Gateway/host pair per combined contract invocation, keeping local direct entry points and per-test isolation. Run `mise run test-openai-chat-completions-adapter`, compare Go and TS scenario names/outcomes, and record after timings and build invocation counts under the baseline conditions.
- [x] 3.2 Route only the moved contract package away from the redundant CI invocations. Keep one explicit owner under the existing required race check; document coverage ownership. Test routing and fail-closed merge/publication evidence with `mise run test-ci-workflow` and the locked release tooling tests.
- [x] 3.3 Verify the new harness rejects known bad test outcomes and missing binaries rather than skipping execution. Run the retained ProviderWire semantic request mutants via `mise run test-providerwire-v4`; confirm the differential, not build/setup failure, rejects the copied mutant code.

## 4. Browser experiment

- [ ] 4.1 Run the measured worker/shard comparison with fresh, independent resources and the same package, config inputs and test census. Report timings, setup duplication, retry-free outcomes and limitations. If isolation or repeatable gains are not demonstrated, leave the default unchanged.
- [ ] 4.2 If an improvement qualifies, use the supported Gateway Playwright extension, document it, and validate the full E2E set and unchanged report and artifact gates on the selected configuration.

## 5. Integrated validation

- [ ] 5.1 Run structural/classifier and module-policy tests, module/short/full Gateway and race tests, cross-language integration and both command suites, plugin build/validation/frontend/E2E, lint and vet. Validate the registered baseline and run `mise run parity-check` if committed parity behavior changes.
- [ ] 5.2 Compare required versus workflow/publication timings without conflating them. Provide revision-tagged command results and a before/after review walkthrough; resolve verified findings with the supervisor before acceptance.

## Workflow follow-up

- Create the draft pull request with focused commits and measured/unmeasured distinctions; verify effective required checks and plugin validation on the current pushed head. No merge, publication or protection changes.
