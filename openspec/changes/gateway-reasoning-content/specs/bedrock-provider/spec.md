## ADDED Requirements

### Requirement: Redacted reasoning continuation

The provider SHALL preserve native redactedContent as reasoning metadata under both amazonBedrock and bedrock. Streaming fragments SHALL accumulate per native block and be published as the complete opaque value on reasoning-end. Assistant replay SHALL select signature, then redactedContent, then legacy redactedData, preserving present empty raw values and signed whitespace. Existing public typed string options SHALL remain compatible.

#### Scenario: Fragmented opaque continuation
- **WHEN** one native reasoning block emits multiple redactedContent fragments
- **THEN** its final metadata SHALL contain their concatenation, not merely the last fragment
- **AND** replay SHALL reconstruct the native redactedContent value
