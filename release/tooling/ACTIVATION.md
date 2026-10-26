# Library activation and completion

This is the final library activation rollout. Gateway image/plugin delivery is
outside this workflow and does not block libraries. Do not close #245 or #468 until
the live evidence below is recorded and any remaining provider work is explicitly
accounted for.

## Review and merge order

The transport prerequisite #492 is merged. PR #493 combines publication integration
and this final activation rollout; the separate #494 draft was consolidated into it.
Review the settings below and merge #493 only after its required checks pass against
main. Do not bypass checks.

The activation PR introduces `RELEASE_LIBRARY_MODE`:

| Value | Effect on the next canonical main push |
| --- | --- |
| Absent or any other value | No release job or writes |
| `root` | Only merged root #490 may publish; no new release PRs |
| `libraries` | Publish verified pending libraries, then prepare library release PRs |

No mode includes Gateway. The CLI repeats the mode, repository, event and branch
checks. Manual dispatch is not supported by the App broker. The workflow's path
and non-cancelling concurrency group stay the same. Every main push can run it;
there is no workflow-file path filter. The retired smoke and one-shot App PR jobs
are removed from the workflow; their prior successful evidence remains valid.

## Settings to approve before activation

Read the current settings and preserve unrelated requirements. These are proposed
changes, not settings applied by this PR:

1. In the main ruleset, require up-to-date branches and retain existing required
   checks. Add GitHub Actions checks `release-readiness`, `race`, and `check`.
   Required source checks are `ci`, `race`, `docs-lint`, `module-resolution`,
   `parity-baseline`, `integration-test`, and `conformance-test`. Keep
   `Validate Changes` and `license/cla` as well. Actions checks must bind to App
   ID `15368`. Require at least one approving review.
2. Retain squash-only repository merging and `PR_TITLE` / `PR_BODY` squash defaults.
   The current repository already has these defaults. Align the main ruleset's
   merge-method list to squash as part of the same settings review.
3. Add an active tag ruleset targeting **all tags**, with no exclusions or bypass
   actors, restricting **updates** and **deletions**. Do not restrict creation:
   the App needs to create new tags. Existing tags remain immutable for everyone.
   After inspecting the complete bypass list with your maintainer credential, set
   `RELEASE_TAG_RULESET` to JSON containing that ruleset’s numeric `id` and exact
   `updatedAt` (the API `updated_at` value), for example
   `{"id": 123, "updatedAt": "2026-10-22T00:00:00Z"}`. Use real values from the
   reviewed ruleset, not this example.
4. Confirm the release App remains installed only on `grafana/ai-sdk`. Existing
   contents/issues/pull-request write permissions are sufficient. Actions evidence
   uses the separate read-only workflow token; no new App Actions permission or
   broker binding is needed.

