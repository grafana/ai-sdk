## MODIFIED Requirements

### Requirement: Invalid optional field types do not disappear during decoding

Typed tool/approval JSON decoding SHALL reject explicit null and wrong JSON types for supported optional non-null strings, booleans and object metadata before Go nil/zero-value decoding can treat them as absent. Input, Output and approval Descriptor SHALL retain their target-supported arbitrary JSON values, including null. RawInput SHALL be a string in input-streaming state and MAY retain arbitrary legacy JSON in output-error state. Invalid persisted optional fields SHALL NOT reach Agent/provider invocation through normalization to absence.

#### Scenario: Optional null string or boolean is not silently accepted
- **WHEN** persisted tool JSON includes title null, preliminary null or an approval reason of the wrong JSON type
- **THEN** decoding/validation SHALL return a contextual error before provider invocation
- **AND** the invalid field SHALL NOT be normalized into an absent optional value

#### Scenario: Streaming raw input requires a string
- **WHEN** an input-streaming tool part carries RawInput
- **THEN** decoding and Agent validation SHALL retain a JSON string and reject null or non-string values before invocation

#### Scenario: Opaque null remains data
- **WHEN** valid tool JSON includes null Input, Output, RawInput or approval Descriptor in a target-supported state
- **THEN** decoding SHALL retain that null value as data
- **AND** tool schema/state checks SHALL determine subsequent validity at their specified gates
