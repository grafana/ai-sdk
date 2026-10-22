# Library publication integration

Publication defaults to disabled. The command and workflow require an explicit
`RELEASE_LIBRARY_MODE`; see [activation and completion](ACTIVATION.md).
This slice tests publication with mocked GitHub writes through the pinned
release-please 17.6.0 writer. It does not establish live publication evidence.

## Candidate and write boundaries

The publisher discovers one frozen inventory using the existing
[merged candidate authorizer](AUTHORIZATION.md). It validates each selected library
immediately before publishing, including retained readiness evidence, exact squash
source CI, standalone builds, consumer checks and published prerequisite freshness.
It does not trust a saved authorization report or rescan for new candidates.
An unready provider does not prevent ready root from proceeding. Gateway remains
unsupported. Duplicate component or tag claims block the affected candidates.

The writer permits only four operations for the current candidate:

1. Create its exact tag at its immutable squash SHA.
2. Create its release with the pinned builder's exact name, notes and prerelease
   flag, targeting that same SHA. Draft publication is unsupported.
3. Add `autorelease: tagged` to its PR.
4. Remove `autorelease: pending` from that PR after the tagged label exists.

Live metadata, source/readiness run attempts, artifact identity, canonical ancestry
and publication state are reread before every mutation. Existing tags are never
moved or deleted; existing releases are never edited. Tag and release creation are
verified afterward. A tag-create conflict is acceptable only if a fresh read proves
the exact intended target. API errors stop that candidate's remaining operations.
Other independently validated candidates retain their own outcomes.

The process rejects overlapping invocations and handles candidates sequentially.
Future workflow integration must retain the canonical workflow's non-cancelling
concurrency group. This in-process guard does not coordinate arbitrary separate
processes. Canonical serialization and repository tag protections remain necessary
external boundaries; API checks cannot make separate GitHub operations atomic.
This change neither changes protections nor enables that workflow integration.

## Recovery

Every retry repeats authorization and selected-library validation. Missing or
expired evidence, failed required jobs, changed metadata or unavailable prerequisites
still block recovery. Historical candidates receive no automatic exemption.

| Observed canonical state | Permitted action after validation |
| --- | --- |
| No tag or release | Create exact tag, then release and labels |
| Exact tag, no release | Create missing release and finish labels |
| Exact tag and matching release | Finish missing lifecycle labels only |
| Conflicting tag, release metadata, or release without verifiable tag | Block without modifying that state |

Annotated tags must resolve to the candidate SHA through a bounded, acyclic chain.
Matching release metadata includes the target SHA, tag, name, notes, draft status
and prerelease flag. A timeout or lost response is ambiguous: do not retry a write
blindly in the same run. Start a new inventory/validation run and inspect canonical
state. Tests cover lost responses both before and after successful mutations.

Adding the tagged label before removing pending keeps unfinished work discoverable.
If pending was successfully removed but its response was lost, normal discovery no
longer selects that completed PR; its matching tag, release and labels already
exist. An explicitly supplied frozen candidate can also be revalidated without
repeating completed writes. The CLI accepts no saved inventory or report input.
The read-only authorizer continues to reject all existing tags/releases; only the
publisher has the matching-state recovery path.

## Bootstrap and activation still pending

Root PR #490 is the first bootstrap candidate: version `0.1.0-alpha.2`, intended
tag `v0.1.0-alpha.2`, squash `c8e497f3ca45571ef57c3556ebde32a836e8b918`.
Its previous read-only authorization is historical evidence, not permission for a
later write. Before activation, discover again and revalidate the current canonical
inventory, artifacts, source checks, tag/release state and selected library.

The [activation rollout](ACTIVATION.md) connects the entrypoint to the canonical
serialized workflow with separate read and App write credentials. Live settings
and publication still require explicit approval. Do not switch to stock
`Manifest.createReleases()`, which rescans candidates.

The remaining live proof is exact root tag/release creation, lifecycle completion
and downstream consumption of the published root version. Provider publication
requires each provider's own prerequisites and readiness. Issues #245 and #468
remain open for activation and that evidence.

## Validation

`npm test --prefix release/tooling` tests actual pinned writer request payloads,
per-operation rechecks, independent outcomes, late arrivals, overlapping runs,
partial recovery, conflicting state and the disabled CLI. Python release tests
cover the shared Git/selected-library validator. No test creates a live GitHub tag
or release, and no generated version or changelog is edited by this integration.
