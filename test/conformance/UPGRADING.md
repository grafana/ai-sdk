# Parity upgrade tooling and evidence

Use the [parity-upgrade skill](../../.agents/skills/ai-sdk-parity-upgrade/SKILL.md)
for two separate activities: a pinned-version upgrade with a comprehensive parity
assessment, and subsequent implementation of registered parity work packages.
This reference explains commands and evidence, not a shortcut to full parity.

The pinned-version upgrade produces one green PR containing consistent versions,
expectations, verification, necessary compatibility corrections and an accounted-for
assessment of the current Go implementation. Remaining parity work is delivered
separately. The pinned reference is the comparison contract, not a completeness claim.

## Establish a fixed comparison

`upstream.yaml` identifies the registered baseline. Select a coherent mature target
once, or use the target already chosen for the work:

```bash
TARGET=/absolute/path/to/new-target.json mise run parity-select
```

Selection uses the stable npm latest release lines and the minimum release age in
`test/pnpm-workspace.yaml`. It writes exact package versions, publication times,
per-package source commits, the starting baseline and selection policy to a new
record. It does not change canonical pins and refuses to overwrite an existing file.

Preserve that small record with the assessment. New releases alone do not change
the target. A deliberate target change requires a new record, an incremental source
comparison and an updated scope decision. Coherent intermediate checkpoints can
be proposed when a single transition is too large; do not mix arbitrary package
versions or bypass maturity checks.

For source investigation, use exact Git objects, matching installed package sources
or versioned upstream URLs. Useful Git operations include `git log`, `git diff`
and `git show` between the registered and target references. Verify package
manifests: the `ai` tag alone does not establish every package's source version.
Changelogs identify leads; implementation and tests establish semantics.

## Choose evidence for the finding

| Tool / source | What it establishes | Limit |
| --- | --- | --- |
| Exact upstream implementation/tests and Go source | Semantics, execution paths and likely missing behavior | Requires analysis; not executable proof by itself |
| `PARITY.md` | Declared support, existing evidence and known gaps | A coverage map, not an automatically verified feature inventory |
| `parity-select` / `parity-apply` | Fixed versions, maturity, coherence and exact source evidence | Does not decide scope or certify behavior |
| `mise run parity-coverage` | Provider fixture inventory, imports and byte-identical provenance | Does not measure behavioral coverage of every capability |
| `mise run generate-conformance` | Upstream expectations from existing inputs | Generation succeeding does not mean Go matches |
| `mise run test-conformance` | Go request/output equivalence for exercised scenarios | Unrepresented behavior requires other tests or source review |
| `mise run parity-provider-shape` | Selected provider discriminator differences | Not full API/semantic parity; missing values are warnings and unavailable source can be skipped |
| `mise run test-integration` | Covered frontend hook and stream interactions | Lower-level schema acceptance alone does not prove hook lifecycle behavior |
| ProviderWire/Gateway contract and runtime tests | Covered mapping, client consumption, service lifecycle and privacy | Verify the actual dependency versions and runtime exercised |
| `mise run verify-module-resolution` | Tests against published Go dependencies outside the workspace | Does not decide good PR boundaries or replace workspace/integration tests |
| `mise run validate-parity-baseline` | Consumer-pin consistency, verification-date validity and reviewed Gateway witness consistency | Metadata consistency is not semantic review |

Use request snapshots for provider conversion, UI/output snapshots for orchestration,
and hook scenarios for frontend behavior. For unsupported fixture inputs, use focused
unit/integration tests and identify the remaining boundary gap. Never invent recorded
provider input or modify a recording to produce a desired result. Imported upstream
inputs must match their exact source and be indexed; see [fixture guidance](README.md).

A successful `parity-check` aggregates several checks, not an exhaustive feature
assessment. Inspect skipped/unavailable source, warning-only reports and uncovered
paths before making a parity claim.

## Apply a target

For the pinned-version upgrade, apply the fixed target to run candidate checks
and assess the current Go implementation against it:

```bash
TARGET=/absolute/path/to/approved-target.json mise run parity-apply
```

Apply rechecks the starting baseline or an identical already-applied target, age
policy, stable/nondecreasing versions, publication/source evidence, coherence and
all four consumer manifests before writing. It queries exact versions, never latest.
It changes the baseline and conformance/integration/CLI/ProviderWire pins, not the
lockfile or generated expectations.

