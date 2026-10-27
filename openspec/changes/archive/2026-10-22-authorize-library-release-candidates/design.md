# Design

## Context

See [proposal.md](proposal.md) for motivation. This reassessment is based on main
`e9d676b90b7369debb26aa423ddb97ed9f98f4c5` (2026-10-22), not the archive's
`c11bbfa6` snapshot. #466, #467 and #469 are merged. #487 prepares the workflow
trigger for the separately authorized App exercise; live signed-PR evidence is
still pending. #468 records merged implementation separately from that evidence.

### Archived plan disposition

The archive at `../2026-10-20-activate-artifact-release-readiness/` is a
historical partial delivery record, not an active checklist to mechanically apply.
Its six checked tasks were accurate at archive time; later work is tracked in #468.
Do not rewrite the archived checkboxes to imply all of #245 is complete.

| Archived work | Current evidence and disposition |
|---|---|
| 1.1–1.5 | Implemented in #466. Reuse identity, generated-file, ownership, tagged-prerequisite and selected-consumer checks. |
| 2.1 | Implemented in #467: locked release-please preview with read-only transport. Do not recreate the version engine. |
| 2.2–2.3 | This slice: frozen pending library inventory, exact merged-source identity, check provenance and fresh selected validation. No publication handoff yet. |
| 2.4 | Deferred: publication, serialization, immutable tag/release retries and partial-failure recovery. Existing-tag candidates fail closed here, including same-target tags. |
| 2.5 | Only authorization documentation belongs here. Full bootstrap inventory and recovery remain deferred. |
| 3.1–3.5 | Deferred Gateway integration. Current specs still contain image-era language; do not copy those assumptions into this library slice or claim plugin work complete. |
| 4.1–4.3 | Deferred Renovate progression, first-tag/adoption policy and dependency-only release work. |
| 5.1 | Verified by #465; do not repeat authentication setup. |
| 5.2–5.3 | Deferred protection migration and maintainer approval. No settings changes here. |
| 5.4 | #469 supplies a controlled PR exercise, not production activation. #487 and live results remain pending. |
| 6.1–6.2 | Validate this slice only; whole-system integration and final #245 disposition remain outstanding. |

### Current implementation and gaps

- `scripts/release_readiness.py:verify_release` requires an open PR;
  `verify_candidate` expects two parents for GitHub's PR test merge. Neither is a
  post-squash authorization entry point. Reusing the CLI on a closed PR would fail.
- `verify_live_candidate` deliberately compares against moving main. Preserve that
  pre-merge behavior; a delayed merged release needs a distinct immutable-parent mode.
- `verify_library` already implements selected graph/origin/tag/freshness/consumer
  validation, but calls candidate-relative scripts. Its trusted execution boundary
  needs explicit tool paths for post-merge use.
- `.github/workflows/release-readiness.yml` emits console results, not a retained
  structured binding of tested head, base, tree and metadata. Job success alone
  cannot establish the squash-to-tested-tree relationship.
- `release/tooling/preview.mjs` builds pending PR proposals, not merged publication
  candidates. `verify-app-pr.mjs` verifies one open root exercise PR; it is not a
  general merged-candidate authorizer.
- The current `ci.yml` has source jobs `ci`, `race`, `docs-lint`,
  `module-resolution`, `parity-baseline`, `integration-test`, `conformance-test`.
  The plan preserves all seven. Image/publication jobs are not library evidence.

### Pinned upstream comparison

Read release-please 17.6.0 at `712fcf01effd08d7b0e7b1fd3861f2cb388bc8d1`,
`src/manifest.ts` (`buildReleases`, `createReleases`),
`src/strategies/base.ts` (`buildRelease`), and merged-release/duplicate-release
cases in `test/manifest.ts`. `buildReleases()` discovers merged pending releases;
`buildRelease()` returns `mergedPullRequest.sha` as the release target.
`createReleases()` calls `buildReleases()` again and performs publication/lifecycle
writes. Therefore it must not be used for this read-only slice or as an eventual
unrestricted rescan after validation. Keep the package lock unchanged. The pinned merged-PR iterator is capped at 200
PRs in `findMergedReleasePullRequests`; calling it unchanged is not proof of a
complete pending inventory.

The registered runtime baseline remains `ai@7.0.127`, commit
`eb77f09e3c06c28e860d92e0de941b143c2eecec`. No SDK runtime or wire-format behavior
changes are proposed. The authorization wrapper is repository policy around
release-please, not a change to Vercel parity or another version calculator.

## Goals / Non-Goals

