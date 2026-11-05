## ADDED Requirements

### Requirement: Tool input preserves arrays before the first negative digit

For static and dynamic tools, partial JSON input whose first array element is an incomplete minus sign SHALL retain a valid empty array in Input. Subsequent digits SHALL update that same input with the negative number, preserving surrounding object fields.

#### Scenario: Progressive input pauses at an initial minus

- **WHEN** a tool receives input deltas `{"series":"temp","values":[`, `-` and `3,`
- **THEN** its first two input snapshots SHALL both contain `{"series":"temp","values":[]}`
- **AND** its third input snapshot SHALL contain `{"series":"temp","values":[-3]}`
- **AND** each snapshot SHALL remain in input-streaming state and serialize as valid JSON
