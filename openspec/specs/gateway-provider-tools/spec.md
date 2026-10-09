# gateway-provider-tools Specification

## Purpose

Define bounded provider-tool execution and continuation on direct Gateway routes, including ownership, metadata, lifecycle, privacy and client acceptance.

## Requirements

### Requirement: Distinct registered provider definitions
Unary and streaming routes SHALL map provider tools with only registered type, ID, name and required object args. Empty/nested args and aliases SHALL survive. Function-only fields, including explicit empty/false fields and definition-level providerOptions, SHALL fail complete validation before resolution or invocation. Native providers SHALL retain tool support and conversion authority.

#### Scenario: Mixed definitions
- **WHEN** function and provider definitions are supplied with aliases and empty or nested args
- **THEN** their ordered distinct shapes SHALL reach the native adapter

#### Scenario: Invalid provider definition
- **WHEN** provider args is missing/null/nonobject or a function-only member is present
- **THEN** both HTTP modes SHALL fail before model invocation

### Requirement: Registered execution markers
Calls/input-start SHALL preserve providerExecuted true. Dynamic and preliminary SHALL preserve absent/false/true on their registered arms; absent and false retain disabled/final semantics. Results SHALL preserve preliminary but SHALL NOT emit providerExecuted. Definition type alone SHALL NOT imply execution ownership.

#### Scenario: Hosted and client-owned calls
- **WHEN** provider-defined tools emit hosted or client-owned calls
- **THEN** both clients SHALL retain the returned ownership without inventing a result-level ownership field

#### Scenario: Input-start presence
- **WHEN** input-start dynamic is absent, false or true
- **THEN** the encoded SSE and Go client SHALL preserve each state for subsequent inference

### Requirement: Provider result and continuation transport
Unary/SSE output SHALL preserve ordered calls and non-null JSON results, including error, dynamic and preliminary semantics. Assistant provider calls and basic assistant-side results SHALL map into continuation. Ordinary tool-part options SHALL retain opaque object namespaces while rejecting host-reserved controls.

#### Scenario: Selected result values
- **WHEN** output results contain empty string, false, zero, empty object or array
- **THEN** those values SHALL survive; output null SHALL fail without changing nullable prompt JSON result arms

#### Scenario: Native continuation
- **WHEN** either client replays an aliased provider call and result in assistant history
- **THEN** native conversion SHALL preserve the pairing without a Gateway session or executor

### Requirement: Tool option consuming authority and deferred content boundaries
Native-consuming adapters SHALL reject fields that could override validated tool identity, shape or request authority at their actual consuming scopes; unknown extension fields SHALL NOT be projected away. Nested output/content options and deferred content families SHALL remain unsupported.

#### Scenario: Tool option consuming authority and deferred content boundaries
- **WHEN** an ordinary tool option at its consuming scope would override validated tool identity or authority
- **THEN** the adapter SHALL reject that bypass without projecting unknown extensions away, and nested output/content options and deferred families SHALL remain unsupported

### Requirement: History-aware deferred result correlation
Output results SHALL match current-response calls or unresolved provider-owned assistant calls from request history. Completed/client-owned historical calls SHALL NOT qualify. Unknown IDs, mismatched names, duplicate calls, results before their matching call, and results after completion SHALL fail safely. Unrelated results in truncated history SHALL NOT seed new pending calls. State SHALL retain only bounded identity/lifecycle information and SHALL be discarded per request.

#### Scenario: Deferred success or error
- **WHEN** a provider call finishes without a result and the next request includes it unresolved in history
- **THEN** unary and streaming success/error results SHALL be accepted without repeating the call

#### Scenario: Invalid historical match
- **WHEN** an output matches only unknown, completed, out-of-order, mismatched or client-owned history
- **THEN** unary output SHALL fail before commitment and streaming SHALL use the bounded safe terminal path

#### Scenario: History accounting
- **WHEN** bounded request history seeds deferred state
- **THEN** its entries SHALL NOT consume the current provider stream-part budget or retain history payloads

### Requirement: Opaque tool metadata
Tool metadata SHALL use gateway-provider-metadata's shared bounded opaque transport, preserving unknown namespace objects, nested extension fields and omission versus explicit empty objects. Provider-specific metadata interpretation SHALL belong to the actual native consuming boundary; the codec SHALL retain generic tool ownership, ID/name, deferred-result and lifecycle checks without interpreting namespace fields as routing or execution authority.

#### Scenario: Correlation metadata with extensions
- **WHEN** tool metadata contains valid caller correlation alongside unknown object namespaces and nested fields
- **THEN** both clients SHALL receive all metadata unchanged, including omitted/empty presence, independently of operator telemetry capture

#### Scenario: Native hosted MCP consumption
- **WHEN** the selected native adapter consumes Anthropic mcpServers and provider-owned MCP history or an OpenAI MCP tool definition
- **THEN** the native request SHALL preserve the provider's MCP settings and history semantics without a Gateway capability rejection; opaque returned metadata SHALL NOT trigger codec-level configured-server classification or membership rejection
- **AND** MCP authorization and headers SHALL remain distinct from the configured inference credential and metadata-only operator capture

