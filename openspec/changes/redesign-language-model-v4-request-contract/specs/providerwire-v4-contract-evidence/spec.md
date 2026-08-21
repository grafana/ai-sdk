## ADDED Requirements

### Requirement: Phase 2 evidence documentation has an explicit resolved lifecycle

The ProviderWire V4 evidence README, Phase 2 delta table, Go contract tests, `mise.toml`, and parity map SHALL transition together when the provider request losses are resolved. Pinned-client captures, classification, schema, and source-equivalence artifacts SHALL remain evidence of the registered client and SHALL NOT be rewritten merely because the Go provider model changed.

#### Scenario: Historical evidence is retained
- **WHEN** a Phase 1 loss is resolved in Go
- **THEN** its pinned scenario, semantic path, original loss description, and required change SHALL remain reviewable in the delta table
- **AND** the row SHALL gain a resolved status and positive contract-test reference

#### Scenario: Documentation no longer claims an active loss
- **WHEN** all Phase 2 rows are resolved
- **THEN** the README and parity map SHALL describe positive provider-contract coverage rather than current Go losses

#### Scenario: Pinned captures remain unchanged
- **WHEN** only the Go provider contract changes
- **THEN** semantic request captures and request-surface classification SHALL not be regenerated or edited unless the registered client evidence itself changed

## MODIFIED Requirements

### Requirement: Evidence-backed Phase 2 delta table

The workspace SHALL retain a Phase 2 delta table derived from the complete classified request surface and the pre-redesign transport-neutral Go provider request model. Every row SHALL have a stable identifier and SHALL identify the valid pinned distinction, supporting scenario and path, original semantic representation loss, original external-package witness, required provider-model change, resolution status, and positive provider-contract assertion after implementation. Generic `encoding/json` tags and output SHALL NOT be treated as protocol authority. Items differing only because of `omitempty`, JSON formatting, invalid inputs, or permissive flat unions while remaining semantically representable SHALL NOT be classified as losses.

The historical file SHALL remain named `test/providerwire-v4/phase2-delta.md`. Resolving a row SHALL update its status and test reference rather than deleting the evidence or rewriting the pinned capture.

#### Scenario: Lost distinction has a witness
- **WHEN** the parent Go model cannot preserve a valid classified request distinction
- **THEN** the delta table SHALL contain one row and one passing Phase 1 witness demonstrating the loss

#### Scenario: Resolved distinction has a positive assertion
- **WHEN** Phase 2 makes the distinction representable
- **THEN** the same row SHALL be marked resolved and reference a positive external-package provider-contract test

#### Scenario: No unsupported redesign is proposed
- **WHEN** a Go type can preserve every valid classified value despite permitting invalid inactive-arm combinations, using nil versus non-nil collections, or requiring explicit discriminator-based wire mapping
- **THEN** the delta table SHALL NOT require a provider-model redesign solely for permissiveness or generic JSON omission

#### Scenario: Delta coherence is evaluated
- **WHEN** the complete resolved table is inspected
- **THEN** it SHALL preserve the Phase 1 conclusion that the changes formed one coherent provider-contract correction
- **AND** it SHALL show that every row has corresponding positive coverage

### Requirement: Non-mutating check workflow

The repository SHALL expose `mise run check-providerwire-v4` as the complete ordinary ProviderWire V4 verification workflow. It SHALL validate exact pins, committed source-equivalence evidence against installed package inputs, classification exhaustiveness, strict parsing, runtime observations, semantic captures, schemas, resolved positive Go provider-contract assertions, and response probes without repeating the upstream source-tree comparison, writing committed artifacts, or changing the worktree.

Positive contract tests SHALL live in `provider/providerwire_v4_contract_test.go`, use one top-level `TestProviderWireV4Contract_*` test per stable delta-row identifier, and record that exact test name in the resolved table. The non-mutating workflow SHALL parse the resolved row IDs and test names, enumerate actual top-level tests through `go test ./provider -list`, and require exact set equality before running the complete `provider` package tests. A missing, extra, renamed, or deleted contract test SHALL fail even when unrelated provider tests pass. The former `provider/providerwire_v4_loss_test.go` file and `TestProviderWireV4Loss_*` prefix SHALL no longer exist after resolution.

#### Scenario: Check succeeds without changes
- **WHEN** committed artifacts match the registered packages and all validation passes
- **THEN** `mise run check-providerwire-v4` SHALL succeed and leave no worktree changes

#### Scenario: Provider contract assertion set is exact
- **WHEN** the check runs after Phase 2
- **THEN** the resolved delta-row test names and enumerated `TestProviderWireV4Contract_*` names SHALL be equal sets
- **AND** missing, extra, renamed, or deleted contract tests SHALL fail before the full provider package test run

#### Scenario: Provider contract assertions execute
- **WHEN** the name-set check succeeds
- **THEN** the workflow SHALL run the full provider package test suite
- **AND** it SHALL not depend on a potentially empty execution regex

#### Scenario: Generated evidence is stale
- **WHEN** in-memory or temporary regeneration differs from committed evidence
- **THEN** the check SHALL fail with reviewable artifact differences and SHALL NOT rewrite files

#### Scenario: Artifact baseline metadata is stale
- **WHEN** semantic, classification, or source-equivalence metadata differs from `upstream.yaml` or the exact workspace pins
- **THEN** the check SHALL fail without rewriting the stale artifact

#### Scenario: Required verification runs
- **WHEN** aggregate repository parity or required CI verification runs
- **THEN** it SHALL invoke the complete non-mutating ProviderWire V4 check

### Requirement: Truthful parity coverage classification

`test/conformance/PARITY.md` SHALL classify the ProviderWire V4 request-contract surface by layer and confidence source. It SHALL distinguish automated pinned-client request/schema evidence, resolved positive Go provider-contract coverage, smoke-level client response consumption, unchanged tolerant legacy interop, parent-pinned legacy request compatibility, and absent production strict-runtime coverage.

#### Scenario: Reviewer inspects Phase 1 coverage
- **WHEN** a reviewer reads the parity map after implementation
- **THEN** the map SHALL identify every resolved request-model distinction as automated positive coverage
- **AND** it SHALL no longer describe the current Go contract as exhibiting the Phase 1 losses

#### Scenario: Historical and runtime claims remain bounded
- **WHEN** the parity map describes Phase 1 evidence and Phase 2 completion
- **THEN** it SHALL retain the original evidence boundary
- **AND** it SHALL identify remaining strict runtime gaps without treating local probes or legacy goldens as provider recordings
