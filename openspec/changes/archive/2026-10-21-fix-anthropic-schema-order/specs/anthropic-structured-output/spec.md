## ADDED Requirements

### Requirement: Anthropic schema member order

Native structured output, JSON-tool fallback and function-tool requests SHALL preserve the caller's ordering of `properties`, `definitions` and `$defs` names, including nested schemas. Native output SHALL retain existing sanitization, including `oneOf` conversion to `anyOf`, while tool schemas SHALL remain unsanitized. Conversion MUST NOT mutate caller JSON or round its numeric tokens.

#### Scenario: Dependent properties retain their order

- **WHEN** a schema lists `reason` before `answer`, including inside array items or composition branches
- **THEN** generation and streaming requests SHALL transmit `reason` before `answer` on all three schema paths
- **AND** native sanitization SHALL still remove unsupported constraints

#### Scenario: Definition order and numeric tokens survive

- **WHEN** a schema lists `Zebra` before `Alpha` in `definitions` or `$defs` and contains a large integer constant
- **THEN** outgoing schemas SHALL retain that name order and the original integer token
- **AND** caller JSON SHALL remain unchanged