**Goals:** Produce a reproducible read-only decision for each frozen merged library
candidate, with enough evidence to explain exactly why it is approved or blocked.
Demonstrate independent root readiness while downstream standalone readiness fails.

**Non-Goals:** No publication API, tags/releases, mutable labels, App write tokens,
normal automation activation, generated versions/changelogs, protection changes,
Gateway integration, Renovate edits, historical bypass or automatic recovery.
An `approved` report entry is an in-run policy result, not durable authority to
publish later. Preserve the manual live-exercise boundary.

## Decisions

### 1. Discover once through pinned release-please; freeze explicit records

Add a read-only merged-candidate entry point beside `preview.mjs`. Use
`Manifest.buildReleases()` with the existing locked client and `readOnlyFetch`.
Finish all discovery before validation; API failure aborts inventory rather than
approving a partial scan. Supply a read-only SCM iterator adapter that exhausts
the canonical merged-PR pages instead of accepting the upstream 200-PR cap.
Freeze that set before giving it to the pinned builder; preserve its pending-label
and body parsing behavior, and independently validate every returned candidate.
Capture main/config before and after discovery and abort if they move during the
scan. This prevents pagination movement from silently omitting a pending candidate.
A 201st-PR regression must prove the adapter does not silently truncate history. Explicit component selection supports root-only runs;
a full inventory reports unsupported Gateway without granting it authorization.

Copy candidates into immutable plain records, including repository ID, component,
version, tag, release title/notes, PR number, signed App head, squash SHA, parent,
tree, manifest/changelog blob identities, selected config and trusted tooling SHA.
Resolve authoritative Git/REST metadata rather than trusting candidate labels.
Block duplicate component or tag claims. Do not discover candidates a second time;
new arrivals require a later run. Never call `createReleases()`.

Alternative rejected: validating the latest preview or one merged PR followed by
a stock action run. Both can publish a different inventory from the checked set.

### 2. Retain explicit pre-merge evidence, then bind the squash revision

Extend the successful readiness path to emit a versioned JSON record only after
its final live recheck. Include repository/PR identity, component/version, head H,
base B, tested merge commit C, complete tree T, generated metadata digests and
trusted workflow/tooling revision. Upload the record through a pinned artifact
upload action only on readiness success; no credentials or environment dump.
Use a fixed artifact name/schema and documented finite retention. Expired or
missing evidence blocks authorization rather than waiving the check.

Retrieve evidence from the canonical readiness workflow/run/attempt for that PR
and head. Verify API-owned workflow identity/path, repository ID, event, exact
head, PR association, successful job and artifact association. An editable local
JSON file or artifact from another workflow is not proof. Validate strict bounded
JSON and read the named record without extracting arbitrary archive paths.
The existing API transport rejects redirects and non-API hosts. Artifact downloads
therefore need a separate bounded GET-only helper for GitHub-issued download URLs:
validate the HTTPS storage destination, strip API credentials before following the
redirect, enforce size/time limits and reject caller-supplied URLs. Do not relax
the general read-only API transport to accept arbitrary hosts or redirects.
Compare all record fields to Git objects and live PR data. The record is evidence
from an authenticated run, not a self-signed authorization token.

For this slice require a squash commit M with exactly one parent P, reachable from
canonical main, associated with the merged PR, and P = B. Reconstruct the expected
merge of B and H and require its complete tree and M's tree to equal T. Revalidate
the generated-only footprint and notes against B and M. Require immutable App
identity and a verified signed H; do not require the squash author to be the App,
since the maintainer may merge the PR. Reject post-validation title/body edits.

This deliberately blocks historical candidates without retained evidence and
unsupported merge/rebase topology. Report those gaps for later inventory/recovery;
do not silently approve them or rewrite history. Strict-base protection remains a
separate workstream; this gate catches stale merges even if protection was bypassed.

Alternative rejected: equating a PR check's head SHA with the squash SHA. GitHub
creates distinct objects. Display-name-only checks cannot establish tree identity.

### 3. Require exact-merge source checks and rerun library readiness

Require a completed canonical `ci.yml` push-main run whose head is exactly M,
with successful conclusions for all seven source jobs listed above. A failed
image/publication job in that same run is not library evidence and does not block
a library whose required source jobs succeeded; cancelled runs remain unusable. Bind workflow
ID/path and GitHub Actions provenance; paginate run/job lookups and use the newest
applicable run attempt. A skipped job, pending rerun, cancelled run or success on a
later main revision does not substitute. The merge pusher may be a maintainer;
do not copy the exercise's App-actor requirement onto push-main CI.

