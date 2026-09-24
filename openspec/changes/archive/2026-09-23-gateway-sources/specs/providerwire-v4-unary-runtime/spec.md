## MODIFIED Requirements

### Requirement: Minimal unary success response

A successful unary response SHALL contain only ordered supported text, function-tool-call and source `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, raw usage, provider metadata, and content metadata SHALL be omitted except the closed public source metadata defined by gateway-sources. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call/source subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result contains warnings, response metadata, raw usage, backend identity, or provider metadata
- **THEN** none of those values SHALL appear in the unary response document except the explicitly normalized public source metadata
