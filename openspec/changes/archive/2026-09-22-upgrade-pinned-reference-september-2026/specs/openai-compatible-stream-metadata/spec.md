## ADDED Requirements

### Requirement: Placeholder chunks do not consume response metadata
The OpenAI-compatible provider SHALL wait for the first chunk containing a
nonempty response ID, model ID or nonzero creation timestamp before emitting
response metadata. It SHALL emit metadata at most once. Zero creation timestamps
SHALL be treated as placeholders in both unary and streaming responses.

#### Scenario: Empty placeholder precedes metadata
- **WHEN** a provider emits an empty-ID/model, zero-time placeholder before a chunk containing real metadata
- **THEN** the response metadata part contains the later ID, model and timestamp
- **AND** no placeholder metadata part is emitted

#### Scenario: Unary timestamp is zero
- **WHEN** a unary response has created equal to zero
- **THEN** the normalized timestamp is absent rather than the Unix epoch
