## MODIFIED Requirements

### Requirement: Unsupported capability families

Schema-valid files, reasoning content, custom content, provider tools and tool approvals, structured output, non-empty provider options, body headers, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Unary function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools subset. Function-tool provider options SHALL be supported within that subset; deferred nested result options SHALL remain unsupported. Streaming function requests SHALL remain unsupported until WP12. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

### Requirement: Minimal unary success response

A successful unary response SHALL contain only ordered supported text and function-tool-call `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, raw usage, provider metadata, and content metadata SHALL be omitted. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result contains warnings, response metadata, raw usage, backend identity, or provider metadata
- **THEN** none of those values SHALL appear in the unary response document
