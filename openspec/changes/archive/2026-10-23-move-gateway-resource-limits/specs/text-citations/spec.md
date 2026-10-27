## MODIFIED Requirements

### Requirement: Gateway citation projection uses shared output bounds

Gateway unary text and text-end events and the independent Go client SHALL preserve citations. Gateway projection SHALL reject unrepresentable sources, invalid UTF-8 and invalid offsets without truncation. Citation size SHALL be bounded only by the shared outbound transport under gateway-resource-policy, and the handler SHALL NOT add a citation-specific resource limit or operator capture field.

#### Scenario: Both clients consume bounded citations
- **WHEN** the authenticated handler returns valid citations in unary and streaming modes
- **THEN** the pinned TypeScript client and independent Go client SHALL receive equivalent citations and existing strict output schemas SHALL accept every event

#### Scenario: Citation output cannot be represented
- **WHEN** citation offsets are negative or unsafe integers or source fields are invalid
- **THEN** the handler SHALL fail its existing output validation rather than omit citations or return partial citation JSON

#### Scenario: Large citations are returned whole
- **WHEN** a citation's cited text or source metadata is large
- **THEN** the handler SHALL return it complete in unary and streaming modes
