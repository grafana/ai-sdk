# Upgrade verification

[PR #230](https://github.com/grafana/ai-sdk/pull/230) advances the frozen reference
and required compatibility corrections, not the entire parity backlog.
[selected-target.json](selected-target.json) retains exact versions, publication
times and per-package source commits, including the independently resolved
provider package. [PARITY.md](../../../../test/conformance/PARITY.md) owns current
coverage and issue links; issues own detailed findings and acceptance criteria.

## Provenance

All four TS consumers and the lockfile use the approved target without reselection.
Existing provider inputs are unchanged. The two added streaming inputs are exact
upstream copies: OpenAI parallel-tool-call-wrapper.1 and compatible
placeholder-first-chunk. The wrapper's malformed first/last JSON frames are
preserved and produce the upstream error/child-call sequence, not repaired input.
Repeated generation, including the UI generators, left 301 expectation hashes
unchanged.

Unary evaluation, guard-content and wrapper counterparts remain indexed as
unimported: evaluation is outside scope, guard-content requests are #223, and the
harness does not replay unary OpenAI fixtures. Synthetic error tests and live
Mantle probes are not provider-conformance recordings.

## Regression evidence

| Correction | Proof |
| --- | --- |
| Cross-step text/reasoning IDs | Failing Anthropic replay, root collision tests and frontend schema/assembly scenarios. |
| Anthropic caller/error continuation | Replay plus unary/stream/full native error-block assertions. Forcing unavailable instead of the supplied error code fails mutation controls. |
| OpenAI assistant history | Changed request snapshots and seven focused cases; six fail with the old encoder. Stored references and phase remain covered. |
| Apply-patch finish mapping | Imported unary/stream tests cover completed local calls and tool-calls finish. |
| Parallel wrappers | Exact import, atomic expansion/fallback and scalar grouping tests; incomplete/conflicting output assertions reject a mutation dropping child results. Real HTTP/core tests execute each child once and inspect the second request. |
| Decoder ownership and error finishing | Red/green tests cover one SDK decoder, constructor-consumed bytes, cancellation, buffered input at EOF and malformed data before/after completion with retained usage. |
| Tool dispatch/continuation (#208) | Recovery made error-finish execution newly reachable; a real HTTP/core regression failed before correction. The finish/approval matrix and frontend tests verify visible calls, zero prohibited execution, result/denial-based continuation and preserved approval observations. |
| Compatible metadata | Exact placeholder import plus focused regression tests cover delayed metadata and absent zero timestamps. |
| Grafana unary warnings | Five target client/differential assertions failed before correction; forwarding and malformed-warning tests now pass. |

Three independent review rounds found and verified the decoder/finish/buffering
corrections and promoted #208 from deferred work into the upgrade. Final fresh
correctness and evidence reviews found no further actionable in-scope issues.
No production Go API/dependency was added, comparison relaxed, or fixture input
rewritten. Behavioral requirements are retained in the change's capability specs.

## Validation

Local validation of the implementation and review fixes passed:

- Full parity checks, root tests, root and affected-provider race suites, fresh
  public-cache standalone module resolution and the Gateway module boundary.
- Integration: 32 frontend and 28 real-command tests after review fixes.
- Build, short tests, vet/lint/docs, strict OpenSpec, formatting and diff checks.
- Native Gateway image build and non-root/read-only/IPv6-disabled readiness smoke.

After rebasing onto the updated workflow branch (including #194), parity,
standalone-module and formatting checks passed again. The rebased tree matched
the reviewed upgrade plus the new base exactly.
[CI for rebased head 6d17524f](https://github.com/grafana/ai-sdk/actions/runs/35855656442)
passed, including integration, module resolution and multi-platform image
validation; signed-commit and specification checks also passed. Later commits
require their own CI results. The Gateway witness was reviewed against exact
client source and differential/mutation/runtime evidence before re-attestation.

## Remaining limits

- [#207](https://github.com/grafana/ai-sdk/issues/207) retains the approved
  producer-first Mantle adoption split. Live unary probes in us-east-2 found old
  assistant encoding failed on GPT-OSS-20B but worked on GPT-5.6 Terra; corrected
  encoding worked on both. Streaming, other models/regions and web-search includes
  were not live-tested. Published consumer adoption is not inferred from workspace
  success.
- Multipart/reference limitations (#221), native SDK metadata/raw-output gaps
  (#229), plain-EOF policy and unsupported families remain in the coverage map.
  Constructor and UI-hook scope questions remain decisions, not API approvals.
- Work registration searched open/closed and labeled/unlabeled candidates,
  inspected discussions/linked PRs and verified upstream-sync labels without
  replacing other metadata. #208 is addressed here; #193 was resolved by the base's
  #194. Other issue links indicate work packages, not implementation completion.

Raw source archives, probe results, red/green/mutation logs and review reports are
retained outside Git under
`/home/nara/.local/state/ai-sdk-parity/nrbrd-parity-upgrade-22-09/`.
