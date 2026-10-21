# Spec Delta

## MODIFIED Requirements

### Requirement: Small native failure summaries

Candidate-local projection SHALL retain available native message, type, string-or-number code and native status.

#### Scenario: Small native failure summaries policy
- **WHEN** a candidate-local native failure is summarized
- **THEN** It SHALL follow only a single error chain and SHALL NOT assign an arbitrary aggregate member to a candidate. It SHALL NOT publish original bodies, headers, request data, arbitrary causes or diagnostic details. Native JSON decoding SHALL be standard Go shallow decoding with numeric precision preserved. Pure projection SHALL NOT extract credentials from requests or diagnostic trees. Runtime delivery SHALL redact exact configured API-key matches from summary message, type and string code under gateway-attempt-failure-evidence; other provider-originated values SHALL remain available independently of logger field policy. Inputs SHALL remain unchanged. Projection SHALL NOT take a source/read byte budget: native error bodies are already bounded by the shared outbound transport. Malformed or unavailable diagnostics SHALL degrade optional fields without creating an error or a new quota.

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
- **WHEN** a native error body within the outbound bound is larger than the former unary response budget
- **THEN** projection SHALL decode it and retain its message, type, code and status

## ADDED Requirements

### Requirement: Metadata placement and native namespace relocation
Metadata enrichment SHALL place the overview under gateway.execution and SHALL relocate any original native gateway value under gateway.nativeMetadata without merging it into trusted facts. Placement and relocation SHALL NOT depend on the size of the enclosing response or event. Enrichment SHALL return the original metadata only when the overview cannot be encoded.

#### Scenario: Native namespace collides
- **WHEN** native metadata already has a gateway value and an overview is available
- **THEN** the complete native value SHALL appear under nativeMetadata while execution facts remain separate and inputs remain unchanged

#### Scenario: Large enclosing response
- **WHEN** the enclosing response or finish exceeds the former unary or frame budget
- **THEN** the overview and relocated native value SHALL still be delivered

#### Scenario: Enrichment cannot be encoded
- **WHEN** optional overview assembly contains data that cannot be encoded
- **THEN** enrichment SHALL return the original metadata without changing existing content, error or finish behavior

#### Scenario: Other namespaces and provenance
- **WHEN** enrichment is applied
- **THEN** other namespaces SHALL remain opaque, primary protocol encoding failures SHALL retain existing behavior, and namespace presence alone SHALL NOT establish Gateway provenance, routing authority or absence of attempts

## REMOVED Requirements

### Requirement: Best-effort metadata placement under existing envelope limits
**Reason**: Fit-based enrichment and conditional relocation are replaced by unconditional placement under shared resource policy.
**Migration**: Use "Metadata placement and native namespace relocation".
