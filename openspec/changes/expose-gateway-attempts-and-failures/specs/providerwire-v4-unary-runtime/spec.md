## ADDED Requirements

### Requirement: Optional unary execution overview

The handler SHALL use synchronized request-local observation to optionally enrich a valid unary result or classified invocation failure with a compact execution overview. Original primary validation and existing complete-response limits SHALL apply before accepting enrichment. Optional encoding/fit failure SHALL preserve the original document and classification; original adaptation failures SHALL NOT replay generation. Protected route sources SHALL remain private, and configured direct calls SHALL retain their existing invocation/cancellation ownership.

#### Scenario: Valid result with no enrichment room
- **WHEN** a native result fits but adding or relocating the overview exceeds the existing response bound
- **THEN** the original successful result SHALL be written without metadata loss or a diagnostic-induced failure

#### Scenario: Owned cancellation and late native result
- **WHEN** cancellation wins invocation ownership before a late native result
- **THEN** the existing context error SHALL be returned and published attribution SHALL NOT capture the unowned late native failure
