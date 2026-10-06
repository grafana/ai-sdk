## ADDED Requirements

### Requirement: PartResponseMeta carries provider over the wire

The provider wire SHALL round-trip the `Provider` field on `StreamPart` for `PartResponseMeta` events, alongside the existing `ResponseID` and `ModelID` fields, with no field loss.

#### Scenario: Response-meta provider round-trip
- **WHEN** a `provider.StreamPart{Type: PartResponseMeta, ResponseID: "r1", ModelID: "claude-x", Provider: "anthropic.vertex"}` is encoded and decoded via the SSE helpers
- **THEN** the decoded part SHALL equal the original (using `reflect.DeepEqual`), preserving `Provider == "anthropic.vertex"`

#### Scenario: Empty provider omitted on the wire
- **WHEN** a `StreamPart` with an empty `Provider` is encoded
- **THEN** the `provider` key SHALL be omitted from the JSON (`omitempty`), keeping the wire backward compatible
