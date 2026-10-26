# Controlled release App PR exercise

This is the next #245 test after #465 (token reads), #466 (library readiness),
and #467 (read-only proposals). It opens **one root release PR** using the
brokered App token and waits for its source CI and library-readiness jobs.
It does not merge the PR, create tags, publish releases, or enable automation.
Track implementation and live evidence in #468.

## Review before enabling

The `app-pr-test` job in the existing release workflow defaults to skipped.
It requires repository variable `RELEASE_APP_PR_TEST=root`, canonical `main`,
and a `push` event. The broker authorizes this workflow/branch/event combination;
manual dispatch does not run the exercise. The normal `release-please` job stays
disabled and retains `skip-github-release: true`.

Merge the readiness and preview prerequisites before this implementation reaches
`main`. Review the generated root preview and resolve its parser diagnostics.
The known historical warnings are in squash commits for #341 and #168. The pinned
parser discards those full messages despite valid Conventional Commit subjects.
For each merged PR, a maintainer can append the supported override to its body,
preserving the rest of the description:

For #341:

```text
BEGIN_COMMIT_OVERRIDE
fix(deps): update npm non-major dependencies
END_COMMIT_OVERRIDE
```

For #168:

```text
BEGIN_COMMIT_OVERRIDE
chore(deps): update npm devdependencies
END_COMMIT_OVERRIDE
```

Run the root preview again and confirm those warnings are gone and the notes
match the intended dependency changes. This implementation does not edit those
PRs, rewrite history, or silently substitute commit titles. Any remaining
unparsable commit blocks the write exercise.

## Run once after approval

1. Obtain approval for the App to create the root test PR. Before the approved
   workflow-changing push to `main`, set `RELEASE_APP_PR_TEST` to `root` in the
   repository's Actions variables. The workflow's path filter remains limited
   to `.github/workflows/release-please.yml`.
2. Inspect the resulting Release workflow run. The job installs the locked
   tooling with lifecycle scripts disabled and runs its tests before minting
   an App token. No local user credential is used for writes.
3. The job checks that the workflow SHA is still canonical `main`, no root
   release branch exists, exactly one root proposal is generated, and only
   root's changelog/manifest are changed. It freezes those contents before
   calling release-please's existing PR writer.
4. The write adapter only permits creation of the root branch and its frozen
   tree/commit/PR/labels. It rejects tag/release endpoints, other branches and
   custom commit author/signature fields. The commit must be verified before
   the branch advances; the branch update uses `force: false`.
5. The verifier uses the ordinary workflow token with read-only permissions.
   It checks the immutable bot/repository identities, signed head commit,
   expected parent and file footprint. It waits up to 30 minutes for both
   `ci.yml`'s `ci` job and `release-readiness.yml`'s `release-readiness` job to
   succeed in PR-triggered runs at that exact head. Skipped jobs are not success.
6. Record the workflow, PR and commit links in #468. Unset the test variable
   after the exercise. Leave the root PR unmerged until publication governance
   and required checks are approved under #245.

GitHub documents that [bot signature verification](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification#signature-verification-for-bots)
requires App-authenticated requests without custom author, committer or signature
information. The pinned release-please writer uses that form. The live exercise,
not local mocks, establishes that this App receives a verified signature.

## Failures and reruns

A failed run may leave an unmerged root branch or PR. The exercise refuses to
reuse or overwrite an existing root branch. Inspect the partial result before
any cleanup; do not delete a branch or PR just to turn the test green.
An unsigned commit is rejected before the release branch advances.

If creation succeeded but CI was still running at the timeout, verify the same
PR without another App write:

```bash
EXPECTED_BASE_SHA=<original-workflow-sha> GH_TOKEN="$(gh auth token)" \
  node release/tooling/verify-app-pr.mjs <test-pr-number>
```

Use a token with repository Actions/contents/PR read access. Verification rejects
an advanced `main`, a changed head, failed checks or missing evidence. A newer
base needs fresh readiness evidence and a separately reviewed recovery; this
one-shot exercise is deliberately not an automatic release-PR update loop.

This tests PR preparation only. Exact merged-candidate publication authorization,
immutable-tag recovery, dependency progression, Gateway integration and protection
migration remain #245 work. Passing this exercise does not authorize publication.
