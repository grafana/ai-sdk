## MODIFIED Requirements

### Requirement: Mapped fallback and independent observation
Structured output SHALL be eligible on configured fallback routes under existing native namespace interpretation, selection, cancellation and commitment rules. Selected failures SHALL identify the actual candidate and exhaustion SHALL retain available candidate failure information through compact execution metadata in existing provider metadata/error-data carriers. Operator and consumer capture SHALL remain independently configured and SHALL NOT change admission or caller responses.

#### Scenario: Native secondary succeeds
- **WHEN** the primary fails eligibly before commitment and a native secondary accepts the structured request
- **THEN** either client in either mode SHALL receive the secondary output with schema/name and matching native options preserved

#### Scenario: Commitment and failure
- **WHEN** a candidate emits a first stream part, encounters a noneligible failure, or the context is cancelled
- **THEN** existing no-replay, eligibility and bounded cleanup rules SHALL apply without structured-output exceptions

#### Scenario: Exhaustion and capture
- **WHEN** all candidates fail or capture is independently enabled on either side
- **THEN** scoped tests SHALL prove actual failure attribution, one logical observation, useful supported consumer capture, and unchanged caller output/error behavior

#### Scenario: Optional attribution fits existing transport
- **WHEN** a complete observed execution overview exists for a success or HTTP failure, including one above the former document, frame or Go client error-envelope limits
- **THEN** it SHALL identify actual ordered configured candidates without aggregate causes or fallback-intent serialization, regardless of its encoded size
- **AND** incomplete enrichment SHALL be omitted without changing the original response or retry classification

#### Scenario: Native metadata collision and selected adaptation failure
- **WHEN** native metadata contains the gateway namespace and an overview is attached
- **THEN** the original namespace SHALL be preserved under gateway.nativeMetadata
- **AND** a local adaptation error after selection SHALL identify the selected execution without fabricating a native error summary or replaying the request

#### Scenario: Cancellation after an owned candidate failure
- **WHEN** request cancellation replaces an already observed native failure during fallback selection
- **THEN** the outcome SHALL remain canceled and optional diagnostics SHALL summarize the owned source failure without substituting another candidate or replaying the request
