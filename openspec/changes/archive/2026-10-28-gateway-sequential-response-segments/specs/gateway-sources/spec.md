## MODIFIED Requirements

### Requirement: Atomic source lifecycle and observation
Sources SHALL preserve relative output order and SHALL NOT close active text or tool blocks. A source SHALL NOT prevent a later response-metadata part once no block is open, and SHALL reset idle activity through the existing loop. Provider error parts SHALL remain non-terminal; a source after a finish without intervening response-metadata SHALL fail safely. Sources SHALL commit fallback candidates without replay. Existing cancellation, timeouts and cleanup ownership SHALL remain effective.

#### Scenario: Source interleaving
- **WHEN** a source appears between text-start and text-end
- **THEN** the text block SHALL remain active and subsequent text deltas SHALL be accepted
- **AND** a response-metadata part before that text-end SHALL fail safely

#### Scenario: Source-only observation
- **WHEN** a source is the only payload before finish
- **THEN** reusable observers SHALL record first-output timing without recording its content in metadata-only output

#### Scenario: Metadata after a source
- **WHEN** response-metadata follows a source with no block open
- **THEN** the metadata SHALL be written and open a new response segment

#### Scenario: Source after a finish
- **WHEN** a source follows a valid finish without intervening response-metadata
- **THEN** the source SHALL NOT be written, the finish SHALL remain written and one synthetic terminal internal error SHALL follow it
