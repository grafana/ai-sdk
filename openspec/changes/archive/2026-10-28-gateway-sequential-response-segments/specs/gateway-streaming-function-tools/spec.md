## MODIFIED Requirements

### Requirement: Bounded tool input lifecycle

The stream SHALL support input-start/delta/end with independent tool IDs, preserving empty deltas. Deltas and ends SHALL match an open ID; IDs SHALL not reopen within the stream. Calls after input blocks SHALL match ID/name and follow input-end; standalone complete calls SHALL be accepted. ID tracking SHALL NOT accumulate input deltas. Response-metadata and finish SHALL each require every input block closed.

#### Scenario: Interleaved tool inputs
- **WHEN** two independent input blocks have interleaved deltas, including an empty delta, and then close before their matching calls
- **THEN** event order and required empty values SHALL be preserved

#### Scenario: Invalid or unfinished input
- **WHEN** an ID is reused, a delta/end lacks an open ID, names mismatch or finish arrives with an open input
- **THEN** the handler SHALL cancel and emit at most one safe synthetic terminal error

#### Scenario: Metadata after a complete call
- **WHEN** a complete tool call is followed by response-metadata with no input block open
- **THEN** the metadata SHALL be written and open a new response segment

#### Scenario: Metadata while input is open
- **WHEN** response-metadata arrives between an input-start and its input-end
- **THEN** the handler SHALL cancel and emit at most one safe synthetic terminal error

#### Scenario: Tool ID reused in a later segment
- **WHEN** a later response segment starts input or a call with an ID used in an earlier segment
- **THEN** the handler SHALL cancel and emit at most one safe synthetic terminal error

### Requirement: Existing streaming bounds and terminal semantics
Tool events SHALL use the existing idle/total deadlines and single cleanup owner, without a part count or frame budget. Provider error parts SHALL remain ordered and non-terminal without changing tool state. Finish SHALL follow the sequential response segment rules of providerwire-v4-streaming-runtime: the response SHALL end at clean EOF when the provider closes while the last lifecycle part is a valid finish, and no DONE sentinel SHALL be emitted. Write failure SHALL stop without another write.

#### Scenario: Error during input then completion
- **WHEN** a provider error occurs between valid input deltas and the provider later closes the block, emits its call and finish
- **THEN** safe error and subsequent events SHALL remain ordered with no candidate switch

#### Scenario: Cancellation or oversized result
- **WHEN** cancellation occurs with open input
- **THEN** provider work SHALL be canceled, owned cleanup SHALL remain bounded and no partial event SHALL be written
- **WHEN** a valid tool input delta, call or result encodes to a frame above the former 1 MiB limit, or a stream opens more tool calls than the former part count
- **THEN** each complete event SHALL be written in order and the stream SHALL continue

#### Scenario: Provider-executed result in a later segment
- **WHEN** a complete provider-executed call ends one response segment's output and its matching final result arrives in a later segment of the same stream
- **THEN** the call, both finish parts and the result SHALL be written in order with no candidate switch
