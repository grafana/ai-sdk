# gateway-unary-function-tools Specification

## Purpose

Define the bounded, stateless unary function-tool subset supported by the Gateway, including both clients, native provider conversion, effect rejection on fallback routes and metadata-only logical observation.

## Requirements

### Requirement: Strict unary function definitions and choice
Unary mapping SHALL support function names, object schemas, examples, optional descriptions, presence-aware strict, namespaced function-tool options and auto/none/required/named tool choice. Empty and absent descriptions SHALL normalize to no description; strict absence, false and true SHALL remain distinct. Complete schema validation SHALL reject inactive fields before resolution. Reserved host namespaces SHALL not reach providers.

#### Scenario: Strict false and empty description
- **WHEN** a unary definition contains strict false, empty description, empty schema and empty examples
- **THEN** strict false and selected empty schema SHALL survive and description SHALL normalize consistently in both clients and provider mapping

#### Scenario: Mixed function and provider fields
- **WHEN** a function definition also contains provider-tool id or args
- **THEN** validation SHALL fail before resolution and invocation

### Requirement: Unary function history and selected results
The mapper SHALL support assistant tool calls and tool-role results with text, json, error-text, error-json or text-only content output. It SHALL preserve required empty text, empty content arrays and selected JSON null. It SHALL reject inactive fields and leave approvals, execution-denied, provider-executed behavior, file/custom content and non-empty nested result options unsupported.

#### Scenario: Empty selected result
- **WHEN** continuation includes a text result with empty value or a JSON result with null
- **THEN** the selected result arm and value SHALL reach the provider without omission or substitution

#### Scenario: Deferred result family
- **WHEN** a schema-valid result contains an execution-denied or file-content arm
- **THEN** it SHALL fail as a fixed unsupported capability before resolution or invocation

### Requirement: Bounded private unary tool calls
Unary output SHALL preserve ordered text and client-executed function tool calls using explicit private DTOs with toolCallId, toolName and string input. Provider output containing providerExecuted true or dynamic true SHALL be rejected with the existing fixed internal-error document before HTTP 200. False or absent markers MAY normalize to disabled. The encoder SHALL NOT strip an enabled execution marker and forward the call as client-executed. Provider-tool results and enabled preliminary behavior SHALL remain outside this supported unary output union. New strings/cardinality SHALL participate in preflight and final encoding bounds. Arbitrary provider metadata and backend identity SHALL not leak.

#### Scenario: Enabled execution marker cannot become a client call
- **WHEN** a provider unary result contains a tool call with providerExecuted true or dynamic true, including alongside valid text
- **THEN** the entire response SHALL be the fixed internal-error document before HTTP 200, with no partial success content
- **AND** both registered Vercel and Go client scenarios SHALL observe an error and execute zero local tools

#### Scenario: Disabled markers preserve client ownership
- **WHEN** a supported function call has absent or false providerExecuted and dynamic markers
- **THEN** it SHALL remain client-executed and its disabled markers MAY be omitted without changing ownership

#### Scenario: Tool call consumed by both clients
- **WHEN** a provider produces a supported function call with a tool-calls finish
- **THEN** both clients SHALL receive semantically equivalent call IDs, names, inputs, usage and finish through the real handler

#### Scenario: Oversized tool input
- **WHEN** a provider output cannot fit the configured unary budget
- **THEN** no partial HTTP 200 SHALL be committed and the fixed safe error SHALL be returned

### Requirement: Unary function tools remain direct and stateless
The Gateway SHALL not execute application functions or persist tool-loop state. A direct route SHALL accept supported unary tool requests. Streaming tool requests SHALL remain unsupported until WP12. If WP9 is present, fallback-configured routes SHALL reject definitions, choices or tool history before physical invocation.

#### Scenario: Two-call unary continuation
- **WHEN** an application executes a returned tool locally and sends the call plus result in a second unary request
- **THEN** the real handler SHALL process two independent requests and the application SHALL receive final text

#### Scenario: Fallback route receives tool history
- **WHEN** a supported unary tool continuation targets a fallback-configured route
- **THEN** no candidate SHALL execute and the response SHALL be a fixed safe unsupported-request error

### Requirement: Unary tools extend logical observation without content capture
Supported unary tool calls SHALL use the single WP8 logical chain and finalize one generation per HTTP invocation with canonical identity, approved context, usage, normalized finish, timing and safe error state. Gateway metadata-only exports, logs and metrics SHALL omit tool definitions, names, IDs, schemas, inputs/results, options, private backend identity and raw errors. Reusable observation mapping SHALL remain provider-domain based.

#### Scenario: Private tool payload markers
- **WHEN** a two-call unary round trip embeds hostile markers in tool data and provider metadata
- **THEN** two canonical logical generations SHALL finish and exported records/logs/metrics SHALL contain none of those markers

### Requirement: Unary client and provider acceptance evidence
Exact registered Vercel and Go clients SHALL complete authenticated unary tool round trips through the production handler. Native request tests SHALL verify schemas, examples, strict presence and choices. Apache modules SHALL remain independent of the Gateway module, and fixture provenance SHALL be preserved.

#### Scenario: Capability verification
- **WHEN** WP11 verification runs
- **THEN** differential HTTP semantics, native request assertions, unsupported mode/family rejection, privacy and response bounds SHALL pass without claiming invented provider payloads as recorded evidence
