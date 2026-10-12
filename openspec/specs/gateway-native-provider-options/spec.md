# gateway-native-provider-options Specification

## Purpose

Define opaque native provider-option forwarding at supported Gateway scopes, consumption-backed bypass protections, and independent direct-route evidence.

## Requirements

### Requirement: Opaque options reach native adapters at supported scopes
The Gateway SHALL forward ordinary provider-option namespace objects to native adapters at their original supported call, message, content-part, function-tool and nested tool-result file-entry scopes. It SHALL preserve namespace spelling, opaque namespace JSON bytes and meaningful empty/nested values without mutation, field inventories, namespace filtering, hoisting or cross-provider translation.

#### Scenario: Empty and nested options survive mapping
- **WHEN** a supported direct unary or streaming request carries namespace objects containing null, false, zero, empty strings, empty arrays, empty objects and nested JSON at the supported scopes
- **THEN** the mapped model SHALL receive the original namespaces and namespace JSON at those exact scopes, including explicitly empty namespace objects
- **AND** the input SHALL remain unchanged

#### Scenario: Irrelevant and future fields are not inventoried
- **WHEN** a direct request carries an ordinary namespace irrelevant to the selected adapter or an ordinary field absent from a Gateway inventory
- **THEN** the Gateway SHALL retain it for native interpretation rather than silently remove it or refuse it merely for being unlisted

#### Scenario: Native namespace precedence remains authoritative
- **WHEN** a request supplies competing supported native namespace aliases or a differently cased namespace
- **THEN** actual native consumption SHALL follow the adapter's existing exact matching and precedence without Gateway merging or spelling normalization

### Requirement: Native option consumption and precedence authority
Native adapters SHALL remain responsible for ordinary namespace selection, option validation, consumption and precedence. Forwarding SHALL NOT promise that unknown fields are emitted into every native HTTP request.

#### Scenario: Native option consumption and precedence authority
- **WHEN** the Gateway forwards an unknown ordinary field alongside a supported native option
- **THEN** the adapter SHALL decide selection, validation, consumption and precedence without a promise to emit the unknown field into HTTP

### Requirement: Concrete native bypass protections
The Gateway SHALL protect credentials, authorized accounts and destinations, resolved model/prompt ownership, mapped role/content unions, tool ownership and expected transport/execution against concrete option overrides. Native-specific protections SHALL depend on the actual consuming adapter, namespace, scope, recognized spelling and request precedence; they SHALL run before external provider I/O. Native precedence that already prevents an override SHALL remain authoritative.

#### Scenario: Compatible extensions cannot replace mapped ownership
- **WHEN** a matching compatible namespace at a consumed scope can override the configured model, mapped role/content, tool ownership or expected transport
- **THEN** the actual bypass SHALL fail safely before external provider I/O unless existing native precedence already prevents it
- **AND** no unauthorized native request SHALL be emitted

#### Scenario: Same spelling is ordinary in a non-consuming scope
- **WHEN** an ordinary field with a protected-looking name occurs in a namespace or scope that cannot cause that native bypass
- **THEN** it SHALL remain available to native interpretation rather than fail under a universal spelling rule

#### Scenario: Safe contextual history is not an execution override
- **WHEN** supported supplied Anthropic history carries safe compaction metadata on an assistant text part or nested caller metadata on a local assistant function-call
- **THEN** it SHALL follow native conversion without enabling an unsupported role, union or provider-tool family

#### Scenario: MCP history cannot bypass execution ownership
- **WHEN** scoped Anthropic history options would enter the native MCP tool-use conversion branch despite a local function-call's false or absent ProviderExecuted flag
- **THEN** the Gateway SHALL refuse that concrete unsupported execution path before external provider I/O
- **AND** the protection SHALL NOT reject unrelated contextual type or caller values

### Requirement: Harmless native options and safe refusal formatting
Ordinary ignored fields, safe contextual native controls and nested opaque application values SHALL NOT be rejected through blanket spelling rules or a generic field inventory. Refusals SHALL use the existing fixed safe error contract without echoing supplied contents.

#### Scenario: Harmless native options and safe refusal formatting
- **WHEN** one request supplies a protected-looking field in a non-consuming namespace/scope and another request supplies a concrete native bypass
- **THEN** the harmless request SHALL retain its value for native interpretation and the bypass request SHALL use the fixed safe error without echoing supplied content

### Requirement: Consumption-backed guard weakening audit
Before weakening global guards, the implementation SHALL record each actual consumed override path, namespace/scope, recognized spelling, native precedence, disposition and paired bypass/harmless regression case. This audit SHALL NOT introduce a new provider-field inventory or blanket all-provider scope validator.

