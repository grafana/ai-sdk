## MODIFIED Requirements

### Requirement: Unsupported capability families

Schema-valid custom content, provider tools and tool approvals, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Structured output SHALL execute within gateway-structured-output. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs, alongside existing call-level options and body-carried headers.

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

#### Scenario: Reasoning continuation is supported
- **WHEN** a valid assistant reasoning part carries supported scoped continuation options
- **THEN** the model SHALL receive them without enabling deferred capability families

#### Scenario: Ordinary files no longer trigger blanket rejection
- **WHEN** a schema-valid unary or streaming request contains only supported text, ordinary files, supported tool history, and permitted message/file options
- **THEN** it SHALL map without a blanket files or provider-options failure and continue through the existing execution boundary
