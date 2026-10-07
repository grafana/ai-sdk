## MODIFIED Requirements

### Requirement: Unsupported capability families

Schema-valid custom content, provider tools and tool approvals, structured output, and raw output SHALL return a stable invalid-request document naming the unsupported family before resolution or model invocation. Ordinary file inputs and message/file-part options SHALL execute within gateway-file-inputs, alongside existing call-level options and body-carried headers. Function definitions/choices and assistant-call/tool-result history SHALL execute only within the gateway-unary-function-tools and gateway-streaming-function-tools subsets, including the ordinary file-result extension. Function-tool and ordinary file-entry provider options SHALL be supported within those subsets; deferred output-level and non-file nested result options SHALL remain unsupported. Assistant reasoning text and reasoning-file history SHALL execute within gateway-reasoning-content, preserving scoped options under gateway-native-provider-options rather than selected-backend filtering. The runtime SHALL not define client-visible precedence among multiple simultaneously activated unsupported families.

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

### Requirement: Reserved provider options and protected call headers

The runtime SHALL reserve the `grafana`, `gateway`, and `grafana-ai-sdk` provider-option namespaces for the host. A request carrying any of them SHALL be rejected with a stable invalid-request document before resolution or model invocation unless an owning host feature explicitly consumes it. Such controls SHALL never reach native adapters as provider options. This change SHALL NOT implement routing, BYOK or new operator controls. Unknown ordinary namespaces and fields SHALL NOT be treated as host controls merely because they are unlisted.

The runtime SHALL refuse body-carried call headers whose name matches a credential-bearing header, compared without case sensitivity, covering at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key` and `anthropic-api-key`. The refused set SHALL cover every header name the inbound authenticated edge refuses in outer headers, which a test SHALL assert by driving the edge with a valid stack assertion and each candidate name, with an accepted ordinary header as a negative control, and requiring the body mapper to refuse every name the edge refused. Outer authentication SHALL remain independent from body-carried provider call headers; unrelated outer HTTP headers SHALL NOT be forwarded automatically.

Provider-option protections SHALL follow gateway-native-provider-options: actual native consumption and precedence at the consuming namespace and scope SHALL justify any check preventing credential/account/destination, model/prompt, role/union, tool-ownership or transport/execution bypass. A blanket case/underscore/hyphen-folded field list SHALL NOT reject ordinary fields that cannot cause the bypass. Native-specific checks SHALL fail before external provider I/O; schema, reserved-host and protected-body-header refusals SHALL retain their pre-resolution processing order.

These refusals SHALL use fixed documents that never echo the offending namespace, field name, header name or value.

#### Scenario: Reserved namespace
- **WHEN** a request carries the `grafana`, `gateway`, or `grafana-ai-sdk` provider-option namespace without an owning feature consuming it
- **THEN** the response SHALL be a stable invalid-request document naming neither the namespace contents nor the caller's values
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protected provider option
- **WHEN** an option can cause a concrete native model/prompt, credential/account/destination, role/union, tool-ownership or transport/execution bypass
- **THEN** the request SHALL fail with a stable invalid-request document before external provider I/O unless native precedence already prevents the override

#### Scenario: Harmless field with the same spelling
- **WHEN** an ordinary option field has a spelling used by a protected field but its namespace and scope cannot cause the native bypass
- **THEN** it SHALL remain intact for native interpretation rather than be refused under a universal blacklist

#### Scenario: Protected call header
- **WHEN** a request carries a credential-bearing body header in any letter case
- **THEN** the response SHALL be a stable invalid-request document
- **AND** no model SHALL be resolved or invoked

#### Scenario: Protocol header in the body
- **WHEN** a request carries a protocol header name such as `AI-Language-Model-Id` as a body-carried call header
- **THEN** the request SHALL be accepted, because the HTTP contract retains that name in the body
- **AND** the name SHALL NOT reach the selected backend

#### Scenario: Reserved namespace in a different case
- **WHEN** a request carries a provider-option namespace such as `Grafana`
- **THEN** it SHALL be mapped like any other ordinary namespace, because namespace names are compared exactly

## REMOVED Requirements

### Requirement: Selected-backend provider options
**Reason**: Gateway namespace/field inventories silently lose native-consumed semantics and duplicate adapter interpretation. Candidate policy intersection also hides options before the separately owned fallback guard.
**Migration**: Forward opaque ordinary namespaces at supported scopes under gateway-native-provider-options; remove catalog/service option-policy plumbing without a compatibility shim. Use consumption-backed bypass checks and native namespace precedence. Retain fallback restrictions on preserved requests until #319 expands eligibility; do not discard options to make fallback requests eligible.