#### Scenario: Consumption-backed guard weakening audit
- **WHEN** a change weakens a global option guard for an actual native consumed path
- **THEN** the audit SHALL record namespace/scope/spelling, native precedence, disposition and paired bypass/harmless cases without adding a provider inventory or all-provider scope validator

### Requirement: Native-consumed request semantics have independent proof
Acceptance SHALL include the exact registered TypeScript Gateway client and independent Go Grafana client driving direct unary and streaming production handlers and the authenticated command. Synthetic native Anthropic, OpenAI Responses and compatible HTTP request assertions SHALL prove consumed values, scoped extensions, namespace precedence, ordinary negative controls and supplied caller history. Missing codecs SHALL remain explicit errors before model invocation.

#### Scenario: Anthropic local assistant-call caller is consumed
- **WHEN** either client sends supported local assistant function-call history carrying supplied Anthropic caller metadata
- **THEN** native fake-request assertions SHALL show the consumed assistant tool-use caller values in original history order for both invocation modes
- **AND** no response metadata codec, server-side tool execution or provider-tool activation SHALL be required

#### Scenario: Ordinary tool-result caller reaches the adapter without a consumption claim
- **WHEN** either client sends ordinary tool-role result history carrying supplied Anthropic caller metadata
- **THEN** mapped options SHALL retain it at the original result-part scope through native adapter invocation in both modes
- **AND** native request assertions SHALL reflect the adapter's existing behavior, which currently ignores caller on ordinary tool-role results, unless exact registered native semantics establish otherwise
- **AND** acceptance SHALL NOT require a native converter change or activation of deferred assistant/provider-executed result branches

#### Scenario: OpenAI and Azure history controls are consumed
- **WHEN** either client supplies supported phase, item/reference and reasoning controls under the namespace selected by the native OpenAI adapter
- **THEN** unary and streaming native fake requests SHALL reflect the same native consumption and precedence as equivalent direct adapter options

#### Scenario: Compatible scoped extensions are consumed
- **WHEN** either client supplies safe compatible configured-name call extensions and supported message/part extensions with empty and nested values
- **THEN** native fake requests SHALL retain their native request positions and meanings in both modes

#### Scenario: Deferred codec remains explicit
- **WHEN** a request requires an unsupported provider tool, approval, output-level tool-result option or other missing codec
- **THEN** it SHALL fail under the existing unsupported-family contract before model invocation rather than appear successful after dropping options

### Requirement: Supplied native history evidence is request-only
Supplied history SHALL prove only the request path, not output-derived continuation or newly activated tools.

#### Scenario: Supplied native history evidence is request-only
- **WHEN** a fake native request proves supplied caller-history consumption
- **THEN** the evidence SHALL NOT claim output-derived continuation or activation of new tool families

### Requirement: Request isolation and capture independence
Mapping, native invocation and observers SHALL NOT mutate caller inputs or reusable adapter configuration, share option values across concurrent invocations or promote options into other scopes. Operator capture policy SHALL NOT change provider-bound request or caller-result semantics. This change SHALL preserve operator/consumer capture settings and SHALL NOT reflect credential-bearing host controls.

#### Scenario: Concurrent distinct invocations remain isolated
- **WHEN** concurrent unary and streaming requests carry distinct namespace JSON and history markers through a shared handler and native adapter configuration
- **THEN** each native request SHALL contain only its own scoped values
- **AND** inputs SHALL remain unchanged and Go race checks SHALL pass

#### Scenario: Operator observation does not filter semantics
- **WHEN** equivalent direct requests execute with operator observation enabled and disabled
- **THEN** the native requests and caller-visible results SHALL be semantically equivalent
- **AND** credential-bearing rejected controls SHALL remain absent from server capture and responses

### Requirement: Evidence stays within its provenance and acceptance boundary
The implementation SHALL ship applicable specs, centralized documentation, ProviderWire/parity/module validation and focused regression tests together. Synthetic native requests SHALL remain focused test evidence and SHALL NOT be labeled recorded provider fixtures. Direct-route evidence SHALL NOT be presented as fallback eligibility, live provider acceptance or full continuation proof.

#### Scenario: Fresh bounded delivery is validated
- **WHEN** the change is considered implemented
- **THEN** registered ProviderWire, command, parity and applicable candidate-source module/boundary checks SHALL pass
- **AND** the coverage summary SHALL distinguish mapped preservation, native fake-request consumption and remaining support/evidence gaps
