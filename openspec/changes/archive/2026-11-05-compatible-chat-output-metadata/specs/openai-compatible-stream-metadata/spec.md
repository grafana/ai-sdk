## ADDED Requirements

### Requirement: Opt-in Chat output metadata
The provider SHALL offer constructor option `WithChatCompletionMetadata()` to
retain validated first-choice refusal and logprobs under `chatCompletion` in the
resolved provider metadata namespace. Client request options SHALL NOT activate
this extension or be rewritten to enable it. Native usage SHALL remain in
`Usage.Raw`. This opt-in Go extension exceeds registered compatible normalization.

#### Scenario: Unary refusal with probabilities
- **WHEN** the model enables preservation and the first choice contains refusal and logprobs
- **THEN** provider metadata SHALL retain both alongside existing namespace fields
- **AND** empty logprob objects and unrelated fields SHALL be omitted

#### Scenario: Streaming metadata
- **WHEN** an enabled stream sends refusal fragments and logprobs across chunks
- **THEN** the finish part SHALL contain concatenated refusal and appended logprob channels
- **AND** no custom content part SHALL be emitted or stored in message history

#### Scenario: Malformed optional metadata
- **WHEN** first-choice metadata has an invalid type, probability or byte value
- **THEN** the extension for that response or chunk SHALL be dropped with a unary warning or non-retryable recoverable stream error
- **AND** valid content and subsequent chunks SHALL continue to be processed

#### Scenario: Unused data does not invalidate metadata
- **WHEN** non-first choices contain malformed metadata or content has invalid UTF-8 bytes
- **THEN** first-choice metadata SHALL remain available with ordinary Go JSON decoding

#### Scenario: Default provider parity
- **WHEN** preservation is not enabled in the constructor
- **THEN** no Chat metadata extension SHALL be emitted and provider options SHALL retain existing passthrough behavior
