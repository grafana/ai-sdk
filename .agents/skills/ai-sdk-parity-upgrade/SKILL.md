---
name: ai-sdk-parity-upgrade
description: Upgrade the registered upstream Vercel AI SDK parity baseline for the Go ai-sdk repo. Use when bumping upstream ai or @ai-sdk/* package versions, regenerating conformance expectations, reconciling upstream behavior changes, or moving test/conformance/upstream.yaml forward.
---

# AI SDK Parity Upgrade

Use this skill to move `test/conformance/upstream.yaml` to a newer upstream
baseline.

## Workflow

1. Read `test/conformance/upstream.yaml`, `test/conformance/PARITY.md`, and
   `test/conformance/README.md`.
2. Identify the old upstream target and the affected providers.
3. Inspect upstream changelog, source, and tests for parity-sensitive changes
   between the old baseline and the mature stable package set selected from the
   npm `latest` release lines.
   Prefer sources in this order:
   - a local upstream checkout at the registered version or matching tag
   - the conformance tool package installed from the registered baseline
   - raw GitHub source for the registered package version
   Record which source was used in the PR description when it explains a
   finding or accepted risk.
4. Run `mise run parity-upgrade` to update `test/conformance/upstream.yaml`,
   update `test/conformance/tools/package.json`, refresh `test/pnpm-lock.yaml`,
   regenerate conformance expectations, and validate baseline consistency.
5. Run focused Go tests for implementation areas affected by upstream changes.
6. Classify every mismatch using the divergence handling table below.
7. Fix implementation bugs before marking the upgrade complete. Stop and ask
   before deferring a large design gap that affects public API shape, wire
   format, provider contracts, or broad harness behavior. Document
   accepted deviations, gaps, and residual parity risk in the PR description.
   Do not add broad one-off upgrade findings to
   `test/conformance/upstream.yaml` or `test/conformance/PARITY.md` unless the
   user explicitly asks or the finding changes a long-lived project policy,
   coverage map, or baseline contract.
8. Update the coverage map when the upgrade adds a new provider capability,
   stream behavior, frontend interop concern, or harness limitation.

## Divergence Handling

| Classification | Default action |
| --- | --- |
| Implementation bug | Add or update the relevant fixture when practical, fix the Go implementation, and rerun focused checks. |
| Upstream behavior change | Trace the change to upstream source or tests, regenerate the matching conformance artifact, adapt Go behavior, and rerun parity checks. |
| Intentional Go deviation | Keep only with a rationale. Document in the PR description unless it changes a durable baseline contract. |
| Coverage gap | Add a fixture or test when cheap and relevant. Otherwise document the residual risk in the PR description. |
| Large design gap | Stop and ask before deferring when the gap affects public API shape, wire format, provider contracts, or broad harness behavior. |

## Rules

- Do not update the registered baseline without reviewing generated
  `expected.jsonl` and `expected-requests.jsonl` diffs.
- Always upgrade to the newest coherent stable package set from the npm
  `latest` release lines that satisfies `test/pnpm-workspace.yaml`'s
  `minimumReleaseAge`; do not bypass the age gate or pin a beta, canary, or
  partial package set unless the user explicitly asks for that exception.
- Do not treat snapshot churn as automatically correct; trace behavior changes
  to upstream source or tests.
- Treat regenerated conformance as the upgrade confidence suite: provider
  implementation changes should be reviewed through request snapshots, and core
  stream behavior should be reviewed through UI chunk snapshots.
- Live provider recording remains manual and must not be required for normal CI.
- Keep package pins, lockfile changes, regenerated snapshots, and baseline
  metadata in the same change when practical. Keep residual finding summaries in
  the PR description unless they need durable parity metadata.
- End the PR description with a short residual-risk section listing accepted
  gaps, intentional deviations, skipped fixtures, and follow-up owners when
  known.
