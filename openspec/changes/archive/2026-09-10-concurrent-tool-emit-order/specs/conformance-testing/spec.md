## MODIFIED Requirements

### Requirement: ID comparison strategy

The system SHALL use exact comparison for all IDs in the UIMessageChunk stream. Content block IDs (from the provider, e.g. stringified block index) and tool call IDs (from the API response) are deterministic for a given fixture. Message-level IDs SHALL be controlled via deterministic generators configured identically in both the TypeScript tools and Go tests.

Chunk sequences SHALL be compared positionally, with one exception. The comparator SHALL sort each maximal run of adjacent `tool-output-available` and `tool-output-error` chunks by `toolCallId`, in both the expected and the actual sequence, before comparing, because locally-executed tools in a step emit in completion order.

A chunk carrying `providerExecuted: true` SHALL be excluded from such a run and SHALL end one: provider-executed outputs are recorded in the order the provider sent them and SHALL stay exactly ordered.

A run SHALL NOT extend across a chunk of any other type, so ordering relative to every other chunk, and across step boundaries, stays exact.

#### Scenario: Exact comparison with deterministic IDs

- **WHEN** both SDKs process the same fixture with deterministic message ID generators
- **THEN** the UIMessageChunk sequences are compared exactly (byte-identical JSON per line), except for the order within a run of adjacent locally-executed tool output chunks

#### Scenario: Concurrent tool outputs compare regardless of arrival order

- **WHEN** a step contains two locally-executed tools whose output chunks are adjacent in the expected sequence
- **AND** the Go implementation emits them in the opposite order because the second tool completed first
- **THEN** the comparison passes

#### Scenario: Success and error outputs normalize as one run

- **WHEN** a step contains one tool that succeeds and one that fails, and their `tool-output-available` and `tool-output-error` chunks are adjacent
- **THEN** the two chunks are compared as an unordered pair

#### Scenario: Provider-executed output order is still exact

- **WHEN** two adjacent `tool-output-available` chunks carry `providerExecuted: true` and are swapped relative to the fixture
- **THEN** the comparison fails
