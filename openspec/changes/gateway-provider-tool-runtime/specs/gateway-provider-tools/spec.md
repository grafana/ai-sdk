## ADDED Requirements

### Requirement: Distinct registered provider definitions
Unary and streaming direct routes SHALL map provider tools with only registered type, ID, name and required object args. Empty/nested args and aliases SHALL survive. Function-only fields, including explicit empty/false fields and definition-level providerOptions, SHALL fail complete validation before resolution or invocation. Native providers SHALL retain tool support and conversion authority.

#### Scenario: Mixed definitions
- **WHEN** function and provider definitions are supplied with aliases and empty or nested args
- **THEN** their ordered distinct shapes SHALL reach the native adapter

#### Scenario: Invalid provider definition
- **WHEN** provider args is missing/null/nonobject or a function-only member is present
- **THEN** both HTTP modes SHALL fail before model invocation

### Requirement: Registered execution markers
Calls/input-start SHALL preserve providerExecuted true. Dynamic SHALL survive on registered arms and input-start SHALL preserve absent/false/true. Results SHALL preserve preliminary but SHALL NOT emit providerExecuted. Definition type alone SHALL NOT imply execution ownership.

#### Scenario: Hosted and client-owned calls
- **WHEN** provider-defined tools emit hosted or client-owned calls
- **THEN** both clients SHALL retain the returned ownership without inventing a result-level ownership field

#### Scenario: Input-start presence
- **WHEN** input-start dynamic is absent, false or true
- **THEN** the encoded SSE and Go client SHALL preserve each state for subsequent inference

### Requirement: Provider result and continuation transport
Unary/SSE output SHALL preserve ordered calls and non-null JSON results, including error, dynamic and preliminary semantics. Assistant provider calls and basic assistant-side results SHALL map into continuation. Ordinary tool-part options SHALL retain object namespaces under the Gateway's protected-field and configured-backend policy while rejecting host-reserved controls. Nested output/content options and deferred content families SHALL remain unsupported.

#### Scenario: Selected result values
- **WHEN** output results contain empty string, false, zero, empty object or array
- **THEN** those values SHALL survive; output null SHALL fail without changing nullable prompt JSON result arms

#### Scenario: Native continuation
- **WHEN** either client replays an aliased provider call and result in assistant history
- **THEN** native conversion SHALL preserve the pairing without a Gateway session or executor

### Requirement: History-aware deferred result correlation
Output results SHALL match current-response calls or unresolved provider-owned assistant calls from request history. Completed/client-owned historical calls SHALL NOT qualify. Unknown IDs, mismatched names, duplicate calls and results after completion SHALL fail safely. State SHALL retain only bounded identity/lifecycle information and SHALL be discarded per request.

#### Scenario: Deferred success or error
- **WHEN** a provider call finishes without a result and the next request includes it unresolved in history
- **THEN** unary and streaming success/error results SHALL be accepted without repeating the call

#### Scenario: Invalid historical match
- **WHEN** an output matches only unknown, completed, mismatched or client-owned history
- **THEN** unary output SHALL fail before commitment and streaming SHALL use the bounded safe terminal path

#### Scenario: History accounting
- **WHEN** bounded request history seeds deferred state
- **THEN** its entries SHALL NOT consume the current provider stream-part budget or retain history payloads

### Requirement: Reviewed non-MCP tool metadata
Public metadata SHALL project reviewed Anthropic caller type/toolId and OpenAI/Azure itemId, namespace and caller type/callerId shapes. Unknown/private fields SHALL be omitted; malformed supported fields SHALL fail. Metadata bytes and cardinality SHALL be bounded before copying. This capability SHALL NOT enable MCP: MCP server options and explicit MCP continuation/output metadata SHALL remain rejected. Other root, message and part options and call headers SHALL retain the Gateway's existing protected-field and configured-backend policy.

#### Scenario: Correlation metadata with private fields
- **WHEN** reviewed metadata includes arbitrary secret-bearing fields
- **THEN** only reviewed fields SHALL reach both clients and subsequent native continuation

#### Scenario: MCP remains deferred
- **WHEN** a request supplies mcpServers or MCP tool-part options, or output contains MCP metadata
- **THEN** this runtime boundary SHALL reject it safely rather than treat it as ordinary provider-tool metadata

### Requirement: Bounded preliminary lifecycle
Streaming SHALL allow multiple correlated previews followed by exactly one final result. Complete calls without results MAY finish for deferred execution, but unfinished previews and post-final results SHALL fail. Existing frame/part limits, cancellation, ordered errors, authoritative finish and bounded cleanup SHALL remain. Pre-call image previews SHALL remain unsupported under WP16 (#110).

#### Scenario: Preview closure
- **WHEN** a call receives two preliminary results and one final result
- **THEN** all events SHALL arrive in order and any later result SHALL fail

#### Scenario: Invalid finish or exceeded bound
- **WHEN** finish arrives during a preview series or output/metadata exceeds a configured bound
- **THEN** oversized bytes SHALL NOT be committed and cleanup SHALL remain bounded

### Requirement: Stateless direct execution and private observation
The Gateway SHALL execute no local tools and retain no cross-request state. Fallback routes SHALL reject definitions, choices and tool history before any candidate. Each HTTP invocation SHALL have one canonical logical generation; tool payloads, names, IDs, metadata and physical attribution SHALL remain absent from metadata-only exports, logs and metric labels.

#### Scenario: Authenticated native round trip
- **WHEN** both clients execute direct Anthropic code-execution call/result continuation in unary and streaming modes
- **THEN** native requests SHALL preserve aliases and results, logical generation counts SHALL match requests, and exported observations SHALL exclude private tool values

#### Scenario: Fallback effects
- **WHEN** provider definitions or tool history target fallback
- **THEN** both candidate invocation counts SHALL remain zero

### Requirement: Independent acceptance evidence
Both registered clients SHALL exercise real-handler and authenticated-command scenarios without requiring MCP. Request goldens SHALL be captured from the pinned client. Provider fixtures SHALL retain authentic provenance and Apache modules SHALL NOT import Gateway code. Required source checks SHALL use candidate modules with merged internal pins; standalone Gateway validation SHALL gate artifact publication separately.

#### Scenario: Provider-tool validation before MCP activation
- **WHEN** this change is validated before MCP support lands
- **THEN** provider-tool contract, native continuation, lifecycle, privacy and module checks SHALL pass with MCP still rejected