#### Scenario: Foreign MCP settings are inert
- **WHEN** compatible inference uses a configured namespace other than anthropic and receives foreign anthropic.mcpServers settings
- **THEN** ordinary inference SHALL use the configured compatible credential without forwarding the MCP URL/token or mcp_servers to the backend, invoking the MCP endpoint or capturing those values in server observations; caller-owned request metadata MAY retain the submitted body

### Requirement: Tool metadata aggregate resource and capture boundaries
Aggregate original bytes/cardinality and complete encoded response/frame sizes SHALL remain bounded. Returned metadata SHALL NOT authorize telemetry capture.

#### Scenario: Tool metadata aggregate resource and capture boundaries
- **WHEN** tool metadata namespace objects each fit the response/frame bound individually but their aggregate exceeds it
- **THEN** the output SHALL fail within shared aggregate/complete-envelope limits and metadata SHALL NOT authorize telemetry capture

### Requirement: Native hosted MCP and foreign namespace conversion authority
Hosted MCP configuration, provider tool definitions and continuation metadata SHALL follow the selected native provider's existing conversion semantics without a Gateway hosted-MCP capability gate. Foreign namespaces not consumed by a native adapter SHALL retain the base runtime's forwarding semantics without gaining execution or routing authority.

#### Scenario: Native hosted MCP and foreign namespace conversion authority
- **WHEN** a selected native adapter consumes hosted MCP settings while another receives a foreign unconsumed namespace
- **THEN** the consuming adapter SHALL retain existing MCP semantics without a Gateway gate, and foreign namespaces SHALL keep base forwarding semantics without execution/routing authority

### Requirement: Bounded preliminary lifecycle
Streaming SHALL allow multiple correlated previews followed by exactly one final result. Complete calls without results MAY finish for deferred execution, but unfinished previews and post-final results SHALL fail. Existing frame/part limits, cancellation, ordered errors, authoritative finish and bounded cleanup SHALL remain. Pre-call image previews SHALL remain unsupported under WP16 (#110).

#### Scenario: Preview closure
- **WHEN** a call receives two preliminary results and one final result
- **THEN** all events SHALL arrive in order and any later result SHALL fail

#### Scenario: Invalid finish or exceeded bound
- **WHEN** finish arrives during a preview series or output/metadata exceeds a configured bound
- **THEN** oversized bytes SHALL NOT be committed and cleanup SHALL remain bounded

### Requirement: Stateless execution and private observation
The Gateway SHALL execute no local tools and retain no cross-request state. Configured fallback SHALL retain gateway-fallback's native option interpretation, candidate order, retry decisions, cancellation and successful-result/first-provider-part selection boundary without a blanket tool/history refusal. Post-selection provider, encoding or lifecycle failures SHALL NOT advance to another candidate.

#### Scenario: Authenticated native round trip
- **WHEN** both clients execute direct Anthropic code-execution call/result continuation in unary and streaming modes
- **THEN** native requests SHALL preserve aliases and results, logical generation counts SHALL match requests, and exported observations SHALL exclude private tool values

#### Scenario: Fallback selection boundary
- **WHEN** a candidate returns a successful result or its first provider part, including a tool, start or error part
- **THEN** it SHALL remain selected through later provider, encoding and lifecycle failures without replay on another candidate

### Requirement: Preselection native effects are not exactly once
Native work before an eligible pre-selection failure may be repeated; no exactly-once provider generation or effect guarantee is made. Delivery SHALL NOT claim such a guarantee.

#### Scenario: Preselection native effects are not exactly once
- **WHEN** native work occurs before an eligible preselection failure and a later candidate runs
- **THEN** that work may be repeated and no exactly-once generation or effect guarantee SHALL be claimed

### Requirement: Canonical tool generation and metadata-only privacy
Each HTTP invocation SHALL have one canonical logical generation; tool payloads, names, IDs, metadata and physical attribution SHALL remain absent from metadata-only exports, logs and metric labels.

#### Scenario: Canonical tool generation and metadata-only privacy
- **WHEN** one HTTP tool-bearing call invokes multiple physical candidates
- **THEN** it SHALL remain one canonical logical generation and metadata-only exports/logs/metric labels SHALL exclude tool payloads/names/IDs/metadata and physical attribution

### Requirement: Independent acceptance evidence
Both registered clients SHALL exercise real-handler and authenticated-command scenarios without requiring MCP. Request goldens SHALL be captured from the pinned client. Provider fixtures SHALL retain authentic provenance and Apache modules SHALL NOT import Gateway code. Candidate-source checks SHALL select `go.gateway.work` explicitly; standalone Gateway builds SHALL use published pins with GOWORK disabled.

#### Scenario: Intermediate PR validation
- **WHEN** this change is validated independently of its successor change
- **THEN** provider-tool contract, native MCP request conversion, supplied-history continuation, lifecycle, privacy and module checks SHALL pass independently; synthetic provider tests SHALL NOT claim live MCP effects or deployed readiness
