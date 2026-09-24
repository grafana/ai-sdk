## ADDED Requirements

### Requirement: Source response consumption

The independent Go client SHALL decode registered URL and document sources in unary and streaming responses without importing Gateway code. Required document title SHALL accept an empty string. Optional URL title and document filename absence and empty string SHALL normalize to empty Go strings. Public source identity, order and object-valued provider metadata SHALL survive. Missing required fields, malformed types and unknown source discriminators SHALL use the existing bounded protocol-error path. Unary source Title SHALL be populated, with Text retained for compatibility with older consumers.

#### Scenario: URL and document consumption
- **WHEN** both registered variants arrive through bounded unary or SSE readers
- **THEN** the source content SHALL retain the appropriate variant fields, identity and metadata
- **AND** a missing document title SHALL fail while an explicitly empty title SHALL succeed
