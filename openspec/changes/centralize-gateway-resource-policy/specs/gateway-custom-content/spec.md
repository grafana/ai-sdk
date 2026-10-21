# Spec Delta

## MODIFIED Requirements

### Requirement: Unary custom content output
Unary success SHALL encode provider custom content as `{type: "custom", kind, providerMetadata?}` in original content order under explicit private DTOs. The kind SHALL be non-empty valid UTF-8 containing a dot. Metadata SHALL follow gateway-provider-metadata presence and validity rules, without an adapter size budget. Invalid custom output SHALL fail before HTTP 200.

#### Scenario: Custom-only paid response
- **WHEN** a provider returns only `anthropic.fallback` custom content with metadata
- **THEN** both registered clients SHALL consume success with the same kind and metadata, and default high-level retries SHALL NOT invoke the provider again

#### Scenario: Invalid custom output
- **WHEN** a provider returns custom content with an empty, undotted or invalid-UTF-8 kind, or invalid metadata
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200
- **WHEN** valid custom metadata exceeds the former unary budget
- **THEN** the handler SHALL return it unchanged

### Requirement: Streamed custom content events
The stream SHALL encode each provider custom part as one atomic `{type: "custom", kind, providerMetadata?}` event in provider order. Custom events SHALL be valid between stream start and finish, SHALL count as started content so later response metadata is rejected, and SHALL NOT open, close or alter text, reasoning or tool state.

#### Scenario: Custom between active blocks
- **WHEN** a custom part arrives while a text block and a tool-input block are open
- **THEN** it SHALL be emitted in order and both blocks SHALL remain open and closable by their existing rules

#### Scenario: Response metadata after custom
- **WHEN** response metadata arrives after a custom event
- **THEN** the handler SHALL emit at most one safe terminal error and no synthetic finish

#### Scenario: Invalid custom stream part
- **WHEN** a custom part has an invalid kind or invalid metadata
- **THEN** the handler SHALL emit at most one safe terminal error without serializing the provider-domain part
- **WHEN** valid custom metadata exceeds the former complete-frame budget
- **THEN** the complete custom event SHALL be written in order
