# gateway-unary-function-tools Specification

## Purpose

Define the bounded, stateless unary function-tool subset supported by the Gateway, including both clients, native provider conversion, configured fallback and metadata-only logical observation.

## Requirements

### Requirement: Strict unary function definitions and choice
Unary mapping SHALL support function names, object schemas, examples, optional descriptions, presence-aware strict, namespaced function-tool options and auto/none/required/named tool choice. Empty and absent descriptions SHALL normalize to no description; strict absence, false and true SHALL remain distinct. Complete schema validation SHALL reject inactive fields before resolution. Reserved host namespaces SHALL not reach providers.

#### Scenario: Strict false and empty description
- **WHEN** a unary definition contains strict false, empty description, empty schema and empty examples
- **THEN** strict false, selected empty schema and the explicitly empty examples array SHALL survive while description normalizes consistently in both clients and provider mapping

#### Scenario: Independent explicit-empty Anthropic options
- **WHEN** a unary function definition supplies either `inputExamples: []` or Anthropic `allowedCallers: []` without the other field
- **THEN** the selected empty array SHALL reach the native Anthropic request and independently select the `advanced-tool-use-2025-11-20` beta
- **AND** in the absence of another beta source, omitting both fields SHALL omit both native arrays and SHALL NOT select that beta

#### Scenario: Mixed function and provider fields
- **WHEN** a function definition also contains provider-tool id or args
- **THEN** validation SHALL fail before resolution and invocation

### Requirement: Unary function history and selected results

The mapper SHALL support assistant tool calls and tool-role results with text, json, error-text, error-json or text/file content output. It SHALL also support provider-executed assistant calls and registered assistant-side basic tool results under gateway-provider-tools. It SHALL preserve required empty text, empty content arrays, selected JSON null, and file-data selection and filename presence as defined by gateway-file-inputs.

#### Scenario: Empty selected result
- **WHEN** continuation includes a text result with empty value or a JSON result with null
- **THEN** the selected result arm and value SHALL reach the provider without omission or substitution

#### Scenario: Deferred result family
- **WHEN** a schema-valid result contains an execution-denied or custom-content arm
- **THEN** it SHALL fail as a fixed unsupported capability before resolution or invocation

#### Scenario: File-bearing tool continuation
- **WHEN** supported tool-role history includes ordered text/file result content with empty selected file text/data, an explicit empty filename, and ordinary file-entry provider options
- **THEN** mapping SHALL retain those values and scopes while permitting assistant provider-executed history under gateway-provider-tools without enabling output-level result options

#### Scenario: Provider result in assistant history
- **WHEN** continuation contains a provider-executed call and matching basic result inline in assistant content
- **THEN** the mapper SHALL preserve their order and ownership without converting the result into a client-owned tool message

### Requirement: Unary result option scopes and deferred families

Registered file-content provider options SHALL be supported at the file-entry scope. The mapper SHALL reject inactive fields and leave approvals, execution-denied, custom content, and non-empty output-level or non-file nested result options unsupported. Tool-call/result part provider options SHALL follow gateway-provider-tools continuation rules.

#### Scenario: Unary result option scopes and deferred families
- **WHEN** tool-role file content carries file-entry options and output-level options
- **THEN** file-entry options SHALL be supported while non-empty output-level options SHALL remain unsupported before invocation

### Requirement: Bounded private unary tool calls

Unary output SHALL preserve ordered text, client-executed and provider-executed calls, and provider tool results using explicit private DTOs. Calls SHALL retain toolCallId, toolName, string input and enabled providerExecuted/dynamic markers. Results SHALL retain toolCallId, toolName, non-null JSON result, error status and registered dynamic/preliminary markers.

#### Scenario: Enabled execution marker cannot become a client call
- **WHEN** a supported provider unary result contains a tool call with providerExecuted true or dynamic true, including alongside valid text
- **THEN** enabled markers SHALL survive to both clients
- **AND** a provider-executed call SHALL execute zero local tools

#### Scenario: Disabled markers preserve client ownership
- **WHEN** a supported function call has absent or false providerExecuted and dynamic markers
- **THEN** it SHALL remain client-executed and disabled markers SHALL normalize without changing ownership

#### Scenario: Tool call consumed by both clients
- **WHEN** a provider produces a supported function call with a tool-calls finish
- **THEN** both clients SHALL receive semantically equivalent call IDs, names, inputs, supported call metadata, usage and finish through the real handler

#### Scenario: Oversized tool input
- **WHEN** a provider output cannot fit the configured unary budget
- **THEN** no partial HTTP 200 SHALL be committed and the fixed safe error SHALL be returned

#### Scenario: Actual returned caller continues natively
- **WHEN** each client's first native Anthropic tool-call response supplies caller metadata on a supported local function call and its actual assembled response history is reused
- **THEN** the second native assistant tool_use request SHALL contain that caller without the test injecting history metadata, and the Gateway SHALL execute no local function

#### Scenario: Provider result is consumed
- **WHEN** a unary response contains a provider call and matching non-null JSON result with supported metadata
- **THEN** both clients SHALL preserve ordered content, result semantics and supported metadata without a result-level providerExecuted wire field

#### Scenario: Deferred unary completion
- **WHEN** a request includes an unresolved provider-owned call in assistant history and the unary output contains its matching success or error result without repeating the call
- **THEN** both clients SHALL receive that result, while unknown, mismatched or already-completed historical matches SHALL fail before HTTP 200

#### Scenario: Unsupported content mixed with valid calls
- **WHEN** a unary result contains supported calls plus a deferred output family
- **THEN** the complete response SHALL fail safely before HTTP 200 without partial success

