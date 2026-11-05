## ADDED Requirements

### Requirement: Partial JSON tolerates an initial array minus

Structured output partial parsing SHALL repair an array whose first value consists only of a minus sign as an empty array, including nested arrays and whitespace before the minus. Once a digit arrives, partial parsing SHALL retain the negative number. Whitespace alone after an opening array bracket SHALL NOT become part of the repaired value.

#### Scenario: First output chunk ends at an array minus

- **WHEN** object output streams `{"series":"temp","values":[-`, then `3,`, then `4]}`
- **THEN** the partial output stream SHALL emit `{"series":"temp","values":[]}`, `{"series":"temp","values":[-3]}` and `{"series":"temp","values":[-3,4]}` in order

#### Scenario: Minus arrives after an existing empty array partial

- **WHEN** object output streams `{"series":"temp","values":[`, then `-`, then `3,`
- **THEN** partial parsing SHALL succeed at every boundary
- **AND** the minus boundary SHALL NOT duplicate the already emitted empty-array snapshot