The CLI reads repository merge settings with the existing release App token through
a read-only transport; the workflow token can omit those fields. Effective main
rules, tag rules and Actions evidence use the read-only workflow token. Missing or
incorrect merge settings block publication; no new token permissions are required.
The CLI reads effective main rules and the approved tag ruleset revision and
refuses before writes if its observable settings or revision differ. GitHub hides
`bypass_actors` from read-only callers, so the maintainer approval records the
no-bypass inspection; omission is not evidence of an empty list. If the API does
return bypass actors, any nonempty list is rejected. A changed ruleset requires
fresh review and an updated approval variable. It rereads protections before publication and
preparation mutations. Read/API failures block the run. No script changes rulesets.
The [GitHub rules API](https://docs.github.com/en/rest/repos/rules) defines the
inspection endpoints and rule fields.

## Root bootstrap

After approval, apply the reviewed settings and set `RELEASE_LIBRARY_MODE=root`
before merging this activation PR. Its main push is the bootstrap trigger. If it
is merged while the variable is absent, a later ordinary reviewed main push can
trigger the same code after approval; no additional implementation PR is needed.

The bootstrap candidate is root #490, squash
`c8e497f3ca45571ef57c3556ebde32a836e8b918`, tag `v0.1.0-alpha.2`. A newer root
candidate is rejected in root mode. Readiness artifact `12454303002` from run
`39844872856` attempt 1 and exact-squash CI run `39846721747` are historical
identifiers to inspect, not authorization tokens. The driver rediscovers and
revalidates canonical evidence and selected root source on every run.

The workflow waits up to 20 minutes for successful source CI at its own tooling
SHA, then checks the candidate's separate exact-squash evidence. It installs locked
tooling before minting the App token and gives no API credentials to candidate
build/test processes. Publication uses the existing bounded writer and recovery
contract in [PUBLICATION.md](PUBLICATION.md).

Record these acceptance results in #468:

- Release run, trusted tooling SHA, candidate PR/head/squash and readiness artifact.
- Canonical tag resolves exactly to the squash, and the GitHub Release has the
  generated notes, correct version, and prerelease flag.
- PR #490 has `autorelease: tagged` and no `autorelease: pending`.
- A fresh external module cache resolves the public tag through the Go proxy;
  `go mod download -json github.com/grafana/ai-sdk@v0.1.0-alpha.2` reports the expected
  canonical origin hash. Build a consumer using that public version without local
  replacements or a workspace. The local candidate proxy test is not this proof.
- A subsequent approved main push in root mode performs no duplicate publication.

If artifacts expire or canonical state differs, stop and inspect. Do not substitute
old local reports, recreate evidence, move a tag, or force a release through.

## Library progression

After root proof passes, approve switching the variable to `libraries`. The next
main push publishes only independently ready pending libraries and prepares one
release PR per eligible component. A merged pending component suppresses only its
own next preparation. Late publication candidates wait for another run. Preparation
requires that main still equals the workflow SHA and uses App-signed commits.
Existing release PRs refresh even when their notes are unchanged, so current-base
readiness can run again. Commit-parser diagnostics stop preparation writes.

Preparation uses release-please 17.6.0 with the temporary `last-release-sha` override
removed in memory; actual published releases delimit later notes, while
`bootstrap-sha` remains the boundary for never-published modules. The committed
release config stays unchanged so #490's retained config identity remains valid.
The historical PR header describes publication as disabled; this activation runbook
and the live mode are authoritative until that header is updated after bootstrap.

The final first-party rule in `renovate.json` becomes effective when this PR merges.
It enables independent updates only for registered library modules, accepts tagged
alpha/beta/rc versions, excludes pseudo-version proposals, and overrides inherited
weekly grouping and release-age delay. Updates use `fix(deps)` and require review;
automerge is disabled. This follows Renovate's
[configuration contracts](https://docs.renovatebot.com/configuration-options/).

Review and merge the root requirement update for direct dependents, then repeat
for provider prerequisites and Bedrock. Ordinary source PRs retain candidate-source
CI. A dependency bump does not prove its dependent is release-ready: publication
rechecks tag origin, ancestry, owned-source freshness, standalone and consumer
behavior. Record the root → direct provider → Bedrock progression in #468; if a
provider needs unrelated implementation fixes, track those separately rather than
holding the root release or pretending the progression is proven.

## Stop and recovery

Unset `RELEASE_LIBRARY_MODE` to prevent future jobs. It does not cancel an already
running job or undo completed writes. Inspect any in-flight run before cancellation;
a cancelled or failed publisher may leave a matching tag/release to recover on a
later explicitly enabled run. Never delete or move an existing published tag.

The next run performs fresh validation and only missing operations. Conflicting
state blocks that candidate. A failure for one library does not discard another
library's completed result. A workflow can finish failed after publishing a ready
subset; inspect its per-candidate report before retrying.

Completion means the implementation is merged, settings verified, root publication
and external consumption proven, dependency progression demonstrated or explicitly
tracked, and #245 acceptance criteria reconciled with the library-only scope.
Closing the umbrella still requires maintainer approval.
