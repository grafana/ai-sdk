## ADDED Requirements

### Requirement: Stream part JSON follows the V4 shape

Serializing a stream part SHALL emit `warnings` on `stream-start`, as an empty array when there are none, and SHALL encode a response timestamp as UTC with milliseconds, like JavaScript's `Date#toISOString`.

#### Scenario: Stream start without warnings
- **WHEN** a `stream-start` part with no warnings is serialized
- **THEN** the JSON SHALL be `{"type":"stream-start","warnings":[]}`

#### Scenario: Response timestamp
- **WHEN** a `response-metadata` part with a non-UTC timestamp is serialized
- **THEN** the JSON `timestamp` SHALL be the same instant in UTC with exactly three fractional digits and a `Z` suffix
