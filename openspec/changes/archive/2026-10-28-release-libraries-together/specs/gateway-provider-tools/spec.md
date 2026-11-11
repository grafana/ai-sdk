## MODIFIED Requirements

### Requirement: Independent acceptance evidence
Both registered clients SHALL exercise real-handler and authenticated-command scenarios without requiring MCP. Request goldens SHALL be captured from the pinned client. Provider fixtures SHALL retain authentic provenance and Apache modules SHALL NOT import Gateway code. Candidate-source checks SHALL build Gateway from `ai-gateway/go.mod` with `GOWORK=off`, resolving SDK source through its local replacements.

#### Scenario: Intermediate PR validation
- **WHEN** this change is validated independently of its successor change
- **THEN** provider-tool contract, native MCP request conversion, supplied-history continuation, lifecycle, privacy and module checks SHALL pass independently; synthetic provider tests SHALL NOT claim live MCP effects or deployed readiness
