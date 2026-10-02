## ADDED Requirements

### Requirement: MCP validation at native consumption
Native Anthropic unary and streaming attempts SHALL resolve the same AnthropicOptions consumed by the adapter, then validate the interpreted MCP server names, destinations, tokens and tool configuration under request-local resource bounds. Missing, null and empty server lists SHALL retain their native no-op semantics. Explicit empty tokens, false enabled and empty allowedTools SHALL retain their presence. Go struct field matching and duplicate-field resolution SHALL agree with the actual native conversion; unconsumed extension members SHALL NOT create additional semantic guards. Existing credential, configured destination, tenant and unrelated native authority protections SHALL remain intact.

#### Scenario: Resolved native configuration
- **WHEN** an authenticated request supplies valid MCP configuration, including consumed case variants or duplicate fields
- **THEN** the native request SHALL receive the same resolved ordered definitions and required beta without allowing metadata to change backend credentials or routing

#### Scenario: Invalid consumed destination
- **WHEN** actual native configuration has duplicate or empty names, exceeds resource limits, or uses an insecure URL, embedded credentials or a fragment delimiter
- **THEN** the native consuming boundary SHALL reject it safely before invocation without reflecting token or URL material

### Requirement: Foreign configuration remains opaque
The Gateway codec SHALL transport structurally valid bounded namespace objects without interpreting MCP fields or intersecting candidate option policies. Foreign Anthropic options SHALL be inert when an adapter does not consume them. A compatible adapter's configured namespace SHALL determine which extension options it actually forwards; namespace spelling alone SHALL NOT establish native Anthropic capability.

#### Scenario: Foreign malformed MCP values
- **WHEN** a compatible adapter does not consume an Anthropic namespace containing malformed MCP configuration
- **THEN** ordinary inference SHALL proceed without invoking MCP, changing configured credentials or capturing submitted metadata

### Requirement: Native continuation and opaque response transport
Actual native Anthropic MCP continuation SHALL validate consumed server names against the current resolved configuration. Provider-owned calls SHALL retain the generic ID/name and history correlation contract. Native producer conversion SHALL correlate result IDs to their originating calls. The Gateway codec and independent Go client SHALL preserve complete opaque metadata, including unknown namespaces, caller/item/type extensions and omission versus empty objects. Response metadata SHALL NOT be classified against configured server lists or used as routing, execution, authentication or capture authority. Result metadata SHALL NOT need to repeat a call's server fields.

#### Scenario: Response-derived continuation
- **WHEN** either client reuses an authentic native-fake response's provider-owned MCP call and result with current matching configuration
- **THEN** the native adapter SHALL reproduce MCP tool-use/result blocks using its consumed call context, without requiring codec metadata equality

#### Scenario: Inert local marker
- **WHEN** a client-owned call carries MCP-looking metadata
- **THEN** native conversion SHALL preserve local function-call semantics rather than granting provider execution authority

### Requirement: Existing fallback and lifecycle semantics
MCP SHALL reuse ordinary fallback without direct-only or blanket effect restrictions. Unary success or any first provider part SHALL commit selection; eligible pre-selection failures may advance. Post-selection failures SHALL NOT replay. Billing and remote effects may repeat when an earlier attempt's outcome is unobserved; retryability SHALL NOT imply no effects or exactly-once execution. Developers SHALL choose fallback-safe workflows, application idempotency/deduplication, or no fallback. Generic ownership, ID/name, deferred-result, preview/finalization, cancellation and encoding bounds SHALL remain unchanged without persistent sessions or new provenance machinery.

#### Scenario: Pre-observation failure
- **WHEN** a configured Anthropic candidate fails eligibly before selection with MCP configured
- **THEN** the next candidate may run with the same actual native validation, without a zero-effects guarantee

### Requirement: Independent evidence and capture control
Authenticated command tests SHALL exercise both clients, native projection and response-derived continuation. Returned metadata SHALL NOT authorize operator capture; metadata-only generations, logs and metrics SHALL omit supplied tool data and MCP credentials. Caller-owned request bodies remain sensitive. Synthetic deferred-result tests and native fakes SHALL NOT be represented as recorded provider responses or proof of live hosted MCP egress, deployed readiness or published-module adoption.

#### Scenario: Credential-bearing native round trip
- **WHEN** MCP configuration and responses pass through the real command and native fake
- **THEN** configured native authentication SHALL remain authoritative, public developer data SHALL remain opaque, and metadata-only telemetry SHALL not capture submitted values
