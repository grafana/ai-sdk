## ADDED Requirements

### Requirement: Narrow request-level server projection
Direct Anthropic unary and streaming routes SHALL accept registered `providerOptions.anthropic.mcpServers` with ordered type/url/name definitions, optional authorizationToken and toolConfiguration. Omitted versus explicit empty token, enabled false and empty allowedTools SHALL retain native semantics. Other nonempty root members/namespaces SHALL remain unsupported. Empty no-op options SHALL not enable unrelated capabilities.

#### Scenario: Native projection
- **WHEN** an authenticated client supplies valid ordered server definitions with optional token and explicit disabled/empty configuration
- **THEN** only the selected native Anthropic request SHALL receive matching `mcp_servers` and the required beta

#### Scenario: Unrelated root options
- **WHEN** another nonempty namespace or Anthropic member accompanies mcpServers
- **THEN** mapping SHALL fail before provider invocation

### Requirement: Bounded destination policy
The mapper SHALL enforce server count, unique names, URL/token/name/allowed-tool limits and registered field shapes. URLs SHALL use HTTPS with a host and no embedded credentials or fragment delimiter. Failures SHALL NOT expose URL/token material. These restrictions SHALL be documented as Grafana policy, not inferred Vercel hosted-service behavior.

#### Scenario: Invalid server definition
- **WHEN** a definition is malformed, duplicated, oversized or has an unsafe destination, including an empty URL fragment
- **THEN** both HTTP modes SHALL fail safely without physical invocation or credential disclosure

### Requirement: Configured direct-route eligibility
Service composition SHALL permit MCP effects only for configured single direct Anthropic routes. Wrapped public Provider() identity SHALL NOT determine eligibility. Non-Anthropic and fallback routes SHALL reject nonempty MCP configuration before any candidate, even without a tools list. The Gateway SHALL NOT execute remote MCP itself or replace request servers with invented host aliases.

#### Scenario: MCP without definitions targets fallback
- **WHEN** either client sends only MCP root options to a fallback route
- **THEN** the safe unsupported response SHALL precede both candidate invocations

#### Scenario: Public identity is wrapped
- **WHEN** the resolved public model reports grafana
- **THEN** the construction-time physical-route capability SHALL still allow direct Anthropic and deny compatible/fallback routes

### Requirement: Configured-name continuation metadata
MCP tool calls/results SHALL expose only Anthropic type mcp-tool-use and bounded serverName matching a server configured in the current request. MCP caller/private fields SHALL be omitted. Request continuation SHALL require the same configured-name match and provider-owned call context. Unconfigured or malformed required metadata SHALL fail rather than becoming an ordinary tool. Endpoints, tokens, private backend identity and raw provider data SHALL NOT enter normalized public metadata.

#### Scenario: MCP replay
- **WHEN** either client reuses returned MCP call/result metadata in assistant history with matching root server configuration
- **THEN** the native continuation SHALL reproduce MCP tool-use/result blocks and correct server name, without normalized URL/token output

#### Scenario: Invalid name or ownership
- **WHEN** response metadata or request history uses an unconfigured name, missing required server field or MCP call metadata on a client-owned call
- **THEN** the bounded mapper SHALL fail safely

### Requirement: Deferred MCP uses existing bounded transport
MCP SHALL use the provider-tool runtime's request-local correlation and unary/SSE lifecycle. A provider-owned MCP call MAY finish without a result and complete from unresolved history in a later request. This extension SHALL NOT loosen result non-null rules, preview finalization, bounds, cancellation or deferred-family rejection.

#### Scenario: Deferred MCP success and error
- **WHEN** both clients submit an unresolved MCP call from a prior request with current matching server configuration
- **THEN** unary and streaming result-only success/error responses SHALL work without repeating the call or retaining a server session

### Requirement: Private observation and independent acceptance
Authenticated command tests SHALL prove selected-provider native forwarding, aliases, MCP continuation and one canonical logical generation per HTTP request in both modes/clients. Tool metadata, names, IDs, URLs and tokens SHALL remain absent from metadata-only exports, logs and metric labels. Caller-owned request bodies remain sensitive. Separate MCP request goldens SHALL come from the registered client; deterministic fakes SHALL NOT be represented as provider recordings.

#### Scenario: Credential-bearing authenticated round trip
- **WHEN** MCP options and results pass through the real command and native fake endpoint
- **THEN** only the selected provider transport SHALL see the token/URL and normalized outputs, safe failures and operational telemetry SHALL omit them

#### Scenario: Deployment evidence boundary
- **WHEN** deterministic validation passes
- **THEN** documentation SHALL still identify live MCP egress and production activation as unverified pending explicit operational review
