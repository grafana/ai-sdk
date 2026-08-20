# providerwire-v4-contract-evidence Specification

## Purpose

Define exact-pinned evidence, validation, maintenance, and handoff requirements for the ProviderWire V4 language-model contract.

## Requirements

### Requirement: Dedicated exact-pinned evidence workspace
The repository SHALL provide a dedicated `test/providerwire-v4` TypeScript workspace whose `ai` and `@ai-sdk/*` dependencies use exact versions from `test/conformance/upstream.yaml`. The workspace SHALL describe its claim boundary and SHALL remain distinct from provider recordings, upstream provider fixtures, legacy ProviderWire interop, and production runtime tests.

#### Scenario: Workspace versions match baseline
- **WHEN** baseline validation runs
- **THEN** every tracked AI SDK dependency in the workspace SHALL exactly match the registered version

#### Scenario: Evidence provenance is stated
- **WHEN** a contributor inspects the workspace
- **THEN** its documentation SHALL identify request captures as deterministic pinned-client evidence
- **AND** it SHALL state that response probes are locally authored client-consumption inputs rather than provider recordings or server-output oracles

### Requirement: Registered package source equivalence
Before repository source or tests are used to support a compatibility claim, the evidence workflow SHALL establish that the relevant installed registered package sources are equivalent to the registered upstream commit. A manually maintained, reviewable source closure SHALL include provider LanguageModelV4 request declarations and referenced unions, the Gateway language-model serializer and configuration path, provider-utils request/header/response helpers, and the Gateway failed-response and error-normalization path used by the non-2xx probe. Exact package pins SHALL remain the broad package guard; unrelated package source files need not be hashed. The committed equivalence evidence SHALL identify the installed package inputs it covers. Ordinary checks SHALL validate that evidence against the exact installed inputs without repeating the upstream source-tree comparison; the parity-upgrade workflow SHALL refresh the comparison. If equivalence cannot be established, verification SHALL fail with the unresolved package and source surface rather than silently substituting another version.

#### Scenario: Source equivalence is established
- **WHEN** installed package source and matching upstream-commit source are compared for a covered request or error-consumption surface
- **THEN** the workflow SHALL record sufficient evidence to identify them as equivalent for that surface

#### Scenario: Source equivalence fails
- **WHEN** a relevant installed package source cannot be found or differs from the registered commit
- **THEN** the ProviderWire V4 evidence workflow SHALL fail before making a source-backed parity claim

### Requirement: Authoritative request-surface classification
The workspace SHALL contain one canonical typed TypeScript coverage map covering every relevant finite call-options key, finite request-object key, prompt/tool/file/result/approval discriminator, referenced request union, and Gateway serializer transform in the registered packages. Each item SHALL map to a named scenario and semantic JSON pointer with an exact expected value or presence requirement, or to an explicit exclusion with rationale. TypeScript `satisfies` constraints SHALL reject pinned key and discriminator drift. Any reviewer-facing JSON classification SHALL be generated from this map and SHALL NOT be maintained independently.

#### Scenario: Supported item maps to evidence
- **WHEN** a classified item is supported by the registered Gateway request serializer
- **THEN** the classification SHALL name at least one scenario and semantic path where that item is expected

#### Scenario: Excluded item has rationale
- **WHEN** an upstream item does not belong to the Gateway language-model request surface or cannot be emitted by the registered client
- **THEN** the classification SHALL mark it excluded and explain why

#### Scenario: New key is unclassified
- **WHEN** a pinned declaration gains a finite request key not represented by the classification
- **THEN** typechecking or classification verification SHALL fail

#### Scenario: New discriminator is unclassified
- **WHEN** a pinned request union gains a discriminator not represented by the classification
- **THEN** typechecking or classification verification SHALL fail

#### Scenario: Classification artifact is regenerated
- **WHEN** the explicit artifact update workflow runs
- **THEN** the reviewer-facing classification SHALL be generated from the typed coverage map
- **AND** the generic evidence validator SHALL consume that same map

### Requirement: Deterministic Gateway request recorder
The workspace SHALL invoke the ordinary registered Gateway language-model client against a local recording server for direct unary, direct streaming, and multi-step tool-flow scenarios. The recorder SHALL preserve request count and order and SHALL use deterministic model IDs, tool IDs, values, and server behavior.

#### Scenario: Unary request is recorded
- **WHEN** the direct unary scenario runs
- **THEN** the recorder SHALL capture exactly the expected request sequence and unary envelope

