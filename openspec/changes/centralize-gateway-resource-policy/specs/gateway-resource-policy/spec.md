# Spec Delta

## Purpose

Define the few shared resource controls that bound the Gateway plugin across every API adapter and route, give each retained limit one owner, and keep protocol adapters free of local byte budgets.

## ADDED Requirements

### Requirement: Shared route request-body bound
The Gateway SHALL apply one configured request-body byte limit to every plugin API route through its shared route handler, independent of adapter, access mode and payload shape. Access middleware SHALL complete before an adapter decodes the body. The limit SHALL be the existing `max_request_bytes` plugin setting (`GF_PLUGIN_MAX_REQUEST_BYTES`), default 1,048,576 bytes, and SHALL be positive.

#### Scenario: Body at the boundary
- **WHEN** an authorized ProviderWire request body is below or exactly at the configured limit
- **THEN** the adapter SHALL read the complete body and continue validation

#### Scenario: Body exceeds the boundary
- **WHEN** an authorized ProviderWire request body is one byte above the configured limit
- **THEN** the adapter SHALL return its fixed invalid-request document before schema validation, selection or model invocation

#### Scenario: Refused caller with oversized body
- **WHEN** a caller refused by access middleware sends an oversized body
- **THEN** the permission failure SHALL be returned without the adapter decoding the body

#### Scenario: Invalid setting
- **WHEN** `max_request_bytes` is zero, negative or cannot be represented safely
- **THEN** plugin startup SHALL fail before serving routes, naming the setting without its value

### Requirement: Route bound composes with every adapter
The route request-body bound SHALL be owned by the Gateway route handler rather than any protocol adapter, so a handler mounted on a plugin route inherits it without adapter-specific configuration. It SHALL apply equally to configured-account and BYOK handlers.

#### Scenario: Non-ProviderWire handler shares the bound
- **WHEN** a test mounts a non-ProviderWire body-reading handler behind the shared route handler and sends bodies at and one byte above the limit
- **THEN** the handler SHALL observe the complete in-limit body and an overflow read error for the oversized body, without any adapter-local limit

#### Scenario: Configured and BYOK access
- **WHEN** the same oversized body reaches the language-model route as a configured-account caller and as a BYOK caller
- **THEN** both SHALL apply the same configured bound

### Requirement: Host-owned transport bounds are documented
Bounds enforced by the plugin host before the Gateway runs SHALL remain host-owned and SHALL NOT be duplicated by the Gateway. These are the platform front door, the plugin route RPC message size and the platform route deadline. Gateway documentation SHALL identify them and state that the route body bound is a check that rejects early, before parsing, not the memory bound for the received request.

#### Scenario: Operator reads host bounds
- **WHEN** an operator consults the Gateway README limits section
- **THEN** it SHALL name the platform route deadline, the plugin route message bound and the front-door limits as host-owned, with the Gateway route bound described as an early-rejection check below them

### Requirement: Shared outbound transport bounds
Every Gateway-constructed native provider and BYOK account SHALL use the shared outbound transport. It SHALL reject redirects, bound the time to receive response headers (10s), and bound the cumulative bytes of each HTTP response body delivered to its consumer (16,777,216 bytes, below the Anthropic SDK line-scanner limit). Both bounds SHALL be fixed shared values for every provider type, not adapter or provider settings.

#### Scenario: Response body at and above the bound
- **WHEN** a native provider response body is exactly at or one byte above the provider response byte bound
- **THEN** the at-limit body SHALL be readable in full and the over-limit read SHALL fail with a response-too-large transport error without returning further bytes

#### Scenario: Compressed response expands past the bound
- **WHEN** a provider returns a transparently decompressed body whose compressed size is below the bound and whose decompressed size exceeds it
- **THEN** the read SHALL fail at the decompressed limit

#### Scenario: Each attempt owns its response bound
- **WHEN** a configured fallback or BYOK request makes several physical attempts
- **THEN** each attempt's HTTP response SHALL be bounded independently, and the total number of attempts SHALL remain bounded by candidate or account policy and the shared execution deadline

#### Scenario: Provider ignores header deadline
- **WHEN** a provider does not send response headers within the provider response-header timeout
- **THEN** the attempt SHALL fail as a timeout transport error

### Requirement: Honest outcomes at shared boundaries
Shared-boundary failures SHALL use the adapter's existing classification and lifecycle. An upstream overflow before commitment SHALL produce a safe non-2xx error. After stream commitment it SHALL produce ordered SSE error and terminal behavior. No boundary SHALL truncate JSON, manufacture a successful completion or replay a committed candidate.

#### Scenario: Unary upstream overflow
- **WHEN** a unary provider response exceeds the outbound bound
- **THEN** the caller SHALL receive a classified safe error and no HTTP 200 document

#### Scenario: Streaming upstream overflow after commitment
- **WHEN** a committed provider stream exceeds the outbound bound
- **THEN** the caller SHALL receive the existing ordered SSE error and terminal behavior, no synthetic finish, and no later candidate

### Requirement: No adapter-local output byte budgets
Protocol adapters SHALL NOT impose response, SSE-frame, per-field, string, metadata, warning, raw-usage, cardinality or provider-part-count budgets on provider output. Output size SHALL follow from the bounded request, bounded outbound responses and execution deadlines. Adapters SHALL encode each response or event once, under standard JSON encoding.

#### Scenario: Large valid provider output
- **WHEN** a provider returns valid output whose metadata, raw usage, warnings or sources exceed the former adapter budgets while its native response stays within the outbound bound
- **THEN** the adapter SHALL return or stream the complete output

#### Scenario: Correctness validation remains
- **WHEN** provider output contains invalid UTF-8, unregistered discriminators, non-object metadata namespaces, non-object raw usage or invalid lifecycle
- **THEN** the adapter SHALL fail it through its existing safe error path, independently of size

### Requirement: Execution deadlines remain shared runtime policy
The total model duration (`max_model_duration`) and the fixed stream idle and drain durations SHALL remain positive validated values, owned by the shared execution runtime under gateway-execution-runtime, that bound selection, attempts, stream consumption and post-terminal cleanup for every adapter. These deadlines SHALL NOT be replaced by byte or count budgets. Documentation SHALL state that the platform route deadline can end a request before the model duration.

#### Scenario: Continuously ready provider after termination
- **WHEN** a canceled provider channel remains permanently ready
- **THEN** handler-owned drain SHALL stop within the drain duration

#### Scenario: Platform deadline is shorter
- **WHEN** the platform route deadline is shorter than `max_model_duration`
- **THEN** the documentation SHALL state that the platform deadline ends the request first

### Requirement: Documented limit ledger and trade-offs
The Gateway README SHALL list each retained limit with its one owner (plugin host, route handler, outbound transport, execution runtime, configuration loader, discovery configuration, telemetry sink, execution attempt policy or independent client) and SHALL identify each removed adapter restriction. It SHALL document the boundary trade-offs.

#### Scenario: Operator reads the ledger
- **WHEN** an operator consults the Gateway README
- **THEN** each retained limit SHALL name its setting or constant, default and owner, and the removed ProviderWire limits SHALL be listed with their replacement

#### Scenario: Trade-offs are explicit
- **WHEN** the README describes resource boundaries
- **THEN** it SHALL state the following:
- admission and rate limits do not bound one buffered response
- the route body limit does not bound upstream responses
- output size is proportional to the outbound bound plus encoding expansion
- a response-writer limit is not used, because it cannot prevent completed allocations and could terminate committed streams
- aggregate connection and request admission belongs to the plugin host
