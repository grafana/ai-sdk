# gateway-execution-overview Specification

## Purpose

Define the Gateway-owned compact execution projector, native failure summaries and best-effort metadata placement under existing protocol limits.

## Requirements

### Requirement: Compact observed execution overview

The Gateway SHALL provide a pure projector for one invocation's ordered observed fallback decisions.

#### Scenario: Compact observed execution overview policy
- **WHEN** an invocation is projected into an optional execution overview
- **THEN** It SHALL include requested/canonical route identity once and each observed candidate's provider/backend model, optional configured instance, outcome and optional failure summary. It SHALL NOT emit unrun configured candidates, a selected index, timestamps, completion/replay claims, fallback-intent flags, raw diagnostic trees or disposition taxonomies. Only the last attempt SHALL be eligible for selected or canceled outcome. Selected SHALL mean accepted unary result or first stream part, not successful completion. Invalid, incomplete or mixed invocation observation SHALL omit the optional overview rather than invent attribution.

#### Scenario: Earlier candidate fails and later candidate is selected
- **WHEN** one invocation reports a failed primary followed by a selected secondary
- **THEN** projection SHALL include those two candidates in order with the primary summary and no duplicated selected index

#### Scenario: Intent does not establish invocation
- **WHEN** a failure permits fallback but no later candidate decision is observed
- **THEN** projection SHALL NOT invent a subsequent attempt or project an incomplete advance as a complete invocation

#### Scenario: Error part selects a stream
- **WHEN** fallback accepts an error part as the first stream part
- **THEN** the overview SHALL report selection without a completion claim or a duplicated stream-error history

### Requirement: Small native failure summaries

Candidate-local projection SHALL retain available native message, type, string-or-number code and native status.

#### Scenario: Small native failure summaries policy
- **WHEN** a candidate-local native failure is summarized
- **THEN** It SHALL follow only a single error chain and SHALL NOT assign an arbitrary aggregate member to a candidate. It SHALL NOT publish original bodies, headers, request data, arbitrary causes or diagnostic details. Native JSON decoding SHALL be standard Go shallow decoding with numeric precision preserved. Pure projection SHALL NOT extract credentials from requests or diagnostic trees. Runtime delivery SHALL redact exact configured API-key matches from summary message, type and string code under gateway-attempt-failure-evidence; other provider-originated values SHALL remain available independently of logger field policy. Inputs SHALL remain unchanged. Malformed or unavailable diagnostics SHALL degrade optional fields without creating an error. Projection SHALL NOT take or apply a source byte limit; native body size is bounded by the outbound transport under gateway-resource-policy.

#### Scenario: Original native failure survives cancellation
- **WHEN** a canceled decision retains a candidate-native SourceErr
- **THEN** projection SHALL use that source for its native summary while retaining canceled outcome without altering the returned context error

#### Scenario: Useful native message has an endpoint and cause
- **WHEN** an API error contains a safe native message plus URL/cause information
- **THEN** the message SHALL remain available without serializing the URL/cause or suppressing it merely because those fields exist

#### Scenario: Provider echoes a credential in a summary
- **WHEN** a provider-originated message, type or string code contains a credential value
- **THEN** runtime delivery SHALL redact exact configured API-key matches and SHALL preserve caller-owned BYOK values that do not match configured keys, without mutating source errors; account authorization and request isolation SHALL remain unchanged

#### Scenario: Numeric code exceeds float precision
- **WHEN** a native error supplies a large numeric code
- **THEN** projection SHALL retain its numeric value without float conversion

#### Scenario: Large native error body
- **WHEN** a native error body or message exceeds the former 8 MiB source limit but is within the outbound bound
- **THEN** projection SHALL still return its message, type, code and status

### Requirement: Dormant foundation and owned schema

The Gateway SHALL define and test a compact namespace schema independently of production service/handler activation, without a new stream reader or retry/cleanup owner, a runtime schema dependency or an Apache client import. The implementation SHALL remove the old Gateway collector, model wrappers and diagnostic allocation machinery. Root SDK observation SHALL remain reusable without Gateway dependencies. Minimum Go versions SHALL remain unchanged.

#### Scenario: Only the foundation is applied
- **WHEN** this change is applied without the activation change
- **THEN** existing runtime output SHALL remain unchanged while focused Go and strict TypeScript schema tests exercise the new projection

### Requirement: Unconditional metadata placement
Enrichment SHALL place the overview under `gateway.execution` and always relocate a native `gateway` value, complete, under `gateway.nativeMetadata`, without merging it into trusted facts. Other namespaces SHALL remain opaque. Enrichment SHALL have no size limit and SHALL be absent only when no overview exists or it cannot be encoded. Namespace presence SHALL NOT establish provenance, routing authority or absence of attempts.

#### Scenario: Native namespace collides
- **WHEN** native metadata already has a `gateway` value and an overview is attached
- **THEN** the complete native value SHALL appear under `nativeMetadata` while execution facts remain separate and inputs remain unchanged

#### Scenario: Large relocated namespace
- **WHEN** the native `gateway` value or the overview is larger than the former unary or frame budget
- **THEN** relocation SHALL still happen and the complete enriched output SHALL be written

#### Scenario: Enrichment cannot be encoded
- **WHEN** optional overview assembly contains malformed data
- **THEN** enrichment SHALL return no metadata, without truncation or a new error, and existing content, error or finish behavior SHALL remain unchanged
