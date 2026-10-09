## MODIFIED Requirements

### Requirement: Usage aggregation sums nested totals
The `aggregateUsage` function SHALL sum every `InputTokens` and `OutputTokens` field across steps. `InputTokens.Total` and `OutputTokens.Total` SHALL always be set, with an unreported total treated as zero. A breakdown field (`NoCache`, `CacheRead`, `CacheWrite`, `Text`, `Reasoning`) SHALL be nil when no step reported it. Upstream returns `undefined` for an unreported total; Go keeps totals non-nil.

#### Scenario: Aggregate usage across multiple steps
- **WHEN** two steps have `InputTokens.Total` of 100 and 200, and `OutputTokens.Total` of 50 and 75
- **THEN** the aggregated usage SHALL have `InputTokens.Total` of 300 and `OutputTokens.Total` of 125

#### Scenario: Aggregate usage with nil totals
- **WHEN** a step has nil `InputTokens.Total`
- **THEN** it SHALL be treated as zero in the aggregation sum

#### Scenario: Aggregate usage breakdowns
- **WHEN** one step reports `CacheRead` of 800 and another reports `CacheRead` of 1000 and `CacheWrite` of 200
- **THEN** the aggregated usage SHALL have `CacheRead` of 1800 and `CacheWrite` of 200
- **AND** breakdown fields no step reported SHALL be nil
