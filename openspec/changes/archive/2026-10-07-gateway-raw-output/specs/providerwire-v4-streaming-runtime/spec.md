## MODIFIED Requirements

### Requirement: Synthetic terminal adapter errors

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, and URL/document sources SHALL be supported under gateway-sources. Raw parts SHALL follow gateway-raw-output. Ordinary generated files, custom output, approvals and unsupported tool behavior SHALL remain unsupported. Failure SHALL not synthesize block closures or a finish, nor emit after terminal authority or writer failure.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, custom, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten
