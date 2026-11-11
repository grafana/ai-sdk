## MODIFIED Requirements

### Requirement: Streamed custom content events
The stream SHALL encode each provider custom part as one atomic `{type: "custom", kind, providerMetadata?}` event in provider order. Custom events SHALL be valid within a response segment before its finish, SHALL NOT prevent a later response-metadata part, and SHALL NOT open, close or alter text, reasoning or tool state.

#### Scenario: Custom between active blocks
- **WHEN** a custom part arrives while a text block and a tool-input block are open
- **THEN** it SHALL be emitted in order and both blocks SHALL remain open and closable by their existing rules

#### Scenario: Response metadata after custom
- **WHEN** response metadata arrives after a custom event with no text, reasoning or tool-input block open
- **THEN** the metadata SHALL be written and open a new response segment, and a later valid finish SHALL be written

#### Scenario: Custom after a finish
- **WHEN** a custom part follows a valid finish without intervening response-metadata
- **THEN** the handler SHALL emit at most one safe terminal error after the finish and SHALL NOT write the custom event

#### Scenario: Invalid custom stream part
- **WHEN** a custom part has an invalid kind or invalid metadata
- **THEN** the handler SHALL emit at most one safe terminal error without serializing the provider-domain part

#### Scenario: Large custom event
- **WHEN** a valid custom part encodes to a frame larger than the former 1 MiB limit
- **THEN** the complete custom event SHALL be written and the stream SHALL continue