#### Scenario: Streaming request is recorded
- **WHEN** the direct streaming scenario runs
- **THEN** the recorder SHALL capture exactly the expected request sequence and streaming envelope

#### Scenario: Multi-step flow is recorded
- **WHEN** the pinned client continues after a tool interaction
- **THEN** the recorder SHALL capture every model request in execution order with deterministic continuation content

### Requirement: Strict raw-body validation precedes normalization
Every emitted raw request body SHALL pass strict JSON syntax validation before semantic parsing or normalization. Strict validation SHALL reject duplicate object members, comments, trailing commas, invalid number syntax, and trailing non-whitespace data. Raw-body validation SHALL remain separate from JSON Schema validation.

#### Scenario: Valid raw request proceeds
- **WHEN** the pinned client emits a valid raw JSON body
- **THEN** strict syntax validation SHALL complete before the body is converted to a semantic JSON value

#### Scenario: Duplicate object member is rejected
- **WHEN** a raw request body contains the same object member more than once
- **THEN** strict syntax validation SHALL fail before semantic normalization can collapse the duplicate

#### Scenario: Invalid syntax is rejected
- **WHEN** a raw request body contains comments, a trailing comma, an invalid number, or trailing data
- **THEN** strict syntax validation SHALL fail

#### Scenario: Semantic capture does not prove syntax rejection
- **WHEN** a normalized committed capture validates against the request schema
- **THEN** documentation and parity claims SHALL NOT cite that capture alone as evidence that malformed raw syntax is rejected

### Requirement: Stable semantic request captures
The workspace SHALL commit stable semantic captures containing method, path relative to the configured `baseURL`, final Fetch-emitted content type, protocol headers, classified behavior-affecting outer headers, strict-envelope classification, request sequence, and decoded JSON body for each distinct behavior group. Comparison SHALL ignore JSON object member order and insignificant formatting but SHALL preserve array order, absence, null, empty strings and collections, zero, false, union selection, non-exempt header values, intermediate exact-key replacement, pre-Fetch lower-case last-value outcomes, reserved-collision classification, and request order. Client-owned authentication, user-agent, and observability header values MAY be normalized explicitly but SHALL be identified in the capture policy.

#### Scenario: Regeneration is deterministic
- **WHEN** artifacts are regenerated twice with the same registered dependencies
- **THEN** the committed semantic captures SHALL be identical

#### Scenario: Presence drift is detected
- **WHEN** a regenerated request adds, removes, nulls, or empties a presence-sensitive member
- **THEN** artifact comparison SHALL report a semantic difference

#### Scenario: Array ordering drift is detected
- **WHEN** a regenerated request reorders prompt parts, tools, examples, stop sequences, or request sequence
- **THEN** artifact comparison SHALL fail

#### Scenario: Object formatting does not create drift
- **WHEN** only JSON object member order or insignificant whitespace differs
- **THEN** semantic comparison SHALL treat the requests as equivalent

#### Scenario: Call headers are captured in body and outer request
- **WHEN** a scenario supplies a non-empty call-level `options.headers` value
- **THEN** the committed evidence SHALL assert the value in both the semantic JSON body and normalized outer headers

#### Scenario: Exact-key composition drift is detected
- **WHEN** configured, call-level, protocol, or observability headers using the same exact property key produce a different replacement result than the installed client
- **THEN** semantic request comparison SHALL fail

#### Scenario: Case-variant normalization drift is detected
- **WHEN** case-variant header properties produce a different lower-case name or last-value result before Fetch than the installed client
- **THEN** semantic request comparison SHALL fail

#### Scenario: Reserved collision is classified as unsupported
- **WHEN** an exact or case-variant content-type collision produces an invalid final reserved header
- **THEN** the evidence SHALL preserve the emitted behavior and classify it as an explicit strict-envelope exclusion

### Requirement: Runtime observation coverage
For every supported classification entry designated for capture, verification SHALL assert that its expected field, discriminator, value, or presence state actually occurs in at least one named capture. A scenario name without an observed path SHALL not count as coverage.

#### Scenario: Designated field is absent from capture
- **WHEN** a classified supported field names a scenario but its expected semantic path is not observed
- **THEN** verification SHALL fail

#### Scenario: Designated discriminator is observed
- **WHEN** each classified union arm is exercised
- **THEN** verification SHALL confirm the expected discriminator and arm-specific members in its designated capture

