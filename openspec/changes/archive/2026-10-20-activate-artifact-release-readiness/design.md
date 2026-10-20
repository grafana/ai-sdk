# Design

## Context

See [proposal.md](proposal.md) for scope. Inspection on 2026-10-20 found:

- Main is `c11bbfa6b465ee2b3491540d1521e049800c2e3b`. #21 merged an
  inactive workflow. Its selected-library validator already checks App author,
  branch, manifest delta, merge parents, tagged dependency origins and snapshots.
- Root middleware exclusions and preview version alignment are already fixed.
  Preserve their regression tests rather than reimplementing them.
- #428 is open at `5663d734a9d656374993bb13ad6f2cbc5651ebf2`.
  Its workflow builds/validates the plugin, runs frontend, Grafana E2E and test
  host suites, and retains main/tag packages. Its configuration adds the Gateway
  package.json version updater. Integrate its eventual merged revision.
- The main ruleset does not require `release-readiness`, permits merge and squash,
  and has strict required-status checks disabled. Squash defaults already use
  `PR_TITLE` and `PR_BODY`.
- The external broker configuration and access policies were successfully applied.
  PR #465 subsequently verified App token issuance, repository read scope and
  revocation. Signed PR creation and write operations remain untested. Keep private
  infrastructure details out of this public repository.
- The registered Vercel reference is `eb77f09e3c06c28e860d92e0de941b143c2eecec`
  (`ai@7.0.127`). No runtime parity change is proposed.

