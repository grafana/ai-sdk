# Design

## Reassessment

Archived activation tasks2.4–2.5 are this slice. Tasks2.1–2.3 now exist through
#467, #469 and #489; #492 fixes transport after synchronous validation. Root #490
passed live authorization. Gateway image removal is already merged and out of scope.
Pinned release-please17.6.0 GitHub.createRelease supports forceTag: it POSTs a
lightweight ref then a release and swallows ref422. Never use its unrestricted
Manifest.createReleases scan. Our boundary verifies an existing tag before and
after any422 and denies ref updates/deletion. Versions/notes come from frozen
Manifest.buildReleases; draft/prerelease flags must also be retained.

## Decisions

Reuse authorization and trusted local validation. Preserve the read-only default
which rejects all existing tags. A publisher-only state inspector accepts absent,
matching-tag-only or matching-release states, checks peeled tag target and exact
release metadata, and rejects conflicts. Recovery never replaces authorization:
even label-only recovery requires current evidence and fresh library validation.
Snapshot state before validation and reject any unexplained movement afterward.

The writer consumes only this invocation's frozen inventory, not JSON reports.
Authorize candidates sequentially; a blocked provider cannot block an independent
root. Recheck PR metadata, evidence attempts/artifact, ancestry and expected tag/
release state before each mutation. Use actual pinned GitHub.createRelease with
forceTag for absent/tag-only state. Verify resulting immutable state. Add
' autorelease: tagged' (without the leading space) before removing pending so
label failures remain discoverable. Do not post release notification comments.
Existing exact releases skip creation; conflicting names/notes/flags/targets block.

An exact-request transport allows only the candidate tag/ref, release metadata and
its two lifecycle label operations; no PATCH, tag deletion, arbitrary comments or
writes to other components. Allow one attempt per mutation per invocation to avoid
invisible retry after ambiguous failures. Serialize calls in-process and preserve
the workflow's existing non-cancelling concurrency group for eventual activation.
No local mutex can coordinate arbitrary external publishers: activation must retain
one canonical workflow and immutable tag protections. Recheck conflicts on retries.

The production CLI has a literal disabled gate before credentials, discovery or
network access. Existing workflow stays inactive. Tests call the engine using
mocked GitHub responses; no publication is exercised live. Bootstrap inventory,
expired evidence recovery, protections, Gateway and activation remain later work.

## Reference

release-please17.6.0 (commit712fcf01effd08d7b0e7b1fd3861f2cb388bc8d1) manifest and
github-api source/tests inform publication payloads. Runtime upstream ai@7.0.127
is unchanged; this is not runtime parity work.