### Requirement: Request contract validation
Every supported semantic capture SHALL satisfy the ProviderWire V4 HTTP envelope rules and normative request JSON Schema. Explicit collision-exclusion evidence SHALL preserve the pinned client's emitted request while asserting that strict envelope validation rejects its final reserved headers. The workspace SHALL also include focused schema-negative cases for unknown members, unknown discriminators, inactive-arm fields, invalid role/content combinations, and invalid nullability.

#### Scenario: Complete supported capture corpus validates
- **WHEN** the ProviderWire V4 check runs
- **THEN** every supported request envelope and body SHALL validate

#### Scenario: Reserved collision exclusion fails envelope validation
- **WHEN** committed exclusion evidence contains a non-JSON final content type
- **THEN** strict envelope validation SHALL reject it while preserving the observed request as pinned-client exclusion evidence

#### Scenario: Invalid union fixture fails schema validation
- **WHEN** a focused negative case combines multiple union arms or uses an unknown discriminator
- **THEN** schema validation SHALL reject it

#### Scenario: Invalid nullability fixture fails schema validation
- **WHEN** a focused negative case supplies null for a standardized non-null field
- **THEN** schema validation SHALL reject it

### Requirement: Evidence-backed Phase 2 delta table
The workspace SHALL compare the complete classified request surface with the current transport-neutral Go provider request model and SHALL commit a Phase 2 delta table. Every row SHALL identify a valid pinned distinction that an explicit V4 mapper cannot recover from the Go values, its supporting scenario and path, current semantic representation behavior, an external-package executable witness, and the required provider-model change. Generic `encoding/json` tags and output SHALL NOT be treated as protocol authority. Items that differ only because of `omitempty`, JSON formatting, invalid inputs, or permissive flat unions while remaining semantically representable SHALL NOT be classified as losses.

#### Scenario: Lost distinction has a witness
- **WHEN** the current Go model cannot preserve a valid classified request distinction
- **THEN** the delta table SHALL contain one row and one passing Phase 1 witness that demonstrates the loss

#### Scenario: No unsupported redesign is proposed
- **WHEN** a Go type could preserve every valid classified value despite permitting invalid inactive-arm combinations, using nil versus non-nil collections, or requiring explicit discriminator-based wire mapping
- **THEN** the delta table SHALL NOT require a provider-model redesign solely for permissiveness or generic JSON omission

#### Scenario: Delta coherence is evaluated
- **WHEN** the complete delta table is ready for handoff
- **THEN** it SHALL state whether the required changes form one coherent Phase 2 provider-contract change
- **AND** later phase planning SHALL be revised before handoff if they do not

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
- **THEN** the parity map SHALL still classify exhaustive response arms, strict server errors, privacy, and lifecycle behavior as uncovered by Phase 1

### Requirement: Non-mutating check workflow
The repository SHALL expose `mise run check-providerwire-v4` as the complete ordinary ProviderWire V4 verification workflow. It SHALL validate exact pins, the committed source-equivalence evidence against installed package inputs, classification exhaustiveness, strict parsing, runtime observations, semantic captures, schemas, Go loss witnesses, and response probes without repeating the upstream source-tree comparison, writing committed artifacts, or changing the worktree.

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

### Requirement: Explicit artifact update workflow
The repository SHALL expose `mise run update-providerwire-v4-artifacts` as the only public workflow that regenerates committed ProviderWire V4 request evidence. It SHALL rewrite the generated semantic artifacts deterministically and then run the complete non-mutating check.

#### Scenario: Artifacts are updated
- **WHEN** a contributor intentionally runs `mise run update-providerwire-v4-artifacts`
- **THEN** generated request artifacts SHALL be rewritten from the registered client
- **AND** the complete ProviderWire V4 check SHALL run afterward

#### Scenario: No custom rollback mechanism
- **WHEN** generated changes are rejected during review
- **THEN** contributors SHALL use version control to restore them rather than a custom artifact transaction system

### Requirement: Truthful parity coverage classification
`test/conformance/PARITY.md` SHALL classify the new ProviderWire V4 request-contract surface by layer and confidence source. It SHALL distinguish automated pinned-client request/schema evidence, Phase 2 Go-model loss evidence, smoke-level client response consumption, unchanged legacy interop, and absent production strict-runtime coverage.

#### Scenario: Reviewer inspects Phase 1 coverage
- **WHEN** a reviewer reads the parity map
- **THEN** the map SHALL state exactly which request and client-consumption behaviors are automated
- **AND** it SHALL identify the remaining provider-contract and runtime gaps without treating local probes as provider recordings
