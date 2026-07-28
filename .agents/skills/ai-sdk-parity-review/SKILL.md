---
name: ai-sdk-parity-review
description: Review any requested ai-sdk scope against the registered upstream Vercel AI SDK parity baseline. Use for PRs, the current git diff, a branch range, a package or directory such as providers/anthropic, a reported bug, a new feature, stream chunks, SSE framing, provider messages, provider request conversion, orchestration, tools, output, provider options, frontend interop, or conformance fixture changes.
---

# AI SDK Parity Review

Use this skill to review a user-defined scope for problematic upstream parity
issues against the baseline declared in `test/conformance/upstream.yaml`.

The scope comes from the user request. It can be a PR, the current git diff, a
branch range, a package, a directory, a provider, a bug report, or a feature
area. If the user does not specify a scope, review the current git diff.

## Workflow

1. Read `test/conformance/upstream.yaml` and `test/conformance/PARITY.md`.
2. Resolve the review scope:
   - PR: inspect the PR diff and relevant changed files.
   - Current git diff: inspect staged and unstaged changes.
   - Branch/range: inspect the diff against the requested base.
   - Package/directory/provider: inspect the implementation and matching tests
     in that tree.
   - Bug or feature: inspect the affected code path and any existing fixtures.
3. Identify the parity-sensitive layer and capability from the coverage map:
   core ai-sdk, provider contract, provider implementation, frontend interop,
   or conformance harness.
4. Read the matching upstream TypeScript implementation and tests for the
   registered baseline when available. Prefer sources in this order:
   - a local upstream checkout at the registered version or matching tag
   - the conformance tool package installed from the registered baseline
   - raw GitHub source for the registered package version
   Record which source was used when reporting findings.
5. Check conformance evidence appropriate for the layer:
   `expected.jsonl` for core stream/UI behavior, `expected-requests.jsonl` for
   provider behavior, `expected-object.json` for structured output, or a
   documented manual/gap entry.
6. For reported bugs or new upstream-visible behavior, check whether
   conformance-first TDD is possible. If so, prefer adding or updating the
   fixture before or alongside implementation.
7. Classify each mismatch using the divergence handling table below.
8. Report only problematic findings: missing features, behavioral deviations,
   possible bugs, missing or weak conformance coverage, and undocumented
   intentional deviations. Do not enumerate parity-preserving matches unless
   they explain why a suspected issue is not a problem.
9. For broad scopes, include a compact matrix with the reviewed package/area,
   upstream files or tests checked, conformance artifacts checked, mismatch
   classification, action taken, and residual risk.
10. For PR reviews, put accepted gaps, intentional deviations, and residual
   parity risk in the PR description or review summary. Do not add broad
   one-off findings to `test/conformance/PARITY.md` or
   `test/conformance/upstream.yaml` unless the user explicitly asks or the
   finding changes a long-lived project policy, coverage map, or baseline
   contract.
11. Include commands run and residual parity risk in the review summary.

## Divergence Handling

| Classification | Default action |
| --- | --- |
| Implementation bug | Add or update the relevant fixture when practical, fix the Go implementation, and rerun focused checks. |
| Upstream behavior change | Trace the change to upstream source or tests, regenerate the matching conformance artifact, adapt Go behavior, and rerun parity checks. |
| Intentional Go deviation | Keep only with a rationale. Document in the PR description or review summary unless it changes a durable baseline contract. |
| Coverage gap | Add a fixture or test when cheap and relevant. Otherwise document the residual risk in the PR description or review summary. |
| Large design gap | Stop and ask before deferring when the gap affects public API shape, wire format, provider contracts, or broad harness behavior. |

## Review Criteria

- Wire shape, chunk type names, JSON tags, ordering, and SSE framing must match
  upstream unless a documented deviation exists.
- Provider request bodies and behavior-affecting headers must match upstream
  request snapshots unless a documented normalization rule applies.
- Go implementation can use Go idioms, but upstream semantics must be preserved.
- New gaps must be fixed when practical. Accepted gaps and intentional
  deviations must be visible in the PR description or review summary; reserve
  `test/conformance/PARITY.md` and `test/conformance/upstream.yaml` updates for
  durable coverage-map, baseline-contract, or user-requested documentation.
- If conformance is not updated for a wire/provider-boundary change, the review
  must explain why existing coverage is sufficient.
- Provider implementation changes usually need `expected-requests.jsonl`
  coverage; core stream changes usually need `expected.jsonl` coverage.
- If no upstream equivalent exists, state that and classify the compatibility
  impact.
