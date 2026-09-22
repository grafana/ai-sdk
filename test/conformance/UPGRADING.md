# Parity upgrade tooling and evidence

Use the [parity-upgrade skill](../../.agents/skills/ai-sdk-parity-upgrade/SKILL.md)
for analysis, classification, implementation planning and review. This reference
explains how the repository's tools support those decisions. It is not a sequence
of commands that automatically establishes full parity.

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

After the assessment establishes the implementation scope:

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

Applying a new target clears `upstream.verifiedAt`; selection is not verification.
The final metadata check deliberately remains incomplete until evidence is reviewed
and the actual verification date is recorded. The Gateway witness can likewise fail
until its mapping/differential evidence has been reviewed and refreshed. Neither
failure is an exemption from testing or permission to merge.

## Establish merge readiness

Every PR must pass its required checks independently. If new behavior conflicts
with old expectations, the necessary implementation, target pins, lockfile,
expectations and attestation belong in the same baseline transition. Do not weaken
comparisons or defer that PR's failing checks to a later PR.

Check public-module dependencies before deciding boundaries. A provider may require
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

## Report the result

Keep the assessment, decisions and concise proof with the change. Raw logs, probes,
full investigation diffs and candidate manifest dumps belong outside the repository;
the small selected-target record is a useful reproducible input.

Report baseline verification separately from completion of the capabilities selected
for implementation. Missing implementation and missing evidence remain distinct.
An accepted follow-up needs explicit scope and an owner; an unresolved decision is
not an accepted gap. Existing failing assertions cannot be converted to gaps to
make a transition green.

A new baseline can be assessed while earlier feature work remains, but affected
work must be re-evaluated against the new comparison. Automation scheduling,
permissions and execution setup belong to automation configuration, not this
parity-analysis workflow.
