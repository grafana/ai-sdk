# Spec Delta

## MODIFIED Requirements

### Requirement: Ordinary file inputs map without loss
The strict Gateway mapper SHALL support user/assistant ordinary file parts and file entries in supported tool-role content results for unary and streaming requests. It SHALL preserve ordering, selected data/URL/reference/text arms, required empty payloads, media type, and absent/empty/non-empty filename distinctions using explicit private DTO mapping and public provider constructors. The complete registered schema SHALL remain the wire authority, independent of provider-domain JSON methods.

#### Scenario: Four file arms reach the model
- **WHEN** a focused registered-client request contains the four file arms in supported prompt or tool-result positions
- **THEN** the recording model SHALL receive the same selections, payloads, order, media types, and filename presence in both execution modes

#### Scenario: Empty text and data remain selected
- **WHEN** a supported file carries empty data or empty text and an explicitly empty filename
- **THEN** the model SHALL receive the selected empty arm and a present empty filename, not absent data or filename

#### Scenario: Deferred sibling capabilities remain deferred
- **WHEN** a request also activates approvals or unsupported provider-executed history
- **THEN** the applicable deferred capability SHALL fail safely before model invocation without enabling that family as a side effect of file support
