## MODIFIED Requirements

### Requirement: Normalized streaming usage aggregation

Stream-summary middleware SHALL share normalized usage aggregation over every part with non-nil `Usage`, regardless of type. A `PartFinish` SHALL close a response segment, whose usage takes each normalized counter from the finish when it reports it, and otherwise the greatest value observed in that segment. Once a finish was observed, the aggregate SHALL be the last closed segment's usage. Before any finish, each counter SHALL retain its greatest observed value.

#### Scenario: Counters are selected independently
- **WHEN** stream parts report usage
- **THEN** input total, input without cache, cache-read input, cache-write input, output total, output text and output reasoning SHALL each be selected independently by these rules
- **AND** `Usage.Raw` SHALL follow the same selection with the latest non-empty payload in place of the greatest value, without a provider-independent raw merge

#### Scenario: Split usage across stream parts
- **WHEN** an early stream part reports input and cache counters
- **AND** a later stream part reports output counters
- **THEN** the aggregate SHALL contain both sets of counters

#### Scenario: Provisional terminal usage is lower
- **WHEN** an early stream part reports a normalized counter
- **AND** the finish part that closes its segment reports a lower value for that counter
- **THEN** the aggregate SHALL use the finish part's value
- **AND** a counter the finish part omits SHALL retain the greatest value observed in that segment

#### Scenario: Several finishes use the last segment
- **GIVEN** a stream emits two finish parts, each reporting its own usage
- **WHEN** the stream completes
- **THEN** the aggregate SHALL be the second segment's usage
- **AND** counters SHALL NOT be summed or take the greatest value across segments

#### Scenario: Usage after the last finish
- **WHEN** a part after the last finish reports usage and no later finish closes its segment
- **THEN** the aggregate SHALL remain the last closed segment's usage

#### Scenario: Stream without a finish
- **WHEN** a stream ends without a finish part, for example on a provider error or cancellation
- **THEN** each counter SHALL retain its greatest observed value

#### Scenario: Raw usage selection is deterministic
- **WHEN** multiple stream parts of one segment report non-empty raw provider usage
- **THEN** the segment SHALL retain its finish part's non-empty raw payload, or otherwise the most recently observed non-empty raw payload
- **AND** a later omitted raw payload SHALL NOT clear it
