## REMOVED Requirements

### Requirement: Anthropic streaming safeguard verdict lifecycle

**Reason**: The requirement reset the verdict for each message. Upstream keeps the last non-null verdict for the whole stream, and the conformance suite now compares provider parts with upstream.

**Migration**: Replaced by "Anthropic streaming safeguard verdict is stream-level". A later message's finish may carry a verdict from an earlier message.

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Anthropic streaming safeguard error and timing boundary

Malformed present verdict arrays SHALL follow the existing stream-error path at the offending `message_delta`, not a successful finish with malformed metadata. Safeguard handling SHALL NOT change the finish-event timing defined by `anthropic-stream-finish-lifecycle`.

#### Scenario: Anthropic streaming safeguard error and timing boundary

- **WHEN** a message delta contains malformed safeguard_results
- **THEN** the existing error part SHALL be emitted without successful malformed metadata, no `PartFinish` SHALL follow for that message, and the finish-event timing SHALL remain that of `anthropic-stream-finish-lifecycle`
