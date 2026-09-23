## ADDED Requirements

### Requirement: Distinct registered provider definitions
Unary and streaming direct routes SHALL map provider tools using only the registered type, ID, name and required object args. Empty args and nested JSON values SHALL survive. Complete request validation SHALL reject function-only members, including definition-level providerOptions, before policy, resolution or model invocation. Native providers SHALL retain responsibility for native tool support, argument conversion and alias mapping.

#### Scenario: Mixed list with provider alias
- **WHEN** a request contains a function definition and a provider definition with an aliased name, registered ID and empty or populated args
- **THEN** order, types, names, ID and args SHALL reach the selected provider without treating the provider definition as a function

#### Scenario: Forbidden member even when empty
- **WHEN** a provider definition contains description, inputSchema, inputExamples, strict or providerOptions, including empty or false values
- **THEN** both HTTP modes SHALL reject it before policy, resolution and invocation

#### Scenario: Args is not an object
- **WHEN** provider args is absent, null, an array or a scalar
- **THEN** strict mapping SHALL fail without invoking a model

### Requirement: Direct tool validation before backend I/O
Direct native-provider entry points and the Go Gateway client SHALL reject representably populated inactive tool fields instead of silently dropping them. A nil Go provider-tool Args map SHALL mean an empty object at conversion, while non-nil Args maps SHALL contain valid JSON values. This adaptation SHALL NOT permit missing or null args in HTTP input. A provider definition SHALL NOT carry function-only options; a function definition SHALL NOT carry provider ID/args. Function strict presence SHALL remain unchanged. Unsupported but structurally valid native tool IDs SHALL retain the provider's documented unsupported-feature handling.

#### Scenario: Direct provider tool carries function options
- **WHEN** a direct caller supplies a provider-typed tool with non-nil Strict, InputSchema, InputExamples or ProviderOptions, or a nonempty Description
- **THEN** generate and stream setup SHALL fail before native HTTP I/O

#### Scenario: Direct nil provider args normalize
- **WHEN** a direct Go caller omits provider-tool Args but supplies the required ID and name
- **THEN** unary and stream conversion SHALL proceed with empty-object semantics, including an explicit `args: {}` on the Go Gateway request

#### Scenario: Function option remains legal
- **WHEN** a function definition uses strict false and ordinary namespaced provider options
- **THEN** its existing native forwarding and warning semantics SHALL remain intact

### Requirement: Request-level Anthropic MCP server projection
The registered Vercel Gateway client's `providerOptions.anthropic.mcpServers` SHALL be accepted for direct Anthropic calls in unary and streaming modes. The adapter SHALL validate and map the registered ordered server definitions, including type `url`, name, URL, optional authorization token and tool configuration, to only the selected Anthropic provider. Other nonempty root provider options SHALL remain unsupported until WP21; unsupported backend choices SHALL fail safely before native I/O without disclosing private backend identity. The Gateway SHALL NOT replace request configuration with invented host aliases, execute MCP locally or reflect server URLs/tokens. Authentication, configured provider isolation, bounded input, and product-specific destination validation SHALL protect the capability; the published Vercel client is not evidence for the private hosted Gateway's egress policy.

#### Scenario: Authenticated direct Anthropic MCP request
- **WHEN** an authenticated client sends two valid MCP server definitions with distinct names, optional token and toolConfiguration, including empty allowedTools and explicit enabled false
- **THEN** the selected native Anthropic request SHALL contain the corresponding ordered `mcp_servers` values and beta while another physical provider, public output and telemetry SHALL receive no URL or token

#### Scenario: Invalid or non-Anthropic request
- **WHEN** a server entry has an invalid registered shape, unsafe destination, oversize value or duplicate name, or the selected direct backend is not Anthropic
- **THEN** invocation SHALL fail safely and no MCP credential SHALL be forwarded or exposed

#### Scenario: Unrelated root provider options stay deferred
- **WHEN** a request contains another nonempty root namespace or another nonempty `anthropic` member alongside `mcpServers`
- **THEN** the request SHALL fail as unsupported before invoking a provider; empty values that remain semantically no-op SHALL not enable unrelated options

### Requirement: Boolean execution and dynamic semantics
Provider-domain execution, dynamic and preliminary markers SHALL be booleans wherever absent and false have the same meaning. Calls and input-start SHALL preserve providerExecuted true; dynamic SHALL be preserved on registered call/start/result arms and preliminary on result arms. Result DTOs SHALL NOT invent a providerExecuted member. Provider-defined tools SHALL NOT be assumed provider-executed solely from their definition type. Input-start dynamic absence SHALL remain distinct from explicit false through provider transport and text-stream adaptation: an explicit value SHALL win, and only absence SHALL infer from the application tool definition. UI projection SHALL independently classify known tools by their application definitions and unknown tools by the text-stream value, matching pinned upstream.

