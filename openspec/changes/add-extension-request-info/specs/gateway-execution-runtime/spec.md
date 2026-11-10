# Spec Delta

## ADDED Requirements

### Requirement: Call facts for composed layers
The selection the runtime passes to its composer SHALL expose the call's requested model ID as sent, its canonical ID (empty for request accounts), a copy of its expected attempts, and its attempts so far, projected and redacted like its overview for a given outcome. That read SHALL NOT seal the capture and SHALL still see attempts observed before sealing. Attempts observed after sealing SHALL stay ignored.

#### Scenario: Read after the adapter sealed
- **WHEN** a fallback stream's adapter projects its overview at the first part and a composed layer reads the attempts when the stream ends
- **THEN** the layer SHALL see the same failed and selected attempts as the overview

#### Scenario: Read does not seal
- **WHEN** a composed layer reads a unary call's attempts before the adapter projects its view
- **THEN** the adapter's overview SHALL be unchanged, and a second view SHALL still project nothing

#### Scenario: Read before entry
- **WHEN** a composed layer refuses a call before invoking the selected model and reads the attempts
- **THEN** it SHALL see none

#### Scenario: Selection outside a composer
- **WHEN** a selector's selection is read before the runtime selects it
- **THEN** it SHALL report no requested ID and no attempts