The release tooling reference is release-please **17.6.0**, commit
`712fcf01effd08d7b0e7b1fd3861f2cb388bc8d1`, matching the pinned action.
Relevant source:
[manifest](https://github.com/googleapis/release-please/blob/v17.6.0/src/manifest.ts),
[strategy](https://github.com/googleapis/release-please/blob/v17.6.0/src/strategies/base.ts),
[plugin contract](https://github.com/googleapis/release-please/blob/v17.6.0/src/plugin.ts).
The strategy returns the merged PR SHA as release target. Manifest publication
enumerates all pending merged release PRs; allowing the stock action to re-scan
after validating only one candidate is insufficient.

## Library slice reassessment (2026-10-20)

The scaffold already selects one standalone library and checks merge parents.
Extend those checks rather than introducing another release driver in this slice.
Verify the immutable bot user ID 332654929 (App 5038074) and canonical repository
ID 1315458055. Validate the configured single-release title/body and exact
changelog prepend against release-please 17.6.0. Gateway stays fail-closed.
Freshness conservatively includes every owned file below a runtime Go file
containing an embed directive, even normally excluded documentation or hidden
assets. This can over-block but cannot omit an embedded input by extension.
PR #465 already proved token issuance, repository read access and revocation;
do not repeat authentication work. Write operations and signed PR creation
remain unproven. Required-check protections and post-merge authorization remain
later work; a PR check alone cannot prevent stale greens after main advances.

## Goals / Non-Goals

**Goals:** Make each independently releasable component progress with evidence
for exactly its selected source and dependency contract. Keep release-please in
charge of versions, notes, signed-compatible PR updates, tags and releases.

**Non-Goals:** Package signing/catalog upload (#459), deployment (#104), changing
runtime SDK behavior, global standalone source gates or automatic ruleset edits.

## Decisions

### 1. Extend the existing workflow with a pinned release-please driver

Use the 17.6.0 programmatic API in locked tooling for preparation and guarded
publication. Keep the existing broker-bound workflow path and push-main identity.
Preview and CI execute the same driver in read-only mode. Test the extension
against that exact package; do not silently upgrade it.

The driver freezes candidate identity: repository, component, version, tag, PR,
head, base, merge SHA and tree. It validates each candidate separately and only
passes approved immutable candidates into release-please's publication operation.
A second unrestricted `createReleases()` discovery is prohibited. Test that a
candidate arriving during validation cannot enter the publication batch.

Retain release-please's lifecycle behavior and immutable-tag checks. The adapter
is an authorization boundary, not another version calculator. Stock action-only
publication is rejected because it cannot enforce this per-candidate boundary.

### 2. Validate before merging and again before tag creation

PR readiness retains live canonical metadata and exact current-base merge checks.
Compare the complete generated footprint, not just filenames: exactly one
component manifest version, its changelog, and Gateway's version-only package edit.
Verify App identity with immutable IDs resolved during implementation, not login
alone. Validate generated release metadata against the manifest, including
post-merge title/body edits.

Before publication, validate the actual merged revision in an isolated checkout
using trusted tooling from the approved workflow revision. Confirm ancestry and
the merged tree's equivalence to the approved candidate. Require successful
applicable source checks from the expected workflow/App at that revision; missing,
skipped, cancelled or failed results cannot authorize tags.

Run library readiness again against the release merge parent, not today's moving
main. Newer unrelated merges do not invalidate an already-authorized immutable
release. Strict up-to-date protection governs the PR merge boundary; post-merge
validation guards publication even if an administrative bypass occurred.

### 3. Complete library freshness and consumer evidence

Reuse `scripts/module-policy.sh standalone <module>` with clean public-proxy
caches, `GOWORK=off`, readonly manifests and no replacements. Resolve only the
selected library's graph. Verify canonical origin and merged ancestry of every
internal prerequisite and require tagged versions.

Keep the conservative owned-source snapshot rule, including dependency manifests.
Replace blanket extension/directory exclusions where an owned embedded asset
would be missed: enumerate development exclusions and explicitly include embedded
runtime inputs. Test renames, deletion, nested ownership and asset changes.

Add selected consumer harnesses that do not inherit the all-provider integration
server's graph. Exercise root frontend interoperability and provider/middleware
consumer behavior with the candidate as the module under test and prerequisites
resolved from the proxy. The external-consumer harness uses a private file proxy containing only the
candidate module archive. It asserts the cache origin and exact selected graph,
imports public packages, and runs explicit existing provider/middleware behavioral
tests from the archived package in that consumer graph. Root reuses the simple-text
scenario and parses/assembles its SSE with the registered frontend baseline. No
public tag or local replacement is used.

### 4. Attribute Gateway intent through a release-please plugin

Use `ManifestPlugin.preconfigure` to supply relevant original commits to Gateway's
commit list before normal release-please parsing and version calculation. Determine
linked library roots from the selected Gateway workspace/build graph, with an
explicit tested ownership mapping. Preserve commit SHAs, subjects and breaking
metadata; deduplicate commits already touching Gateway.

Collect linked commits from Gateway's own last-release boundary, not each library's
boundary. The library may have released more recently or much less recently;
neither changes which code Gateway has shipped. Test bootstrap, no-tag components,
dependency-only fixes, non-releasing commits and reverted changes.

Do not use `linked-versions`: its 17.6.0 implementation synchronizes component
versions and can synthesize forced-version commits, contrary to independence.
Do not manufacture dependency bumps or rely on commit scope to change path
attribution.

### 5. Reuse the plugin validation contract

After #428 merges, reconcile its workflow and spec updates before editing them.
Gateway readiness consumes the exact candidate revision's frontend, application,
two-architecture package, licensing, validator, Grafana E2E and test host evidence.
Its `package.json` version must match the manifest; generated plugin metadata
comes from the build. No Gateway standalone prerequisite is introduced.

Application tags trigger the same revision-selected plugin pipeline. #459 must
publish the validated package and retain source-revision evidence. This change
does not authorize uploading or deploying packages.

### 6. Prepare independent Renovate progression

Place a narrow first-party library rule after existing generic rules so it
overrides disabled version updates, grouping, scheduling and release age.
Allow alpha tags, keep `fix(deps)` intent, and preserve the signed App path.
Use per-dependent-module updates so unrelated downstream readiness cannot deadlock
an intermediate repin. Keep Gateway maintenance distinct. Activate these rules
together with reviewed release automation.

## Risks / Trade-offs

- Conservative freshness can require releases for behavior-neutral source changes
  → prefer over-blocking to silently publishing stale behavior; document exclusions.
- Custom release-please hooks add maintenance → lock the package and test actual
  preparation/publication APIs with mock GitHub interactions.
- Remote metadata can change during a run → freeze candidates, compare authoritative
  merged files, and recheck target tags immediately before publication.
- #428 may change before merge → rebase its integration tasks on the merged SHA;
  do not copy stale plugin jobs into the release workflow.
- Proxy outages block releases → retry the same candidate; never bypass the public
  dependency validation or move an existing tag.

## Migration Plan

1. Land and validate the implementation with activation guards retained.
2. Integrate merged #428 and reconcile image-era specs and runbook language.
3. Verify broker credentials and signed-compatible App PR creation in an approved
   controlled activation exercise; never log credentials.
4. Present the exact ruleset diff for maintainer approval: require readiness and
   applicable plugin checks, require current-base validation, preserve source gates,
   and enforce squash-only history. Apply no protection changes without approval.
5. Inventory existing tags and pending merged release PRs; explicitly validate or
   exclude historical candidates. Preserve the adoption boundary until first
   releases establish component history.
6. Review and approve activation, then enable main-push preparation/publication and
   the Renovate progression rules. Verify root-first and independent Gateway paths.
7. On failure, disable automation in a reviewed change and retry immutable
   candidates. Correct published mistakes through new versions; never retag.

## Open Questions

- App authentication is resolved by PR #465’s successful smoke run. Signed PR
  creation and write operations still need the later activation exercise.
- What commit will #428 merge at? Its final CI identities must be used in the
  approval record and activation tests.

## Library slice verification (2026-10-20)

- The underlying `release-check` commands pass: Go release configuration tests and
  24 Python tests, including real Git candidate merges and real selected-module
  builds with deliberately broken OpenAI/Bedrock source. Fixture Git commits use
  local unsigned test identity; no tags or releases are created.
- The complete `verify_library(".", HEAD, roots)` path passes: clean-cache public
  dependency resolution, selected standalone build/tests, private candidate proxy,
  exact consumer graph assertion, and pinned frontend SSE parsing/assembly.
- Logger's archived consumer and generate/logging regression pass with 11 pinned
  dependencies. This consumer diagnostic does not waive its tagged-prerequisite
  or freshness gate.
- Real Bedrock readiness rejects the dotted prerelease pseudo-version root pin.
  OpenAI's isolated consumer fails because the published root lacks
  `internal/jsonschema`; Grafana's fails on missing/changed provider data fields.
  These are expected downstream readiness failures, not root release blockers.
  Provider scenario registrations are checked against existing tests; successful
  execution of every provider's consumer still requires fresh published pins.
- Workflow actionlint, structural docs lint, Markdown lint, baseline validation,
  and strict OpenSpec validation pass. The Vercel reference remains
  `eb77f09e3c06c28e860d92e0de941b143c2eecec` / `ai@7.0.127`; release-please metadata
  and changelog behavior were compared with 17.6.0 at
  `712fcf01effd08d7b0e7b1fd3861f2cb388bc8d1`.
- App authentication evidence is the successful PR #465 smoke run:
  <https://github.com/grafana/ai-sdk/actions/runs/39470835455>. No new authentication
  exercise, release activation, repository-protection edit or Gateway integration
  was performed. Live App release-PR writes and publication authorization remain
  unproven; current-base checks still need required-check/protection enforcement.
