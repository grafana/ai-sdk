# gateway-structured-output Specification

## Purpose
Define presence-aware JSON/schema request forwarding, native text transport and client-owned output validation through the Gateway.
## Requirements
### Requirement: Presence-aware structured request
The provider contract and both Gateway clients SHALL distinguish absent, empty and nonempty names and normalize absent/empty descriptions to empty. JSON mode and opaque object schemas SHALL reach native adapters in unary and streaming calls. Explicit text format SHALL retain existing behavior. The Gateway SHALL preserve numeric and nested schema values without rewriting them.

#### Scenario: Name and description presence
- **WHEN** either client sends absent, empty or nonempty name and absent or empty description in either execution mode
- **THEN** the native adapter SHALL receive the corresponding name presence and normalized description
- **AND** a native name default SHALL apply only when name is absent

#### Scenario: Output helper presence
- **WHEN** output helpers receive WithName("") or no name option
- **THEN** their response format SHALL distinguish explicit empty from absence

#### Scenario: Opaque object schema
- **WHEN** schema contains unknown keywords, nested arrays, null, false, Unicode and an integer above 2^53
- **THEN** the Gateway SHALL preserve those JSON values semantically and native response-schema serialization SHALL preserve numeric values without accidental float64 rounding, while retaining intentional registered native schema transformations

#### Scenario: Native numeric fidelity and strict decoding
- **WHEN** Go or raw HTTP submits a response schema containing 9007199254740993 to supported native OpenAI, Anthropic or Bedrock schema conversion
- **THEN** the native request SHALL preserve that exact integer, and malformed or trailing JSON SHALL remain rejected
- **AND** TypeScript evidence SHALL compare its actual submitted values without claiming arbitrary unsafe-integer representation

### Requirement: Existing native and client output semantics
The Gateway SHALL transport structured output using existing text response and stream codecs and preserve supported native warnings, metadata, usage and finish behavior. Each native adapter SHALL retain its registered schema conversion, validation and support behavior. High-level clients SHALL own final object parsing and validation.

#### Scenario: Unary and streamed object
- **WHEN** registered TypeScript and Go high-level output clients consume valid JSON text through unary or streaming generation
- **THEN** they SHALL produce the expected validated object without a new Gateway response codec

#### Scenario: Invalid generated object
- **WHEN** native text is not valid for the requested client schema
- **THEN** final output validation SHALL fail in the client without Gateway repair or a fabricated provider success

### Requirement: Safe malformed requests and native failures
Malformed JSON and invalid response-format field/root shapes SHALL fail before provider invocation under existing request bounds. Schema object contents SHALL remain opaque; provider-invalid schema and provider errors SHALL follow existing safe error handling, retaining supported useful candidate-local native error fields. Raw response bodies, request headers and wrapper prose SHALL NOT be copied; provider-origin messages may echo caller or provider data.

#### Scenario: Invalid schema root
- **WHEN** schema is null, a scalar, a boolean or an array, or name/description has an invalid type
- **THEN** the request SHALL fail validation without invoking a provider

#### Scenario: Native rejection
- **WHEN** a native provider rejects an otherwise accepted schema object
- **THEN** its useful supported error information SHALL reach both clients through existing error conversion

### Requirement: Mapped fallback and independent observation
Structured output SHALL be eligible on configured fallback routes under existing native namespace interpretation, selection, cancellation and commitment rules. Selected failures SHALL identify the actual candidate and exhaustion SHALL retain available bounded candidate failure information through compact execution metadata in existing provider metadata/error-data carriers. Operator and consumer capture SHALL remain independently configured and SHALL NOT change admission or caller responses.

#### Scenario: Native secondary succeeds
- **WHEN** the primary fails eligibly before commitment and a native secondary accepts the structured request
- **THEN** either client in either mode SHALL receive the secondary output with schema/name and matching native options preserved

#### Scenario: Commitment and failure
- **WHEN** a candidate emits a first stream part, encounters a noneligible failure, or the context is cancelled
- **THEN** existing no-replay, eligibility and bounded cleanup rules SHALL apply without structured-output exceptions

#### Scenario: Exhaustion and capture
- **WHEN** all candidates fail or capture is independently enabled on either side
- **THEN** scoped tests SHALL prove actual failure attribution and bounds, one logical observation, useful supported consumer capture, and unchanged caller output/error behavior

#### Scenario: Optional attribution fits existing transport
- **WHEN** a complete observed execution overview fits existing document/frame limits and the default Go client error-envelope limit for HTTP failures
- **THEN** it SHALL identify actual ordered configured candidates without aggregate causes or fallback-intent serialization
- **AND** incomplete or oversized enrichment SHALL be omitted without changing the original response or retry classification

#### Scenario: Native metadata collision and selected adaptation failure
- **WHEN** native metadata contains the gateway namespace and enrichment fits
- **THEN** the original namespace SHALL be preserved under gateway.nativeMetadata
- **AND** a local adaptation error after selection SHALL identify the selected execution without fabricating a native error summary or replaying the request

#### Scenario: Cancellation after an owned candidate failure
- **WHEN** request cancellation replaces an already observed native failure during fallback selection
- **THEN** the outcome SHALL remain canceled and optional diagnostics SHALL summarize the owned source failure without substituting another candidate or replaying the request
