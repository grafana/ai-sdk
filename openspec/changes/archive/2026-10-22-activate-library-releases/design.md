# Design

Use repository variable RELEASE_LIBRARY_MODE: absent disables all release writes;
root authorizes only original root PR #490, libraries enables supported library
publication and preparation. Keep canonical workflow path, push/main OIDC binding
and non-cancelling concurrency. Remove obsolete App smoke/exercise jobs from the
active workflow; their tooling remains as historical evidence.

Wait for source CI at the workflow's tooling SHA before running trusted tooling.
Read Actions/artifacts through github.token; mint the existing scoped App for writes
only after install/tests. Verify repository scope, squash defaults, strict required
checks including readiness, and active immutable all-tag rules before mutations. GitHub redacts bypass actors
from read-only tokens: RELEASE_TAG_RULESET records the maintainer-reviewed ruleset
ID and updated_at revision after inspecting the full bypass list. Changed revisions
block; missing bypass fields alone never establish no-bypass proof. No new App
administration permission is added.
Publish frozen candidates sequentially using existing validator and recovery engine.
Root-only mode never prepares another PR. Library mode prepares per component using
pinned Manifest APIs and excludes Gateway. A blocked merged candidate suppresses
preparation of its component only; do not inherit the stock global pending-PR veto.

The release config must remain byte-semantically unchanged for #490's retained
identity. For preparation only, discard the temporary lastReleaseSha override so
real published releases delimit subsequent notes; bootstrapSha still covers modules
without releases. Existing pinned Manifest behavior supports this distinction.

Renovate receives a final exact first-party library package rule overriding disables,
weekly grouping and delay. The rule becomes effective when this activation PR merges; approval must account
for both the mode variable and dependency PR creation. No automatic merges for prerequisite updates.

The pinned release-please 17.6.0 implementation is the tooling reference. SDK baseline
ai@7.0.127 is unchanged. GitHub rules and Renovate schema are checked against primary
API/documentation before implementation. Remaining proof is live App publication,
Go proxy consumption and provider progression, not another implementation slice.
