# gateway-execution-overview Specification

## Purpose

Define the dormant Gateway-owned compact execution projector, protected failure summaries and best-effort metadata placement under existing protocol limits.

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

### Requirement: Small protected failure summaries

Candidate-local projection SHALL retain available safe message, type, string-or-number code and native status.

#### Scenario: Small protected failure summaries policy
- **WHEN** a candidate-local native failure is summarized
- **THEN** It SHALL follow only a single error chain and SHALL NOT assign an arbitrary aggregate member to a candidate. It SHALL NOT publish original bodies, headers, request data, arbitrary causes or diagnostic details. Native JSON decoding SHALL be standard Go shallow decoding with numeric precision preserved. Actual protected sources and known credential/other-tenant scalar fields SHALL protect summary echoes; ordinary endpoints, identifiers and token-looking application strings SHALL remain available. Inputs SHALL remain unchanged. Malformed, unavailable or oversized diagnostics under the integrator's existing source/read limit SHALL degrade optional fields without creating an error or a new quota.

#### Scenario: Original native failure survives cancellation
- **WHEN** a canceled decision retains a candidate-native SourceErr
- **THEN** projection SHALL use that source for its safe summary while retaining canceled outcome without altering the returned context error

#### Scenario: Useful native message has an endpoint and cause
- **WHEN** an API error contains a safe native message plus URL/cause information
- **THEN** the message SHALL remain available without serializing the URL/cause or suppressing it merely because those fields exist

#### Scenario: Protected scalar appears in a summary
- **WHEN** native message, type or code echoes an actual credential or other-tenant scalar
- **THEN** only affected summary fields SHALL be omitted and the original source SHALL remain unchanged

#### Scenario: Numeric code exceeds float precision
- **WHEN** a native error supplies a large numeric code
- **THEN** projection SHALL retain its numeric value without float conversion

### Requirement: Best-effort metadata placement under existing envelope limits

Metadata enrichment SHALL place the overview under gateway.execution and, when fitting, relocate the original native gateway value under gateway.nativeMetadata without merging it into trusted facts.

#### Scenario: Best-effort metadata placement under existing envelope limits policy
- **WHEN** an integrator attempts optional metadata enrichment
- **THEN** Other namespaces SHALL remain opaque. The caller SHALL use the existing complete-response or complete-frame limit, not an overview allocation. Encoding/fit failure SHALL return the original provider metadata without truncation or a new error. When native relocation alone cannot fit, the primary response SHALL be preserved and the native namespace SHALL remain opaque. Namespace presence alone SHALL NOT establish Gateway provenance, routing authority or absence of attempts. Primary protocol encoding failures SHALL retain existing behavior.

#### Scenario: Native namespace collides and enrichment fits
- **WHEN** native metadata already has a gateway value and the enriched complete envelope fits
- **THEN** the complete native value SHALL appear under nativeMetadata while execution facts remain separate and inputs remain unchanged

#### Scenario: Relocation alone cannot fit
- **WHEN** the original complete envelope fits but adding or relocating Gateway metadata does not
- **THEN** enrichment SHALL preserve the original metadata and SHALL NOT fail the successful response or finish event

#### Scenario: Enrichment cannot be encoded
- **WHEN** optional overview assembly contains malformed data
- **THEN** enrichment SHALL return the original metadata without changing existing content, error or finish behavior

### Requirement: Dormant foundation and owned schema

The Gateway SHALL define and test a compact namespace schema without production service/handler activation, a new stream reader or retry/cleanup owner, a runtime schema dependency or an Apache client import. The implementation SHALL remove the old Gateway collector, model wrappers and diagnostic allocation machinery. Root SDK observation SHALL remain reusable without Gateway dependencies. Minimum Go versions SHALL remain unchanged.

#### Scenario: Only the foundation is applied
- **WHEN** this change is applied without the activation change
- **THEN** existing runtime output SHALL remain unchanged while focused Go and strict TypeScript schema tests exercise the new projection