Keep PR readiness and exact-merge source checks distinct. A later successful
source run cannot reconstruct missing pre-merge readiness evidence. Final rereads
must reject changed candidate metadata or relevant run/attempt state during checks.

Run fresh selected-library validation in a detached clean checkout of M, comparing
prerequisite freshness and ancestry against P rather than today's main. Reuse
Python policy checks through an explicit merged-library entry point, factoring
state-independent identity/generated-file checks without relaxing the open-PR CLI.
Use authorization code, module-policy and consumer harnesses from the trusted tool
revision with explicit absolute paths and candidate working directories. Candidate
tests still execute as intended, in an unprivileged process without write credentials.
Do not import authorization modules from the candidate checkout. Collect resolved
prerequisite versions/origins and selected validation outcomes into the report.

Alternative rejected: global standalone validation. Root readiness must not depend
on provider source waiting for a future root tag. Ordinary coordinated-source CI
remains required and is not the same as all-module standalone validation.

### 4. Report independent outcomes; do not create a publication capability

Emit schema-versioned JSON with inventory/tooling identities, one status and
reason per candidate, exact run IDs/attempts and tree identities, and an explicit
`publicationEnabled: false` marker. The approved subset contains only successfully
validated frozen records; blocked provider results do not erase ready root results.
A blocked requested candidate yields a nonzero overall exit while preserving the
per-candidate report. Discovery failure emits no approved subset.

Check canonical target-tag and GitHub Release absence before and after validation.
An existing same-target tag, conflicting tag, or unknown state blocks this slice;
handling already-published and partial publication belongs to archived task 2.4.
Recheck main ancestry without requiring M to remain the tip. Newer unrelated merges
are allowed, while changed metadata, evidence or candidate tag state invalidates
that candidate. These checks do not remove the later publication race: the future
publisher must independently revalidate and consume only the same frozen records.

Read-only transport rejects mutation endpoints, redirects and GraphQL mutations.
Run deterministic tests against the actual pinned discovery API with mocked GitHub
responses and local Git repositories. No live GitHub release writes are tests.

### 5. Keep delivery separate from activation

No production authorization invocation is wired into the release workflow in this
slice. Source CI runs the new tooling tests; readiness CI can retain evidence after
the implementation merges. Existing `if: false` and `skip-github-release: true`
guards remain unchanged. The controlled root PR exercise can proceed independently
and does not by itself supply the new merged-release evidence contract.

## Risks / Trade-offs

- Required artifact retention introduces a historical boundary → fail closed and
  document manual disposition; never claim old green jobs prove the new contract.
- PR API and workflow runs use different revision fields → validate a fixture from
  real GitHub PR/run metadata read-only, preserve SHA/tree distinction in tests.
- Running candidate tests is code execution → use trusted harnesses, clean temporary
  checkouts, no write credentials and no secret-bearing environment inheritance.
- Proxy/GraphQL failures block progress → report transient evidence failures; retry
  the read-only run without reducing checks or mutating tags.
- Current source job names may change → keep an explicit trusted policy tied to the
  tooling revision and fail on missing expected jobs; no automatic name inference.
- Historical metadata overrides affect discovery → preserve frozen notes and require
  agreement with merged files; changes during validation invalidate the decision.

## Migration Plan

1. Land the evidence contract and read-only authorizer with its regression tests and
   documentation; keep release automation inactive and #245/#468 open.
2. Validate via local Git/mock GitHub fixtures, including a genuine selected root
   validation with deliberately unready downstream source. Do not merge a real
   release PR or publish to obtain planning/implementation test evidence.
3. Archive this new change only after its scoped requirements are implemented and
   verified. Sync only this slice's deltas. Preserve the broad partial archive.
4. A later reviewed publication/recovery change consumes equivalent frozen records,
   with live rechecks and immutable-tag behavior. Protection migration and final
   activation require their own disposition; this report is insufficient approval.

## Verification results

All 13 tasks completed. JavaScript tooling: 26 passing tests. Python release
checks: 32 tests, 31 passing and the explicit integration test skipped in the
fast run. The separate integration run passed all seven authorization tests,
including real root standalone and pinned ai@7.0.127 frontend consumer validation
with deliberately broken downstream provider tests. Go release configuration,
workflow lint and documentation checks passed. Two independent reviewers found
no actionable issues.

Live read-only discovery against main e9d676b90b7369debb26aa423ddb97ed9f98f4c5
returned an empty inventory. Positive authorization and rejection provenance are
covered by Git fixtures and mocked GitHub responses, not a live merged release.
No tags, releases, activation settings or repository protections were changed.
