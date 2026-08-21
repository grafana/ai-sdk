## MODIFIED Requirements

### Requirement: Non-mutating check workflow

The repository SHALL expose `mise run check-providerwire-v4` as the complete ordinary ProviderWire V4 verification workflow. It SHALL validate exact pins, committed source-equivalence evidence against installed package inputs, classification exhaustiveness, strict parsing, runtime observations, semantic captures, schemas, response probes, focused strict-runtime Go tests, and the exact-pinned direct runtime scenario without repeating the upstream source-tree comparison, writing committed artifacts, or changing the worktree. Positive Go provider request-contract assertions SHALL run through normal Go test workflows rather than a completed handoff document or test-name manifest. Strict-runtime tests SHALL preserve the distinction between local schema/encoder authority and pinned-client consumption evidence.

#### Scenario: Check succeeds without changes
- **WHEN** committed artifacts match the registered packages and all evidence and focused runtime validation passes
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

#### Scenario: Strict runtime regresses
- **WHEN** focused Go validation, response schema or golden validation, or the direct exact-pinned unary or streaming scenario fails
- **THEN** the ProviderWire V4 check SHALL fail without rewriting request evidence or runtime artifacts

### Requirement: Truthful parity coverage classification

`test/conformance/PARITY.md` SHALL classify the ProviderWire V4 surface by layer and confidence source. It SHALL distinguish automated pinned-client request/schema evidence, positive public Go provider request-contract coverage, smoke-level client response consumption, focused strict-runtime schemas and Go tests, direct exact-pinned runtime consumption, unchanged tolerant legacy interop, parent-pinned legacy request compatibility, and strict-runtime behavior still deferred beyond the Phase 3 text vertical.

#### Scenario: Reviewer inspects provider request-contract coverage
- **WHEN** a reviewer reads the parity map after implementation
- **THEN** the map SHALL identify the preserved numeric, scalar-presence, and file-data distinctions as automated positive coverage
- **AND** it SHALL not describe the current Go contract as exhibiting the resolved representation losses

#### Scenario: Reviewer inspects strict runtime coverage
- **WHEN** a reviewer reads the strict ProviderWire V4 runtime row
- **THEN** the map SHALL identify strict envelope, syntax, schema, text-subset mapping, safe errors, bounded unary text, minimal text SSE, catalog resolution, and exact-pinned direct consumption as the automated Phase 3 scope
- **AND** it SHALL distinguish raw HTTP and local schema/golden authority for unary canonical identity and empty warnings from the pinned client's observable unary content, finish reason, usage, and consumption
- **AND** it SHALL retain explicit gaps for full request adaptation, complete response and stream families, raw disclosure, privacy hardening, and comprehensive lifecycle hardening

#### Scenario: Historical and runtime claims remain bounded
- **WHEN** the parity map describes pinned evidence and current Go coverage
- **THEN** it SHALL retain the original request-evidence and response-smoke boundaries
- **AND** it SHALL not treat direct pinned-client consumption, local schemas, legacy goldens, or deterministic models as provider recordings or an exhaustive private-server oracle
