## MODIFIED Requirements

### Requirement: Shared stream output validation
The runtime SHALL validate each provider part before an adapter receives it. The rules are: the start position, response segments, text and reasoning IDs, the tool input, call, preview, result and approval phases (including calls seeded from history), finish preconditions and a registered finish reason. A violation SHALL end the stream with the invalid-output outcome. Each adapter SHALL retain its own representation checks and protocol mappings.

#### Scenario: Start position
- **WHEN** a `stream-start` part arrives after any other part
- **THEN** the wait SHALL return invalid output, and a first `stream-start` SHALL be accepted

#### Scenario: Response segments
- **WHEN** a `response-metadata` or a non-interrupted `finish` arrives while text, reasoning or tool input is open, or any `response-metadata` or `finish` arrives while a tool preview is pending, in the stream or in history
- **THEN** the wait SHALL return invalid output, while an accepted `response-metadata` SHALL open a new segment, at the start of the stream, after content or after a `finish`

#### Scenario: Interrupted finish with open blocks
- **WHEN** a finish with unified reason `interrupted` meets all finish preconditions other than having open text, reasoning or tool-input blocks
- **THEN** ProviderWire SHALL receive it unchanged; the runtime SHALL end the segment and discard active text and reasoning state without synthesizing any parts
- **AND** every open tool input SHALL be abandoned, with its ID still reserved for the whole stream; it SHALL NOT resume or yield a tool call in a later segment
- **AND** extension validation SHALL use the same policy as the adapter for inspection and observation: ProviderWire SHALL receive the interruption unless a response inspector denies it, and native adapter streams SHALL get the invalid-output outcome without passing that finish to a response inspector

#### Scenario: Parts after a finish
- **WHEN** a part other than `response-metadata`, raw or provider error arrives after an accepted `finish`, including a second `finish`
- **THEN** the wait SHALL return invalid output

#### Scenario: Text and reasoning IDs
- **WHEN** a text start has an empty ID, overlaps an active text or reuses an ID within its segment, a text delta or end names an inactive ID, a reasoning start has an empty ID or reuses one within its segment, or a reasoning delta or end names an inactive ID
- **THEN** the wait SHALL return invalid output, while concurrent reasoning blocks with distinct IDs, and text or reasoning IDs reused in a later segment, SHALL be accepted

#### Scenario: Tool phases
- **WHEN** a tool part has an empty ID or a missing tool name, skips or repeats a phase, changes the tool name, reuses a tool ID in any segment, starts input for an ID seeded from history in any phase, carries a forbidden provider-executed, dynamic or preliminary flag, re-emits a history call without the provider-executed flag or a second time, or has an empty, invalid or `null` result
- **THEN** the wait SHALL return invalid output

#### Scenario: Results across segments and from history
- **WHEN** a stream emits the result of a call from an earlier segment, or of a call pending in history, with the same tool name
- **THEN** the part SHALL be accepted

#### Scenario: Approvals
- **WHEN** an approval names a call that is not a provider-executed call awaiting its first result, or one already approved, has an empty approval ID or reuses one anywhere in the stream, is provider-executed, or carries any dynamic or preliminary value, even false
- **THEN** the wait SHALL return invalid output

#### Scenario: Finish preconditions
- **WHEN** a finish carries warnings, lacks a finish reason or usage, or has an unregistered unified finish reason
- **THEN** the wait SHALL return invalid output

#### Scenario: Unknown part type
- **WHEN** a part has a type that is not a LanguageModelV4 stream part
- **THEN** the wait SHALL return invalid output

#### Scenario: Pass-through parts
- **WHEN** an error or raw part arrives, in a segment or after a `finish`
- **THEN** the runtime SHALL deliver it without changing segment, text, reasoning or tool state, and it SHALL still consume the start position, so a later `stream-start` SHALL return invalid output
