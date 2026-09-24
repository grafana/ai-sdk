## MODIFIED Requirements

### Requirement: Unsupported capability families
Custom content, provider tools and tool approvals, structured output, unsupported provider-option scopes, body headers, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs. Function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools and gateway-streaming-function-tools subsets, including the ordinary file-result extension. Function-tool and ordinary file-entry provider options SHALL be supported within those subsets; deferred output-level and non-file nested result options SHALL remain unsupported. Assistant reasoning text and reasoning-file history SHALL execute within gateway-reasoning-content, and scoped continuation options SHALL not enable root options or headers. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

#### Scenario: Reasoning continuation is supported
- **WHEN** a valid assistant reasoning part carries supported scoped continuation options
- **THEN** the model SHALL receive them without enabling deferred capability families

#### Scenario: One unsupported family
- **WHEN** a request activates one unsupported family
- **THEN** the response SHALL name that family and no model SHALL be resolved or invoked

#### Scenario: Malformed unsupported branch
- **WHEN** an unsupported branch violates the complete request schema
- **THEN** it SHALL fail as schema-invalid rather than as a valid unsupported capability

#### Scenario: Ordinary files no longer trigger blanket rejection
- **WHEN** a schema-valid unary or streaming request contains only supported text, ordinary files, supported tool history, and permitted message/file options
- **THEN** it SHALL map without a blanket files or provider-options failure and continue through the existing execution boundary
