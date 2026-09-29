## ADDED Requirements

### Requirement: Opt-in Anthropic safeguard request projection
The Anthropic provider SHALL accept model-level `AnthropicOptions.Safeguards` on `CallOptions.ProviderOptions["anthropic"]`, both as typed options and as JSON-round-tripped `RawProviderOption`. Each configured entry SHALL have `type: "dangerous_tool_use"` and MAY contain a JSON object `classifierContext` with arbitrary JSON property values. For both `DoGenerate` and `DoStream`, a nonempty option SHALL produce the request-body array `safeguards`, preserving entry order, each `type`, and any present `classifierContext` as `classifier_context` without changing its JSON values. The provider SHALL add `dangerous-tool-use-2026-09-03` to `anthropic-beta` exactly once, including when the caller also explicitly supplies that beta. An absent or empty safeguards option SHALL NOT add the field or beta automatically. Explicit JSON `safeguards: null` SHALL be rejected before HTTP, matching the registered upstream optional array schema. Explicit caller-supplied `Betas` SHALL continue to be honored independently.

#### Scenario: Omitted safeguards
- **WHEN** a unary or streaming call omits `safeguards` from the Anthropic provider options
- **THEN** its serialized HTTP body SHALL omit `safeguards` and its header SHALL NOT gain the safeguard beta automatically

#### Scenario: Empty safeguards
- **WHEN** a unary or streaming call provides `safeguards: []`
- **THEN** its serialized HTTP body SHALL omit `safeguards` and its header SHALL NOT gain the safeguard beta automatically

#### Scenario: Configured safeguard with context
- **WHEN** a unary or streaming call provides `safeguards: [{type: "dangerous_tool_use", classifierContext: {v: 1, permission_mode: "auto"}}]`
- **THEN** the serialized HTTP body SHALL contain `"safeguards":[{"type":"dangerous_tool_use","classifier_context":{"v":1,"permission_mode":"auto"}}]`
- **AND** the `anthropic-beta` header SHALL include `dangerous-tool-use-2026-09-03` exactly once

#### Scenario: Configured safeguard with empty or absent context
- **WHEN** an entry has an explicitly empty classifier context object or omits the optional context
- **THEN** the HTTP body SHALL preserve `{}` as `classifier_context` in the former case and omit `classifier_context` in the latter case

#### Scenario: Typed and round-tripped input
- **WHEN** typed Anthropic safeguard provider options are serialized through `ProviderOptions` and resolved again, including an explicitly empty `classifierContext: {}` and JSON numbers, booleans, arrays, nested objects and null property values within a classifier context
- **THEN** unary and streaming HTTP bodies and beta headers SHALL match the equivalent directly typed call, preserving the explicitly empty object

#### Scenario: Invalid safeguard option
- **WHEN** raw provider options contain explicit `safeguards: null` or a configured `classifierContext: null`, typed options contain a non-nil classifier-context pointer to a nil map, or a configured safeguard has a missing or unsupported type, a non-object classifier context, or malformed JSON values
- **THEN** `DoGenerate` or `DoStream` SHALL fail before issuing any HTTP request and SHALL NOT silently send a different classifier

### Requirement: Anthropic unary safeguard verdict metadata
When an Anthropic unary response carries a valid non-null `safeguard_results` array, the provider SHALL expose it only at `GenerateResult.ProviderMetadata["anthropic"].safeguardResults`, preserving the response's permitted nested snake_case keys, string values and tool-call-id keys. Each entry SHALL contain string `type` and object `status` with string `type`; optional/null `status.tool_uses` SHALL hold a map of tool-call ids to entries with string `type` and optional/null string `outcome` and `explanation`. Other response fields and extra nested safeguard properties SHALL NOT be copied into this metadata field. If `safeguard_results` is missing or null, the key SHALL be omitted; if it is a valid empty array, the key SHALL be present as `[]`. A malformed present value SHALL yield a contextual conversion error, without embedding the raw verdict payload.

#### Scenario: Tool-call verdict retains wire shape
- **WHEN** a unary response contains `safeguard_results` with an available status and a tool-call-id `toolu_01` whose verdict has `type: "evaluated"`, `outcome: "flagged"`, and `explanation: "[Data Exfiltration]"`
- **THEN** `safeguardResults` SHALL contain that entry with `status.tool_uses.toolu_01` and its snake_case keys intact
- **AND** unrelated top-level response and extra nested safeguard fields SHALL be excluded

#### Scenario: No unary verdict
- **WHEN** a unary response omits `safeguard_results` or explicitly supplies null
- **THEN** its Anthropic provider metadata SHALL omit `safeguardResults`

#### Scenario: Empty unary verdict array
- **WHEN** a unary response contains `safeguard_results: []`
- **THEN** its Anthropic provider metadata SHALL contain `safeguardResults: []`

#### Scenario: Malformed unary verdict
- **WHEN** `safeguard_results` is present but is not an array of entries matching the defined field shapes
- **THEN** conversion SHALL return an error and SHALL NOT expose the malformed raw payload as provider metadata

### Requirement: Anthropic streaming safeguard verdict lifecycle
The Anthropic stream adapter SHALL read verdicts from `message_delta.delta.safeguard_results` and retain the last non-null valid array in the current message, overwriting it with a later non-null array including `[]` and ignoring later null or missing fields. It SHALL reset that state for a new message and SHALL omit the `safeguardResults` key when no delta in the message supplies a non-null array. The last `PartFinish` for a message and the completed stream step SHALL carry the final retained result in `ProviderMetadata["anthropic"].safeguardResults`; unrelated raw fields SHALL NOT leak. A malformed present array SHALL follow the existing stream-error path rather than produce a successful finish with malformed metadata. This requirement does not change the adapter's existing finish-event timing.

#### Scenario: Null then verdict then null
- **WHEN** successive message deltas contain null, a valid verdict array keyed by `toolu_01`, then null, followed by `message_stop`
- **THEN** the final `PartFinish` and completed stream step SHALL contain that verdict array under `safeguardResults` with nested wire shape preserved

#### Scenario: Later valid verdict replaces earlier verdict
- **WHEN** two deltas contain different non-null arrays, including when the later one is `[]`
- **THEN** the final `PartFinish` SHALL contain the later array

#### Scenario: No streaming verdict
- **WHEN** deltas omit `safeguard_results` or contain only null
- **THEN** all produced Anthropic finish metadata SHALL omit `safeguardResults`

#### Scenario: Verdict only in message start
- **WHEN** `message_start` contains `safeguard_results` but the following deltas omit or null that field
- **THEN** all produced Anthropic finish metadata SHALL omit `safeguardResults`

#### Scenario: Reset on new message
- **WHEN** a new `message_start` follows a message with a verdict
- **THEN** a finish for the new message SHALL NOT inherit the prior message's verdict

#### Scenario: Malformed streaming verdict and transport failure
- **WHEN** a delta contains a malformed non-null `safeguard_results` value
- **THEN** the stream SHALL emit the existing error part without a successful metadata projection for that delta
- **AND** a transport failure SHALL remain a transport error, without synthesizing safeguard metadata or silently retrying without safeguards