### Requirement: Unary call ownership and deferred result correlation

Results SHALL correlate with current-response calls or eligible unresolved provider calls in supplied history under gateway-provider-tools; a repeated call in the current response SHALL NOT be required. A complete provider-owned call SHALL be allowed to return without a result. Omitted and false markers SHALL retain disabled/final semantics without discarding presence on registered arms. The encoder SHALL NOT strip an enabled execution marker and forward the call as client-executed.

#### Scenario: Unary call ownership and deferred result correlation
- **WHEN** history contains an unresolved provider-owned call and output supplies only its result
- **THEN** correlation SHALL accept the historical match without repeated call and SHALL preserve disabled/final marker semantics

### Requirement: Unary tool metadata identity and output bounds

Tool metadata SHALL retain opaque namespace objects and omitted/empty presence under gateway-provider-metadata. This SHALL NOT promote provider metadata into routing authority or replace catalog identity. All added strings, result/metadata bytes and cardinality SHALL participate in preflight and final encoding bounds; unsupported or invalid output SHALL fail before HTTP 200.

#### Scenario: Unary tool metadata identity and output bounds
- **WHEN** a supported call contains an unknown metadata object and oversized input
- **THEN** metadata SHALL remain opaque without routing authority, and the oversized complete response SHALL fail before HTTP 200

### Requirement: Unary function tools remain direct and stateless

The Gateway SHALL NOT execute application functions or persist tool-loop state. Direct and configured-fallback routes SHALL accept the existing supported unary function definitions, choices and history/results through the same strict mapper. Configured candidates SHALL receive the same mapped request without tool/choice/history-specific admission guards, namespace translation or candidate intersections.

#### Scenario: Two-call unary continuation
- **WHEN** an application executes a returned tool locally and sends the call plus result in a second unary request through a direct or configured-fallback route
- **THEN** the real handler SHALL process two independent requests and the application SHALL receive final text
- **AND** the deterministic function SHALL execute once for its returned call with no Gateway executor or persisted workflow state

#### Scenario: Fallback route receives tool history
- **WHEN** a supported unary tool continuation targets a fallback-configured route and the primary fails eligibly before success
- **THEN** the next configured candidate SHALL receive the same supported definitions, history/results, choices and scoped options without omission or reinterpretation

#### Scenario: Fallback route receives text-only automatic choice
- **WHEN** an otherwise supported text request contains automatic choice with no tool name and absent or empty tools
- **THEN** the original automatic choice SHALL reach each attempted physical candidate unchanged

#### Scenario: All supported choices remain eligible
- **WHEN** a schema-valid supported unary request supplies auto, none, required or named function choice
- **THEN** a configured-fallback route SHALL NOT reject it merely because of the choice and every attempted candidate SHALL receive the original choice

#### Scenario: Selected unary call is not replayed
- **WHEN** a candidate returns a successful unary function call and the strict response encoder rejects an accompanying unsupported output or exceeds its response budget
- **THEN** the existing safe response failure SHALL apply without invoking a later candidate or executing a consumer function in the Gateway

### Requirement: Unary fallback success boundary and protected capabilities

A successful unary result SHALL select the candidate; later response adaptation failure SHALL NOT restart fallback. Eligible pre-success failures, noneligible errors and cancellation SHALL follow the reusable fallback contract. Unsupported codecs and concrete protocol/credential/account/execution bypass protections SHALL remain effective. Streaming function support SHALL follow gateway-streaming-function-tools rather than a unary-only restriction.

#### Scenario: Unary fallback success boundary and protected capabilities
- **WHEN** a candidate returns a successful tool call whose later adaptation fails
- **THEN** fallback SHALL NOT restart, while reusable pre-success eligibility/cancellation and bypass protections SHALL remain effective

### Requirement: Unary tools extend logical observation without content capture
Supported unary tool calls SHALL use the single WP8 logical chain and finalize one generation per HTTP invocation with canonical identity, approved context, usage, normalized finish, timing and safe error state. Gateway metadata-only exports, logs and metrics SHALL omit tool definitions, names, IDs, schemas, inputs/results, options, private backend identity and raw errors. Reusable observation mapping SHALL remain provider-domain based.

#### Scenario: Private tool payload markers
- **WHEN** a two-call unary round trip embeds hostile markers in tool data and provider metadata
- **THEN** two canonical logical generations SHALL finish and exported records/logs/metrics SHALL contain none of those markers

### Requirement: Unary client and provider acceptance evidence

Exact registered Vercel and independent Go clients SHALL complete authenticated unary function round trips through direct and configured-fallback production routes. Native request tests SHALL verify schemas, examples, strict presence, all supported choices, supported continuation and candidate-specific scoped native options. Tests SHALL prove configured order, unchanged failure eligibility and cancellation, consumer-owned execution and one logical observation per request.

#### Scenario: Capability verification
- **WHEN** unary function verification runs
- **THEN** both-client direct/fallback HTTP semantics, native request assertions, unsupported family rejection, privacy and response bounds SHALL pass without claiming invented provider payloads as recorded evidence

### Requirement: Unary tool evidence provenance and module independence

Apache modules SHALL remain independent of the Gateway module, and fixture provenance SHALL be preserved. Synthetic request evidence SHALL NOT claim live backend acceptance, full output-derived continuation or exactly-once provider execution.

#### Scenario: Unary tool evidence provenance and module independence
- **WHEN** synthetic native request assertions pass
- **THEN** they SHALL NOT claim live acceptance, complete output-derived continuation or exactly-once execution, and Apache module independence and fixture provenance SHALL remain intact
