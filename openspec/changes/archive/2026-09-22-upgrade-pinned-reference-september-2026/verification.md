# Upgrade verification — 2026-09-22

## Summary

The frozen nine-package target in `selected-target.json` was applied without
reselection. The supported-surface assessment and dispositions are in
`test/conformance/PARITY.md`; pins are not a claim of exhaustive feature parity.
Twenty-three new packages (#207–#229) and twenty-two reused packages have verified
`upstream-sync` labels. Remaining behavior is not claimed implemented.

| Review dimension | Result |
| --- | --- |
| Completeness | Core/Agent/Output/UI, provider contract/adapters, React, Gateway, middleware/registry and harness are assessed by surface; unsupported families and unresolved API boundaries are explicit. |
| Correctness | Required compatibility corrections have failing replay or focused regression evidence, exact target source/tests, and passing candidate checks. |
| Coherence | No public Go API or producer dependency was added; SDK transport/authentication remains authoritative. Mantle adoption is explicitly separate. |
| Evidence limits | Fixtures prove represented behavior, not all inputs/interleavings. Vertex transport lacks a separately registered TS package; live validation is limited to the stated Mantle unary cases. |

## Target and provenance

Eight packages resolve to `08ae5ad05bc12496dd1ffcf64e34419e0831300d`;
provider 4.0.17 resolves to `1a82df1c7b6239f4cefe9fb73c357b3a6529c629`.
The starting reference was `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`.
Exact Git archives were read without switching the shared upstream checkout.
All four consumer manifests and the lockfile use the selected package set.

Existing provider inputs were not changed. Two streaming inputs were copied
byte-for-byte from target source: OpenAI `parallel-tool-call-wrapper.1` and
compatible `placeholder-first-chunk`. Inventory verifies all imports and source
objects. New unary evaluation, guard-content and parallel-wrapper counterparts
are indexed as unimported: evaluation is outside the supported product family,
guard-content request support is #223, and the harness does not replay unary
OpenAI fixtures. Existing non-streaming coverage limits were not relabeled as
passing provider proof.

The wrapper input itself has malformed JSON on lines 1 and 6. Target generation
emits two error chunks, valid expanded child calls and an error finish. Go now
matches that result; no brace was added, event spliced, or input otherwise repaired.
Generation was repeated and all 301 expected-artifact hashes were stable.

## Implementation → requirement review

| Correction | Need and executable proof |
| --- | --- |
| Invocation-wide text/reasoning IDs | Regenerated multi-step Anthropic fixtures exposed reused IDs. `streamtext_part_ids_test.go` covers unique/reused/generator-collision cases; frontend schema parsing and assembly preserve separate steps. Exact reference: ai stream-text implementation/tests. |
| Anthropic caller metadata | Target language-model/prompt conversion attaches caller to server calls and web search/fetch results. Recorded/imported replay failed first. `caller_metadata_test.go` covers unary, stream, direct/programmatic callers and native continuation. Its search-error case exposed and corrected dropped error results. |
| OpenAI assistant history | Target uses easy-input string content rather than incomplete output messages. Two existing request fixtures changed. Seven focused cases cover absent/empty text, unstored IDs, phase and stored references; six fail with the old encoder in an isolated copy. |
| OpenAI apply-patch | Target counts completed local patch calls as tool-calls. Existing imported fixtures went red; unary and stream/request suites now pass. |
| Parallel wrapper expansion | Mandatory target streaming import exposed missing expansion. Only undeclared wrappers with valid declared recipients expand atomically; metadata preserves identity. Tests cover declared/invalid wrappers, unary/stream fallback, complete/partial/duplicate scalar continuation and cache breakpoints. Multipart result limits remain explicitly #221, not a full-support claim. |
| Recoverable malformed SSE | Typed SDK iteration terminated on the exact wrapper fixture's malformed JSON. The adapter now uses the SDK framing decoder with event-local JSON errors while retaining SDK HTTP/auth and preflight behavior. One-request recovery, retained text/usage with an error finish, single decoder ownership and EOF wrapper flushing are regression tested. The post-publication review corrected the original success-finish assertion; see below. |
| Compatible response metadata | Imported fixture plus a focused red test exposed premature placeholder metadata. Metadata now waits for real ID/model/time and zero timestamps are omitted in both modes, matching target get-response-metadata. |
| Gateway unary warnings | Five exact-client/differential assertions failed on target warning forwarding. Shared validated warning decoding now preserves them; malformed warning cases fail. Client-owned request/response metadata and server privacy normalization remain unchanged. |

All upstream implementation/test references are to the exact package sources
listed above, not upstream main or the closed #204 proposal. The bidirectional
review also found older native SDK header/metadata/raw-output omissions (#229),
which server-captured request snapshots alone cannot detect.

## Approved publication ordering

The owner approved publishing the OpenAI correction in this upgrade, then
repinning Bedrock's OpenAI dependency in a separate change:
[#207](https://github.com/grafana/ai-sdk/issues/207). Current Mantle still resolves
`v0.0.0-20260910193046-bcca929c67e5`. The new consumer regression is preserved in
the external `mantle-follow-up/` artifacts, not skipped or weakened in this suite.
Existing public-module checks and tests remain enabled; the producer has its own
committed regression. No unpublished pseudo-version or production replacement
was introduced.

Live tests on 2026-09-22 in `us-east-2`, using dummy history and a 256-token limit:

| Mantle model | Old encoding | Target encoding |
| --- | --- | --- |
| GPT-OSS-20B `/v1/responses` | HTTP 200 envelope with failed status and invalid-prompt validation error | Completed; recalled dummy code word |
| GPT-5.6 Terra `/openai/v1/responses` | Completed on explicit retry after an initial timeout | Completed; recalled dummy code word |

Actual Go Mantle calls reproduced GPT-OSS failure with the published dependency
and success with the corrected workspace producer. This is an existing,
model-specific defect, not a target-induced regression or a permanent accepted
deviation. Web-search includes (#227), streaming and other model/region behavior
were not live-tested. Temporary SSO credentials were not printed or persisted in
probe artifacts. These probes are not committed provider-conformance recordings.

## Validation

Passed on the candidate after integrating main, with Go 1.27.1 and golangci-lint
2.13.2:

- `mise run parity-check`: baseline/tooling tests, source inventory, installed
  provider shape, ProviderWire and all Go replay suites. Source was available;
  no shape check was skipped. Warning-only shape limitations remain #35.
- `mise run test-integration`: 30 frontend tests and 28 real Gateway command tests.
- `mise run verify-module-resolution`: fresh public cache, readonly standalone
  tests for every published module; `mise run verify-ai-gateway-boundary`.
- `mise run test-short`, `mise run build`, `mise run vet`, `mise run lint`, and
  `mise run lint-docs`. A fresh external lint cache avoided stale diagnostics
  pointing at deleted files in another worktree; no rule was weakened.
- Root and OpenAI/compatible/Anthropic/Grafana provider race tests.
- `mise run build-ai-gateway-image` and the native image's CI-equivalent non-root,
  read-only, network-none, IPv6-disabled readiness smoke test.
- Strict OpenSpec validation and `git diff --check`. Clean-tree formatting and
  post-archive gates are rerun on the signed commit; remote multi-platform image
  validation is reported separately through PR checks.

The Gateway witness was reviewed against target request conversion, error/stream
consumption, unchanged finite request declarations and successful differential/
mutation/runtime tests before its versions were updated. Verification date was
set only after reviewed evidence, then full parity was rerun successfully.

## Registration and residual decisions

Searches covered open labeled and all-state unlabeled provider/capability/behavior
queries, bodies, discussion and linked PRs. Closed #29/#31/#36/#161/#180 were
recognized as implemented, #153 as superseded, and #204 as unmerged; none became
an invented live owner. Related Gateway packages were reused rather than cloned.
Labels were added without replacing other metadata, and verified afterward.
No comments, closures/reopenings or assignments were made.

Constructor URL policy is #228; expanded UI outcome/step hooks have unresolved
scope overlap with #181; empty-body Chat issue #48 does not establish Mantle Chat
acceptance. Other new APIs remain decisions within their packages, not implicit
approval. TypeScript-only product families and private Gateway behavior are
explicitly excluded in PARITY.md, not silently omitted from the inventory.

## Post-publication review corrections

Independent review of published `7d5bafff` identified three OpenAI stream defects:
duplicate SDK decoder construction/incorrect ownership, loss of buffered wrapper
input on early EOF, and malformed-frame errors overwritten by a later completion.
The parent reproduced them with focused failing tests before correcting them.
Raw responses still use the SDK endpoint/auth/options; one framing decoder owns
the stream. Finish state and usage are retained until input is flushed at EOF,
so malformed events before or after completion cannot report success.

Stronger tests assert exact incomplete/duplicate/conflicting child requests and
full Anthropic error blocks. Isolated mutations dropping ungrouped outputs or
forcing `unavailable` error codes fail these assertions. A synthetic HTTP/core
two-step test executes each expanded child once and verifies the second native
request's grouped result. It lives in the existing conformance test module;
production module dependencies and authentic provider inputs are unchanged.
Frontend scenarios separately verify unfinished wrapper input and failed tool
calls without output or continuation. These additions are focused synthetic
tests, not new provider recordings.

Round two promoted #208 into this upgrade: recovered malformed data could expose
valid local calls that the old core executed despite an error finish. The real
HTTP/core regression reproduced both executions and a second request. Dispatch
now requires stop/tool-calls, preserving prior approval processing; continuation
requires a result or denial for every client call. Tests cover all finish reasons,
missing finish, approval decisions, malformed prefixes and malformed data after
completion. The old incomplete-step specification was corrected accordingly.

The review loop completed three rounds. Oracle checks accepted the bounded SDK
ownership correction and promotion of #208; final fresh correctness/evidence
reviews found no further actionable issues. Parent validation passed full
`parity-check`, root and affected provider race suites, fresh standalone module
resolution, 32 frontend plus 28 command tests, lint, strict specification and
documentation checks. Overlay mutations confirmed the strengthened assertions
reject missing outputs and lost error codes. Plain EOF without a terminal/error
still retains the separately documented Go incomplete-stream policy.

Initial CI results above belong to the published head. Review fixes remain local
until separately committed and published; they do not inherit its remote CI proof.
Review reports, red/green logs and mutation evidence are under `review-loop/` in
the external artifact root below.

## Durable artifacts and branch

Research, source archives, live probes, red/green logs, issue bodies and backups:
`/home/nara/.local/state/ai-sdk-parity/nrbrd-parity-upgrade-22-09/`.
The current branch remains `nrbrd/parity-upgrade-22-09`. Main through #203 and its
toolchain changes were integrated. Existing workflow commits from #205 remain
included, so this branch does not depend on a later unmerged tooling change.

A transient index lock initially prevented stashing; the non-fail-fast command
sequence then applied an older stash. Recovery compared every tracked diff and
untracked artifact with the pre-operation backup, removed only the unintended
checksum/UPGRADE_PLAN additions, and retained the original stash unchanged. The
successful retry used fail-fast commands and the exact newly created stash ID.
This incident did not change the selected target or supply assessment authority.