#### Scenario: Hosted call is never executed locally
- **WHEN** a provider-owned call and result traverse unary or streaming through either client
- **THEN** providerExecuted true SHALL remain on the call and application tool execution count SHALL remain zero

#### Scenario: Disabled marker normalization
- **WHEN** output uses absent or false markers whose meanings are equivalent, excluding input-start dynamic
- **THEN** both clients SHALL expose equivalent disabled semantics, while true SHALL remain distinguishable

#### Scenario: Input-start dynamic presence affects inference
- **WHEN** input-start for a dynamic application tool carries dynamic absent, false or true
- **THEN** both transports SHALL preserve all three states and actual Go and pinned upstream text-stream parts SHALL respectively expose inferred true, explicit false or explicit true
- **AND** UI chunks for that known dynamic tool SHALL carry dynamic true in all three cases

#### Scenario: Known ordinary and unknown UI projection
- **WHEN** an input-start has absent dynamic for a known ordinary tool or explicit false for an unknown tool
- **THEN** the known tool's text-stream part SHALL infer false but its UI chunk SHALL omit dynamic, while the unknown tool's UI chunk SHALL retain false

#### Scenario: Provider-defined client execution
- **WHEN** a native provider-defined tool emits a client-owned call
- **THEN** the client SHALL preserve client execution ownership rather than inferring hosted execution from its definition

#### Scenario: Dynamic undeclared name
- **WHEN** a provider emits a supported dynamic tool call whose name is absent from the request definitions
- **THEN** the dynamic marker and provider-supplied name SHALL survive without requiring a new Gateway tool registration

### Requirement: Provider result and continuation transport
Unary and streaming output SHALL preserve ordered correlated tool results with non-null JSON result, error status, dynamic and preliminary semantics. Basic registered assistant-side tool results and provider-executed assistant calls SHALL map into continuation history. Tool-call/result part provider options SHALL preserve ordinary namespaced objects while rejecting reserved host controls. Nested output/content options, approvals and later content families SHALL remain explicitly unsupported.

#### Scenario: Hosted result followed by another request
- **WHEN** either client sends a prior provider-owned call and basic result inline in an assistant message
- **THEN** the next native request SHALL retain their IDs, aliases, execution marker, selected output and supported continuation metadata without a Gateway-owned session

### Requirement: History-aware deferred result correlation
Unary and streaming results SHALL correlate against calls in the current response or eligible unresolved provider-executed assistant calls in the supplied request history. Historical calls SHALL be eligible only when no completed result is present; client-owned historical calls SHALL NOT satisfy deferred provider-result correlation. Complete provider-owned calls SHALL be allowed to finish without a result and complete in a later request without repeating the call. Unknown IDs, name mismatches and results for already-completed IDs SHALL fail safely. Correlation state SHALL be request-local, bounded by request bytes and current response part/content limits, and SHALL NOT retain copied history payloads or persist between requests.

#### Scenario: Result-only continuation
- **WHEN** each client receives a provider-owned call followed by finish and supplies that unresolved call in the next request history
- **THEN** a subsequent success or error result with the matching ID/name SHALL be accepted without repeating the call, and no local tool SHALL execute

#### Scenario: Invalid historical match
- **WHEN** a result ID is unknown, its name mismatches, its historical call already has a completed result or only a client-owned historical call matches
- **THEN** unary output SHALL fail before success commitment and streaming output SHALL fail through the established bounded safe terminal path

#### Scenario: Bounded history reconstruction
- **WHEN** a request contains a large history within the request byte limit and receives deferred results
- **THEN** correlation SHALL retain only bounded identity/lifecycle state, history entries SHALL NOT consume provider stream-part counts, and state SHALL be discarded at request completion

### Requirement: Provider result payload preservation
Supported provider output SHALL preserve non-null JSON result values and keep deferred content-family failures explicit.

#### Scenario: Result payload shapes
- **WHEN** a result contains valid non-null JSON such as an empty string, false, zero, empty array or empty object
- **THEN** its selected value SHALL survive without omission or substitution

#### Scenario: Non-null output and nullable prompt arm are distinct
- **WHEN** provider output contains result null
- **THEN** it SHALL fail safely, without changing the existing acceptance of null inside a selected prompt JSON tool-result output

#### Scenario: Deferred compound output
- **WHEN** a provider tool also emits a source, file, approval or other not-yet-supported output family
- **THEN** that output SHALL follow the established safe unsupported-family failure instead of being silently discarded

