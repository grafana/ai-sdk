## MODIFIED Requirements

### Requirement: Focused pinned-client response consumption probes

The workspace SHALL include the smallest locally authored black-box probes needed to demonstrate that the registered Gateway client consumes unary JSON success, JSON SSE events ending at clean EOF without requiring `[DONE]`, and a represented safe non-2xx response. Expected non-2xx behavior SHALL derive from the source-equivalent installed Gateway failed-response and error-normalization path together with the represented HTTP status, while the assertion SHALL execute through the public installed client.

#### Scenario: Unary JSON is consumed
- **WHEN** the local probe returns a minimal valid unary LanguageModelV4 JSON result
- **THEN** the registered client SHALL complete the unary call with the expected value

#### Scenario: SSE clean EOF is consumed
- **WHEN** the local probe emits complete JSON stream parts as SSE `data:` events and closes the body without `[DONE]`
- **THEN** the registered client SHALL consume the parts and finish cleanly

#### Scenario: Non-2xx error is consumed
- **WHEN** the local probe returns the represented non-2xx status and safe JSON error input
- **THEN** the registered client SHALL expose the behavior expected from its source-equivalent installed error path through a black-box client call

#### Scenario: Probe claim remains bounded
- **WHEN** all response probes pass
- **THEN** the parity map SHALL still classify exhaustive response arms, strict server errors, privacy, and lifecycle behavior as uncovered by the pinned request-evidence workspace

### Requirement: Non-mutating check workflow

The repository SHALL expose `mise run check-providerwire-v4` as the complete ordinary ProviderWire V4 evidence verification workflow. It SHALL validate exact pins, committed source-equivalence evidence against installed package inputs, classification exhaustiveness, strict parsing, runtime observations, semantic captures, schemas, and response probes without repeating the upstream source-tree comparison, writing committed artifacts, or changing the worktree. Positive Go provider request-contract assertions SHALL run through normal Go test workflows rather than a completed handoff document or test-name manifest.

#### Scenario: Check succeeds without changes
- **WHEN** committed artifacts match the registered packages and all validation passes
- **THEN** `mise run check-providerwire-v4` SHALL succeed and leave no worktree changes

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

`test/conformance/PARITY.md` SHALL classify the ProviderWire V4 request-contract surface by layer and confidence source. It SHALL distinguish automated pinned-client request/schema evidence, positive public Go provider request-contract coverage, smoke-level client response consumption, unchanged tolerant legacy interop, parent-pinned legacy request compatibility, and absent production strict-runtime coverage.

#### Scenario: Reviewer inspects provider request-contract coverage
- **WHEN** a reviewer reads the parity map after implementation
- **THEN** the map SHALL identify the preserved numeric, scalar-presence, and file-data distinctions as automated positive coverage
- **AND** it SHALL not describe the current Go contract as exhibiting the resolved representation losses

#### Scenario: Historical and runtime claims remain bounded
- **WHEN** the parity map describes pinned evidence and current Go coverage
- **THEN** it SHALL retain the original evidence boundary
- **AND** it SHALL identify remaining strict runtime gaps without treating local probes or legacy goldens as provider recordings

## REMOVED Requirements

### Requirement: Evidence-backed Phase 2 delta table

**Reason**: The table was a temporary handoff from evidence gathering into the provider-model implementation. Retaining a resolved ledger duplicates archived change history and durable Go regression coverage.

**Migration**: Preserve the loss rationale and handoff in the archived `providerwire-v4-contract-evidence` change, retain the retired row-level table in repository history, keep current behavior in `provider/request_contract_external_test.go`, and retain strict-codec responsibilities in canonical request-contract specifications.
