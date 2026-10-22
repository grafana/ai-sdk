# Merged library authorization

Publication remains disabled. The next authorization stage consumes evidence for
one exact merged library candidate; it does not create tags or GitHub Releases.

## Retained PR readiness evidence

A successful library readiness check writes `library-readiness.json` after its
final live metadata/base recheck. Ordinary source PRs emit no record. Validation
removes any stale output first, so a failed or ordinary run cannot reuse an old
record. The workflow uploads only after success, using the artifact name
`library-readiness-<run-id>-<attempt>` and 90-day retention.

Schema version 1 records canonical repository identity, PR number, component and
version, release head, tested base, test-merge SHA and complete tree SHA. It also
records SHA-256 digests of the exact UTF-8 PR title/body, the base tooling revision,
readiness workflow blob, and GitHub run ID/attempt. It includes no credentials.
The tooling revision is the base because the validated release footprint permits
only the component changelog and manifest to differ from it.

An artifact is not a publication token. Later authorization must obtain it from
the successful canonical workflow/run/attempt and verify its contents against Git
objects, live PR metadata and the actual squash tree. Missing or expired evidence
blocks authorization; old console output or a locally edited JSON file does not
substitute. Existing PRs need fresh readiness evidence before merging under this
contract. Previously merged candidates require an explicit recovery disposition.

## Read-only merged-candidate report

From a clean trusted checkout with the locked dependencies installed:

```bash
npm ci --prefix release/tooling --ignore-scripts
GH_TOKEN="$(gh auth token)" node release/tooling/authorize.mjs . > /tmp/root-authorization.json
```

Omit `.` to inventory all configured components. The credential needs repository,
Actions and PR read access. The API transport permits only reads; artifact download
uses a separate bounded GET from a GitHub-issued storage URL without forwarding the
API credential. No token is passed to candidate build/test processes.
GitHub API reads request connection closure: synchronous library validation can
otherwise leave a server-closed pooled socket pending in the client event loop,
causing the final metadata reread to fail. This adds no retries or write access.

Discovery uses release-please **17.6.0** `Manifest.buildReleases()` with a frozen,
fully paginated merged-PR iterator. It does not inherit the upstream 200-PR scan
limit or call `createReleases()`. Main/config changes during discovery abort the
run without an approved subset. Candidates arriving afterward require another run.
Labels aid discovery but do not authorize a release. Duplicate component/tag claims
are blocked. The report includes `publicationEnabled: false`, the trusted tooling
revision and one approved/blocked result per frozen candidate. Blocked selections
produce exit code 1 without discarding other candidates' results; an empty inventory
succeeds with no candidates. Unknown components and incomplete discovery fail.

For each library the authorizer requires:

1. Canonical repository and immutable release App identity, a verified signed
   release head, and a merged non-draft PR with one squash parent.
2. A successful readiness record bound to that exact head, base, tree, title and
   body. The squash parent must equal the tested base; its entire tree must equal
   the reconstruction of that base and release head. Versions, tags and generated
   changelog content must agree. Only the manifest and selected changelog may change.
3. A completed canonical `ci.yml` main-push run at the squash SHA, with successful
   `ci`, `race`, `docs-lint`, `module-resolution`, `parity-baseline`,
   `integration-test` and `conformance-test` jobs. A later SHA or a cancelled run
   cannot substitute. Failed image/publication jobs do not replace these source
   results. The PR and squash SHAs are different Git objects.
4. Fresh standalone and consumer validation of the selected merged library, using
   trusted tooling and its published prerequisites. Prerequisite ancestry and
   owned-source freshness are compared with the immutable squash parent. Root's
   validation does not invoke unrelated providers' standalone checks.
5. Final rereads of live PR metadata, source/readiness runs and attempts, artifact
   identity, target-tag/release absence and canonical ancestry. Later unrelated
   main commits are allowed; altered candidate/evidence or tags are not.

The run and job metadata fields were checked against canonical PR #469 runs
`39817055944` and `39817055990`: job `head_sha` identifies the PR head, and closed
PR runs can have an empty `pull_requests` array. Authorization binds the retained
PR number to canonical PR metadata and the authenticated run's exact head instead
of relying on that mutable array. GitHub Actions check-suite App ID is `15368`.

Validation uses a detached temporary checkout and removes it afterward. The
candidate's copies of policy, consumer harnesses and authorization scripts are not
executed as the trusted validator. Candidate library code and its tests do execute;
run this command only in a suitable build environment for reviewed repository code.
The restricted child environment excludes tokens and Python/Node/Git injection
variables. Normal prerequisite downloads still use the public Go proxy.

## Failures and evidence limits

Missing/expired artifacts, skipped or failed required jobs, proxy outages, unsigned
heads, edited release metadata and unsupported merge history block the affected
candidate. Retrying is read-only. If a tag or GitHub Release already exists, even
at the expected revision, this command blocks. The disabled [publication integration](PUBLICATION.md) has
a separate matching-state recovery path.
Gateway is explicitly unsupported. Historical candidates without retained evidence
are not automatically grandfathered into authorization.

An approved result is not standing permission to publish. The publisher must
revalidate and use the same frozen candidate; a handwritten report, a later rescan
or the stock unrestricted publication action cannot replace that boundary. The separate publication command defaults off and requires explicit activation. #245 and #468 remain open.

## Regression evidence

`npm test --prefix release/tooling` exercises the pinned discovery builder with
mocked GitHub reads, provenance checks, mutations during validation, bounded artifact
downloads and independent decisions. Python release tests exercise real Git test
merges/squashes, stale evidence, generated footprints and selected graph failures.
They do not claim live App signature or release-publication evidence.

The explicit root consumer integration uses real candidate source with deliberately
broken OpenAI/Bedrock source, fresh proxy caches and the pinned frontend parser:

```bash
RELEASE_AUTHORIZATION_INTEGRATION=1 PYTHONDONTWRITEBYTECODE=1 \
  python3 -m unittest discover -s scripts -p test_release_authorization.py
```

The default suite skips this extra network/build integration; run it before merging
changes to the selected-library execution boundary. Required source CI continues to
run ordinary integration and conformance jobs.