To apply, install and regenerate expectations together:

```bash
TARGET=/absolute/path/to/approved-target.json mise run parity-upgrade
```

The command requires an explicit target. Reapplication is idempotent and preserves
subsequent gap metadata and an already recorded verification date. A changed or mixed
baseline/pin set is rejected for reassessment rather than silently repaired.

`parity-upgrade` also regenerates the embedded Bedrock Converse
`providers/bedrock/provider_tool_schemas.json` asset from the registered
Anthropic/provider package versions and the Anthropic Zod version in the lockfile
at the pinned upstream commit. It omits tools Bedrock filters out, warns about
new upstream tools for parity assessment, and fails if a Go-supported tool
disappears upstream. Use
`node scripts/generate-bedrock-provider-tool-schemas.mjs --check` to verify the
committed file without rewriting it. This is not a live provider recording.

Applying a new target clears `upstream.verifiedAt`; selection is not verification.
The final metadata check deliberately remains incomplete until evidence is reviewed
and the actual verification date is recorded. The Gateway witness can likewise fail
until its mapping/differential evidence has been reviewed and refreshed. Neither
failure is an exemption from testing or permission to merge.

## Turn work packages into mergeable changes

A parity work package defines behavior, dependencies and acceptance evidence. A PR
is a delivery unit. Several related packages can share a PR; one package may require
separate producer and consumer PRs. Track the complete package outcome across those
changes rather than equating a merged PR with a completed capability.

Keep two dependency decisions separate:

- **Upstream references:** the pinned-version upgrade moves the coherent package
  set, canonical expectations and verification together. Required-check failures
  and target-induced incompatibilities that prevent supported integrations from
  working block that PR, even without existing fixtures. Other assessed differences,
  including older implementation gaps and new capabilities, can become registered
  work packages with explicit dispositions; complete feature parity is not required.
- **Go module requirements:** bump a consumer when it needs an already published
  producer API or behavior. Do not mechanically bump Go modules merely because the
  upstream reference versions changed. A parity correction within the registered
  baseline may need no upstream dependency bump at all.

Every PR must pass its required checks independently. If new behavior conflicts
with old expectations, the necessary implementation, target pins, lockfile,
expectations and attestation belong in the same baseline transition. Do not weaken
comparisons or defer that PR's failing checks to a later PR.

Check public-module dependencies before deciding delivery boundaries. A provider may require
new root fields, or Mantle may require a newer OpenAI adapter. Merge and verify the
producer's published revision before the consumer adopts it. Workspace success can
otherwise hide a dependency failure. Module checks use a fresh public cache,
`GOWORK=off` and readonly tests; local replacements are not publication evidence.

Choose tests from the affected behavior, including conformance, frontend,
ProviderWire/Gateway runtime/privacy and focused module/race tests. Review snapshot
changes, fixture provenance and accepted coverage dispositions; repeat generation
for stability. After recording completed verification, run the full parity and
required CI checks on the merge candidate. The relevant entry points include:

```bash
mise run parity-check
mise run verify-module-resolution
mise run verify-ai-gateway-boundary
mise run test-integration
mise run test-ai-gateway-command
```

These supplement, rather than replace, normal build/test/lint and other required
CI checks. Recheck evidence affected by code or dependency changes. In particular,
a test against an older published adapter cannot prove a new adapter is deployed.

## Register findings and report completion

The upgrade assessment compares current Go behavior with the new reference across
declared supported surfaces, not only the release delta or existing fixtures.
Account for older gaps, unsupported families and inconclusive areas as well as
newly changed behavior.

Keep each durable record focused:

- `upstream-sync` issues are authoritative for actionable deferred work, including
  the assessed upstream reference, Go difference and impact, outcome, decisions,
  dependencies and acceptance tests.
- `PARITY.md` records stable coverage classifications, confidence sources, supported
  boundaries and accepted deviations. Update it only when one of those facts changes.
  Do not add a dated/versioned assessment section, issue catalog, run ledger or copy
  of issue detail. Do not mirror issue state in repository documentation.
- The upgrade PR records its target, corrections, validation and a compact exact
  list of created or reused issues. It may summarize run-specific adaptations,
  exclusions and blockers without turning those notes into a permanent coverage
  section.

Reuse an existing issue rather than creating duplicate queues. Full investigation
notes and raw evidence stay outside Git under the existing artifact policy.

