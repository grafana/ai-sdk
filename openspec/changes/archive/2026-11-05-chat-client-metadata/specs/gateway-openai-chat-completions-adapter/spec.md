## ADDED Requirements

### Requirement: Native Chat response fields
The adapter SHALL project typed compatible-provider refusal and logprobs from the
resolved metadata namespace into Chat fields, and usage details from `Usage.Raw`.
The shared catalog SHALL enable preservation at construction for Chat and ProviderWire
without a client flag. Refusals SHALL bypass structured JSON validation.

#### Scenario: Structured-output refusal
- **WHEN** a compatible backend refuses a structured-output request
- **THEN** unary and streaming clients receive refusal without a codec JSON error

#### Scenario: Detailed token accounting
- **WHEN** native output reports audio, prediction or cache-write token counters
- **THEN** standard Chat usage details preserve them including explicit zero

#### Scenario: Accumulated native metadata
- **WHEN** the compatible provider completes a native stream segment
- **THEN** the adapter SHALL emit accumulated refusal and logprobs once before the public finish
- **AND** ProviderWire SHALL carry them through its existing finish metadata without custom parts

#### Scenario: Malformed optional Chat metadata
- **WHEN** the compatible provider reports ErrInvalidChatCompletionMetadata
- **THEN** Chat SHALL expose a diagnostic warning and continue ordinary content and later chunks
- **AND** unrelated provider errors SHALL retain their existing failure behavior
