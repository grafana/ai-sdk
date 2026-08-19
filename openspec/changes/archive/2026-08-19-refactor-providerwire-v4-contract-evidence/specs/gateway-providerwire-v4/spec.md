## MODIFIED Requirements

### Requirement: Reproducible and privacy-safe interop evidence

A deterministic recorder independent of all Go provider-wire handlers SHALL capture direct `doGenerate` and `doStream` requests and orchestration-level `generateText` and `streamText` requests from the pinned packages. Coverage SHALL include unary and streaming selection, message roles, files, tools, tool execution, structured output, explicit empties, provider options, raw intent, and header precedence.

The fixture index SHALL identify regenerated request captures, curated semantic seeds, mutation recipes, derived response variants, reused provider-independent conformance inputs, and independent UI expectations. It SHALL record their authority, source, direction, update command, normalization, claims, and non-claims without duplicating baseline versions. Authorization values, credentials, volatile identifiers, machine-local paths, and provider-recording claims SHALL not be committed.

Response consumption evidence SHALL keep curated unary JSON and clean EOF-terminated SSE seeds. Tolerated `[DONE]` framing SHALL be derived from the clean stream seed, and safe error projections SHALL be selected from named curated positive error cases. Request-capture scenarios SHALL reuse the same response seeds where their behavior does not require a scenario-specific response.

Selected provider-independent conformance stream inputs SHALL be rendered in memory as ProviderWire SSE and consumed by the pinned Gateway client using each fixture's existing orchestration context. Their assembled UI chunks SHALL compare semantically with the existing pinned TypeScript `expected.jsonl` oracle. This lane SHALL be labeled a derived raw transport projection and SHALL NOT be described as provider recording, private-server behavior, host-policy enforcement, or a Go runtime.

#### Scenario: Recorder is implementation-independent
- **WHEN** request evidence is generated
- **THEN** requests SHALL terminate at the local recorder without importing or starting a Go handler

#### Scenario: Normal verification is non-mutating
- **WHEN** the aggregate ProviderWire V4 check runs, even with an ambient update variable
- **THEN** it SHALL recapture and derive evidence without rewriting committed fixtures

#### Scenario: Mechanical variants have one semantic source
- **WHEN** `[DONE]`, safe errors, schema failures, or envelope variants are tested
- **THEN** each variant SHALL identify its curated seed and deterministic derivation or mutation recipe

#### Scenario: Existing UI output remains the oracle
- **WHEN** a selected provider-independent conformance input is transported through ProviderWire SSE and the pinned Gateway client
- **THEN** the resulting UI chunks SHALL match its existing `expected.jsonl` without changing that oracle

#### Scenario: Ownership is explicit
- **WHEN** a reviewer inspects the fixture index or GitHub diff
- **THEN** regenerated captures, curated seeds, mutation recipes, derived projections, and reused conformance evidence SHALL be distinguishable

#### Scenario: Response projections prove consumption only
- **WHEN** the pinned client consumes a local or derived projection
- **THEN** the result SHALL not be described as Vercel server behavior or a live provider recording

### Requirement: Repeatable validation and coordinated evolution

`mise run validate-providerwire-v4-contract` SHALL validate OpenAPI, offline references, strict syntax, HTTP envelopes, schema compilation, positive payloads, mutation-derived negative failures, response seeds, and selected conformance transport inputs. `mise run test-interop-contract` SHALL validate baseline pins, type-check capture tooling, verify request captures, consume seeded and derived response projections, and compare selected transported conformance inputs with existing UI expectations.

`mise run check-providerwire-v4` SHALL aggregate those non-mutating checks. Committed artifact replacement SHALL require `mise run update-providerwire-v4-artifacts`, which SHALL validate generated content before atomically replacing the request capture.

The parity map SHALL classify V4 contract evidence separately from legacy transport. A baseline change SHALL update the manifest, dependency pins, required source-equivalence evidence, schemas, captures, semantic seeds, recipes, parity classification, and lockfiles together.

#### Scenario: Contract validation is one command
- **WHEN** a contributor runs `mise run validate-providerwire-v4-contract`
- **THEN** every machine-readable contract, curated seed, recipe, and selected transport input SHALL be checked without network access

#### Scenario: Interop verification uses baseline pins
- **WHEN** a contributor runs `mise run test-interop-contract`
- **THEN** baseline validation and all non-mutating client evidence SHALL run

#### Scenario: Aggregate verification does not update artifacts
- **WHEN** a contributor runs `mise run check-providerwire-v4`
- **THEN** contract and interop evidence SHALL be checked without changing committed files

#### Scenario: Artifact refresh is explicit and atomic
- **WHEN** a contributor runs `mise run update-providerwire-v4-artifacts`
- **THEN** generated request evidence SHALL be validated before replacing the committed capture atomically

#### Scenario: Legacy parity remains separate
- **WHEN** the parity coverage map is reviewed
- **THEN** V4 SHALL not claim handler, client, provider, Grafana, frontend runtime, private-server, or live-provider behavior

#### Scenario: Baseline drift is atomic
- **WHEN** a registered package or relied-on serialized behavior changes
- **THEN** all affected machine-readable artifacts and evidence SHALL change in the same parity-governed update