### Requirement: Reviewed tool metadata without physical attribution
Public tool metadata SHALL be projected through explicit reviewed field/shape mappings backed by pinned producer/replay evidence. For Anthropic MCP tool calls/results, `anthropic.type: 'mcp-tool-use'` and a bounded `anthropic.serverName` matching a caller-supplied server in this request SHALL be preserved so registered continuation can replay the native MCP block. The name SHALL NOT be replaced by a URL, authorization token or physical provider identity. Necessary supported correlation metadata SHALL survive through both clients and continuation; unknown or private fields SHALL be omitted. Invalid required supported metadata SHALL fail safely. Backend identity, credentials, raw request/response material and fallback topology SHALL NOT be copied into normalized public metadata.

#### Scenario: Metadata needed for replay
- **WHEN** a supported tool emits reviewed metadata and the client reuses it as tool-part options
- **THEN** the next native request SHALL preserve the intended correlation or native tool block semantics

#### Scenario: Hostile metadata
- **WHEN** supported metadata is mixed with arbitrary secret-bearing or physical-attribution fields
- **THEN** only reviewed public fields SHALL be emitted and all output size limits SHALL still apply

#### Scenario: MCP metadata is required for continuation
- **WHEN** Anthropic returns an MCP call/result for a server configured in the current request and each client includes that call/result in the next request's assistant history
- **THEN** both clients SHALL receive only reviewed `type/serverName` metadata and the next native request SHALL reproduce the MCP tool-use/result blocks with correct server name, without any public URL or token

#### Scenario: Unconfigured MCP server name
- **WHEN** the native provider emits MCP metadata referring to a server name not configured for the current request, or a continuation uses an unconfigured name
- **THEN** output or mapping SHALL fail safely rather than silently converting it to a regular provider tool

### Requirement: Preliminary results remain bounded and require final completion
The Gateway stream SHALL permit multiple correlated preliminary results followed by one final result, including results for eligible unresolved provider calls in request history. Previews SHALL not close the call. Results unmatched against both current-response calls and eligible history, name mismatches, any result after final, and finish with an unfinished preliminary series SHALL fail safely. Finish without any result for a complete provider-owned call SHALL remain valid for deferred execution. Pre-call provider-specific image preview results remain deferred with generated media (WP16); they SHALL fail safely rather than being treated as correlated WP13 results. Existing ID/input ordering, part/frame budgets, deadlines, ordered non-terminal errors, final finish and bounded cleanup SHALL apply without buffering result history.

#### Scenario: Preview replacement and final result
- **WHEN** a call receives two preliminary results followed by a result with preliminary absent or false
- **THEN** both clients SHALL receive all events in order and the final result SHALL complete the series

#### Scenario: Invalid finalization
- **WHEN** finish arrives after a preview without a final result, or another result follows final
- **THEN** the handler SHALL cancel and produce at most one safe synthetic terminal error

#### Scenario: Result or metadata exceeds a bound
- **WHEN** a unary result, stream frame or preliminary-event count crosses its configured limit
- **THEN** the adapter SHALL fail before committing oversized bytes and cleanup SHALL remain bounded

### Requirement: Direct stateless execution with private logical observation
The Gateway SHALL execute no local tools and retain no cross-request workflow state. Fallback-configured routes SHALL reject provider definitions, tool history and nonempty request-level MCP server configuration (even without tools) before any physical candidate executes. Each HTTP invocation SHALL use one canonical logical middleware chain. Reusable observability SHALL distinguish provider ownership and preliminary replacement; Gateway metadata-only exports, logs and metrics SHALL exclude tool payloads, names, IDs, definitions, metadata and private attribution.

#### Scenario: Two hosted-tool requests
- **WHEN** a client completes a direct provider-tool call and continuation in two HTTP requests
- **THEN** there SHALL be two logical generations with canonical identity, normal usage/finish/error accounting and zero local tool executions

#### Scenario: Provider tool targets fallback
- **WHEN** a provider definition, continuation or request-level MCP server configuration targets a fallback-configured route
- **THEN** the fixed safe unsupported response SHALL be returned with zero primary and secondary invocations

#### Scenario: Metadata-only recording of previews
- **WHEN** a tool emits previews and a final result containing hostile markers
- **THEN** reusable result recording SHALL finalize the result rather than count previews as separate completed tools, and Gateway exports SHALL contain no tool payload markers

### Requirement: Layered pinned acceptance evidence
Acceptance SHALL use the registered Vercel client and independent Go client through the real handler, authenticated command scenarios covering request-level Anthropic MCP configuration/continuation, and native provider request snapshots. Genuine recorded/upstream provider inputs SHALL retain their provenance. Affected frontend semantics SHALL have cross-language regression evidence. Apache packages SHALL not import Gateway implementation, and Gateway checks SHALL pass with committed proxy-resolvable dependency pins and GOWORK disabled.

#### Scenario: Capability acceptance suite
- **WHEN** WP13 verification runs
- **THEN** it SHALL prove definition/marker parity, hosted and client ownership controls, continuation, native conversion, privacy, bounds and fallback rejection without treating fabricated provider payloads as recordings
