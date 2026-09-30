## MODIFIED Requirements

### Requirement: Selected-backend provider options
After resolution, provider options SHALL follow reviewed native consumed scopes and concrete restriction dispositions, not be silently removed solely because a current allowlist omitted them. Namespaces actually ignored by the selected backend MAY be omitted without request failure; supported consumed omissions SHALL be corrected or explicitly handed off. Existing protected credential/host/role/union/transport/effect fields SHALL remain refused. Generic zero policy SHALL remain an explicit unreviewed capability boundary, not a claim that its model consumes no options.

On configured direct Anthropic routes, caller options on already-supported assistant function-tool-call history SHALL reach native conversion with registered consumed semantics, including direct and both code_execution variants with required toolId. The policy SHALL NOT impose a competing exact-key/direct-only caller schema or enable providerExecuted/MCP effects. Current supported other fields SHALL retain values at their reviewed scopes. If heterogeneous fallback cannot safely forward active options a candidate consumes, it SHALL reject before invocation rather than silently strip them; all-candidate irrelevant options and supported semantic empties retain their existing behavior.

#### Scenario: Caller history reaches Anthropic
- **WHEN** either registered client supplies ordinary assistant function-call history with a supported Anthropic caller
- **THEN** the direct native request SHALL preserve tool_use.caller without a prior response or future tool helper
- **AND** caller data SHALL not bypass execution/credential/role/union/MCP guards

#### Scenario: Consumed fallback option has no safe route
- **WHEN** an active option is consumed by a candidate but lacks supported common routing
- **THEN** no physical candidate SHALL run and a fixed nonretryable unsupported-request response SHALL result rather than silent option loss

### Requirement: Fixed privacy-safe errors
Host/internal/resolver/panic/protocol/transport-only failures SHALL retain fixed safe documents. Trusted configured direct provider structured failures SHALL use gateway-caller-response-policy's bounded minimal diagnostics/status/retry mapping through existing paths. Unsupported/unreviewed/ambiguous sources SHALL retain safe diagnostics and explicit gaps. Whole bodies/headers/URLs/requests/causes SHALL not be serialized. Existing public error API SHALL remain; native detail access SHALL use existing registered param/cause/body.

#### Scenario: Direct provider API failure
- **WHEN** DoGenerate returns a reviewed direct structured provider failure
- **THEN** only bounded actionable diagnostics SHALL survive with protected sources excluded

#### Scenario: Unknown public model
- **WHEN** resolution reports an unknown public route
- **THEN** the fixed model-not-found document SHALL remain distinct from a native provider model failure

### Requirement: Minimal unary success response
Success SHALL preserve ordered supported text/function-call/source/reasoning/reasoning-file content, finish reason, validated usage, warning values and supplied registered response id/modelId/timestamp. Warnings SHALL be a non-null registered union/order array with required empty strings, optional-empty Go normalization and 4096-byte string plus aggregate/complete-output bounds. Native request/response headers/body and invented topology SHALL remain excluded; canonical route identity SHALL not replace supplied actual identity. Pinned clients SHALL replace typed unary response with Gateway transport, leaving native identity available only in bounded raw response body.

This foundation SHALL NOT add providerMetadata placements or change current bounded source/reasoning metadata transport. #280 owns all future metadata server/client/schema transport and actual-output continuation; those are explicit handoffs rather than foundation success gates. Required empty text/reasoning/file values, current execution unions and usage.raw object/absent/1 MiB behavior SHALL remain unchanged. Invalid output SHALL fail safely before HTTP 200.

#### Scenario: Supported warnings and actual identity
- **WHEN** valid supported output includes warnings and actual registered identity
- **THEN** those values SHALL survive without canonical substitution or native transport
- **AND** typed unary overwrite/raw-body availability SHALL be asserted through both clients

#### Scenario: Foundation metadata boundary
- **WHEN** a foundation response contains current supported metadata or unimplemented future placements
- **THEN** current transport SHALL remain unchanged and its explicit gap SHALL be handed to #280, not claimed as delivered ordinary metadata parity

### Requirement: Bounded preflight and standard success encoding
Before scanning/parsing/encoding, overflow-safe preflight SHALL bound cardinality and aggregate content/current metadata/warning/identity/finish/raw-usage bytes. Raw usage SHALL remain capped at 1,048,576 original bytes and the unary limit, validated as one UTF-8 JSON object only after size preflight. Private explicit DTOs SHALL use standard JSON and final complete-byte checks before commitment; provider-domain marshalers SHALL not control wire output. Valid raw surrogate escapes SHALL remain preserved and worst-case escaping/copies bounded by the containing limit.

#### Scenario: Raw bytes or escaped encoding exceed bounds
- **WHEN** provider values exceed preflight or complete encoded limits
- **THEN** no partial HTTP 200 SHALL be committed and the fixed adaptation failure SHALL remain

### Requirement: Compatibility evidence
Current registered-client request goldens SHALL replay unchanged. Exact-pinned TS and independent Go handler/command tests SHALL prove delivered warnings/identity/source display and caller request semantics; raw tests SHALL own strict schema/security/bytes/lifecycle proof. #280 actual-output metadata continuation and #201 full authentic matrix SHALL remain later explicit acceptance, not foundation requirements or duplicated harnesses.

#### Scenario: Registered unary client
- **WHEN** the pinned client consumes foundation output
- **THEN** content/usage/finish and server-before-client warnings SHALL survive with client-owned transport and typed identity overwrite accurately recorded