### Issue creation and duplicate checks

Create one issue per coherent, actionable remaining work package, not per upstream
commit, changed file or failing assertion. Do not create issues for already fixed
upgrade work, verified matches or documented adaptations/exclusions with no action.
An investigation issue needs a concrete question, impact and completion criteria.

Before creating an issue:

1. Search open `upstream-sync` issues, then search the whole repository without that
   label restriction by provider, capability, behavior and relevant synonyms. Version
   or commit searches are useful supplements, not sufficient duplicate checks.
2. Read candidate titles and bodies, relevant discussion and linked PRs. Compare
   scope and acceptance criteria, not just wording. Account for truncated search
   results rather than treating the first page as exhaustive.
3. Inspect closed matches: verify whether they were implemented, rejected, marked
   duplicate or superseded. A completed issue is not a live owner for unfinished
   work. Do not reopen or recreate rejected work automatically. A demonstrated new
   regression may warrant a distinct issue explaining and linking the prior fix.
4. Reuse an open issue when its scope and acceptance criteria cover the finding.
   For partial or ambiguous overlap, flag the scope decision rather than creating
   a likely duplicate or rewriting someone else's issue. A clearly separate package
   may have its own issue with an explicit relationship to the existing one.
5. Recheck relevant open results immediately before creation, then link the resulting
   issue identifier from the upgrade PR's deferred-work list. Do not copy its body
   into `PARITY.md`, overwrite unrelated content, post comments or close/reopen issues
   as part of registration.

Use `gh issue list --repo grafana/ai-sdk --state all --search '<query>'` for searches
and `gh issue view <number> --repo grafana/ai-sdk --comments` to inspect candidates.
Use `gh issue create --body-file <path>` to preserve the structured body. Titles
should identify the area and observable behavior, not just a version bump or vague
“parity fixes.” Use this compact body structure:

- **Behavior and impact:** current Go behavior versus intended outcome; distinguish
  missing implementation from missing evidence.
- **Upstream evidence:** assessed package/version, exact source/test links and any
  reproduction or existing regression evidence.
- **Scope and decisions:** included/excluded behavior, API questions, dependencies
  and related issues/PRs. State why any apparent overlap is separate.
- **Acceptance:** concrete behavior/tests that establish completion, plus owner if
  known. Leave assignment pending rather than inventing an assignee.

### Issue labels

**Every new or reused parity work-package issue must carry `upstream-sync`.** For a
reused issue missing it, use
`gh issue edit <number> -R grafana/ai-sdk --add-label upstream-sync`;
preserve existing labels and other metadata. Verify labels after
creation or reuse. Registration is incomplete if the required label cannot be added.

Inspect current labels with `gh label list --repo grafana/ai-sdk` before using
optional labels; do not invent provider/area labels or create labels automatically.
The repository currently provides these relevant choices:

| Label | Use |
| --- | --- |
| `upstream-sync` | Required on every parity work-package issue |
| `bug` | Incorrect existing behavior or a regression |
| `enhancement` | A missing capability or a standalone improvement in coverage |
| `documentation` | A documentation-only work package |
| `question` | A concrete investigation or unresolved design question, not a known defect disguised as uncertainty |

Choose applicable labels based on the work, not the upstream commit type. Do not
infer `good first issue`, `help wanted`, `duplicate`, `invalid`, `wontfix` or
`pir-action-item` automatically. Release, dependency-update, automerge, severity and
other workflow-specific labels are not implied by parity work. Security findings
must follow the repository's private reporting policy rather than being published
as ordinary parity issues merely with a `security` label.

### Completion

The pinned-version PR is complete when its checks pass and the comprehensive
assessment is accounted for, including registered remaining work. A parity package
is complete only when its behavior and proof are delivered; update or close its
issue then, and update the coverage map only if stable coverage, evidence, support
boundaries or accepted deviations changed. A tracked correction is not a permanent
accepted deviation, and a proposed API is not approved merely because it has an
issue. Unresolved upgrade-blocking risks or existing failures cannot be relabeled
as follow-ups.

When pins advance again, reassess open packages against the new reference. Raw
logs, probes, full investigation diffs and candidate manifest dumps stay outside
Git; the small selected-target record remains a reproducible input. Automation
scheduling, permissions and execution setup belong to automation configuration.
