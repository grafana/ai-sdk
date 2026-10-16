## Purpose

Define Anthropic's opt-in safeguard request, beta, and verdict metadata behavior for unary and streaming language-model calls.

## Requirements

### Requirement: Opt-in Anthropic safeguard request projection

The provider SHALL accept model-level `AnthropicOptions.Safeguards` under `CallOptions.ProviderOptions["anthropic"]` as typed or JSON-round-tripped `RawProviderOption`. Entries SHALL have type `dangerous_tool_use` and optional object `classifierContext` with arbitrary JSON values. Nonempty options SHALL emit ordered `safeguards` in DoGenerate/DoStream, preserving types and present context as `classifier_context` without changing values.

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

Valid non-null unary `safeguard_results` SHALL appear only at `GenerateResult.ProviderMetadata["anthropic"].safeguardResults`, retaining permitted nested snake_case keys, strings and tool-call-id keys, excluding other response/extra nested fields. Missing/null SHALL omit the key; valid [] SHALL preserve it. Malformed present values SHALL yield contextual conversion errors without raw verdict payloads.

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

### Requirement: Anthropic safeguard opt-in beta and null handling

Nonempty safeguards SHALL add `dangerous-tool-use-2026-09-03` to anthropic-beta exactly once, even when explicitly supplied. Absent/empty safeguards SHALL add neither field nor beta automatically. Explicit JSON safeguards:null SHALL fail before HTTP, matching registered upstream optional-array schema. Explicit caller Betas SHALL be honored independently.

#### Scenario: Anthropic safeguard opt-in beta and null handling

- **WHEN** safeguards are empty with an explicit beta, or null without one
- **THEN** the empty request SHALL omit safeguards but retain the explicit beta; null SHALL be rejected before HTTP

### Requirement: Anthropic safeguard verdict field schema

Verdict entries SHALL contain string type and object status with string type; optional/null status.tool_uses SHALL hold a map of tool-call IDs to entries with string type and optional/null string outcome and explanation. Only these permitted fields SHALL be projected.

#### Scenario: Anthropic safeguard verdict field schema

- **WHEN** a verdict contains a tool_uses entry with type, nullable outcome/explanation, and an extra secret property
- **THEN** projection SHALL preserve the permitted fields and null semantics but exclude the extra property; invalid consumed field types SHALL fail conversion

### Requirement: Anthropic streaming safeguard error and timing boundary

Malformed present verdict arrays SHALL follow the existing stream-error path at the offending `message_delta`, not a successful finish with malformed metadata. Safeguard handling SHALL NOT change the finish-event timing defined by `anthropic-stream-finish-lifecycle`.

#### Scenario: Anthropic streaming safeguard error and timing boundary

- **WHEN** a message delta contains malformed safeguard_results
- **THEN** the existing error part SHALL be emitted without successful malformed metadata, no `PartFinish` SHALL follow for that message, and the finish-event timing SHALL remain that of `anthropic-stream-finish-lifecycle`

### Requirement: Anthropic streaming safeguard verdict is stream-level

The adapter SHALL retain the last non-null valid `message_delta.delta.safeguard_results` for the whole stream, replacing it even with [] and ignoring null/missing, as upstream does; a new message does not reset it. No valid delta SHALL mean no safeguardResults key. The single PartFinish emitted at the message's `message_stop`, and the completed step, SHALL carry the retained array under ProviderMetadata["anthropic"].safeguardResults without unrelated raw fields.

#### Scenario: Null then verdict then null
- **WHEN** successive message deltas contain null, a valid verdict array keyed by `toolu_01`, then null, followed by `message_stop`
- **THEN** the message's only `PartFinish` and the completed stream step SHALL contain that verdict array under `safeguardResults` with nested wire shape preserved

#### Scenario: Later valid verdict replaces earlier verdict
- **WHEN** two deltas contain different non-null arrays, including when the later one is `[]`
- **THEN** the message's `PartFinish` SHALL contain the later array

#### Scenario: No streaming verdict
- **WHEN** deltas omit `safeguard_results` or contain only null
- **THEN** the produced Anthropic finish metadata SHALL omit `safeguardResults`

#### Scenario: Verdict only in message start
- **WHEN** `message_start` contains `safeguard_results` but the following deltas omit or null that field
- **THEN** the produced Anthropic finish metadata SHALL omit `safeguardResults`

#### Scenario: Verdict carries into the next message
- **WHEN** a new `message_start` follows a message with a verdict and the new message's deltas omit or null `safeguard_results`
- **THEN** the finish for the new message SHALL carry the prior message's verdict, as upstream does

#### Scenario: Malformed streaming verdict and transport failure
- **WHEN** a delta contains a malformed non-null `safeguard_results` value
- **THEN** the stream SHALL emit the existing error part without a successful metadata projection for that delta
- **AND** a transport failure SHALL remain a transport error, without synthesizing safeguard metadata or silently retrying without safeguards
