## MODIFIED Requirements

### Requirement: Synthetic terminal adapter errors

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, not classified as unsupported families. Ordinary generated files, sources, custom/raw output, approvals and unsupported tool behavior SHALL remain unsupported. Failure SHALL not synthesize block closures or a finish, nor emit after terminal authority or writer failure.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten
