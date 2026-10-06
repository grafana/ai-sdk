## 1. Baseline and Coverage Artifacts

- [x] 1.1 Create the upstream parity baseline manifest with the current conformance TypeScript package versions, upstream repository metadata, verification commands, status, and known gaps.
- [x] 1.2 Add the parity coverage map covering wire format, SSE framing, provider interfaces, provider request conversion, orchestration, tools, structured output, provider-specific features, frontend interop, and known gaps.
- [x] 1.3 Update conformance documentation to explain how the manifest, coverage map, generated snapshots, and request snapshots relate to each other.

## 2. Validation and Commands

- [x] 2.1 Add a baseline validation script that compares manifest package versions with `test/conformance/tools/package.json` dependency pins and reports mismatches.
- [x] 2.2 Add tests or fixture coverage for the validation script success and mismatch paths.
- [x] 2.3 Add `mise run parity-check` to run baseline validation, conformance tool typecheck, and the configured conformance signal.
- [x] 2.4 Add `mise run parity-upgrade` as the documented entrypoint for package bumps and conformance snapshot regeneration.

## 3. Agent Guidance and Skills

- [x] 3.1 Update `AGENTS.md` with concise parity-sensitive work rules and required divergence classification.
- [x] 3.2 Add `.codex/skills/ai-sdk-parity-review/SKILL.md` for scoped parity review across PRs, diffs, branch ranges, packages, directories, providers, bugs, and feature areas.
- [x] 3.3 Add `.codex/skills/ai-sdk-parity-upgrade/SKILL.md` for upstream baseline upgrade work.
- [x] 3.4 Ensure the new skills reference the baseline manifest and mise tasks instead of duplicating command internals.

## 4. CI and Verification

- [x] 4.1 Decide whether `parity-check` should run full conformance or a documented stable subset for the initial implementation.
- [x] 4.2 Wire baseline validation or `parity-check` into CI according to the chosen initial enforcement level.
- [x] 4.3 Run `mise run parity-check`.
- [x] 4.4 Run `mise run test-conformance` if `parity-check` does not already run the full conformance suite.
- [x] 4.5 Run `mise exec -- openspec status --change "add-upstream-parity-governance"` and confirm the change is apply-ready.

## 5. Layered Coverage and Confidence Suite Refinement

- [x] 5.1 Expand the parity coverage map into core ai-sdk, provider contract, provider implementation, frontend interop, and conformance harness layers.
- [x] 5.2 Document conformance-first TDD guidance for bugs and features that can be represented through recorded provider chunks, provider request snapshots, or structured output snapshots.
- [x] 5.3 Update repo-local parity skills so agents classify work by layer and use conformance fixtures as confidence evidence, not only as a final parity check.
- [x] 5.4 Update OpenSpec requirements and design notes to reflect the layered map and conformance confidence-suite role.
- [x] 5.5 Merge parity check and parity review into one scoped parity review skill that reports problematic findings for the requested scope.
