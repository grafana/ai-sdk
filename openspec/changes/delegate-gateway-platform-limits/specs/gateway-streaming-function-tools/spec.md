## MODIFIED Requirements

### Requirement: Bounded stateless tool correlation and metadata

History-derived state SHALL be bounded by the platform request size, current-stream state by the request deadline and the stream's idle duration, and neither SHALL persist between requests. Tool metadata SHALL retain opaque namespace objects and omitted/empty presence under gateway-provider-metadata; semantic ownership validation SHALL NOT project away unknown fields.

#### Scenario: Bounded stateless tool correlation and metadata
- **WHEN** a continuation carries unresolved history and the new stream supplies a result with an unknown metadata namespace
- **THEN** history and current-stream state SHALL remain request-scoped and nonpersistent, and the namespace SHALL survive without ownership-based projection

