## ADDED Requirements

### Requirement: Stream-wide text and reasoning identifiers
StreamText SHALL preserve the first occurrence of each provider text/reasoning
identifier and reserve a unique replacement for later collisions across steps.
Text and reasoning SHALL use independent identifier namespaces. Matching deltas
and end parts SHALL use the reserved start identifier; generated collisions SHALL
receive deterministic numeric suffixes without repeatedly invoking the generator.

#### Scenario: Provider reuses IDs across steps
- **WHEN** multiple steps each emit text or reasoning with the same provider ID
- **THEN** their emitted start identifiers are unique within that content family
- **AND** deltas and ends identify their corresponding starts
- **AND** the frontend assembles separate text/reasoning parts without overwriting prior steps

#### Scenario: Custom generator also collides
- **WHEN** the replacement generator returns an identifier that is already reserved
- **THEN** a numeric suffix is advanced until the identifier is unique
