# gateway-streaming-function-tools Specification

## Purpose
Define the bounded streaming function-tool lifecycle, basic result transport,
stateless client loops, privacy boundaries, and acceptance evidence for the
Gateway's registered Go and Vercel clients.
## Requirements
### Requirement: Streaming function request parity
Streaming requests SHALL accept exactly WP11's supported function definitions, choices and prompt result subset, through the same complete validation and explicit mapping. Deferred families SHALL remain unsupported before resolution.

#### Scenario: Same request in both modes
- **WHEN** supported tool options are sent in unary and streaming modes
- **THEN** both SHALL map equivalent provider options and only the model invocation mode SHALL differ

### Requirement: Bounded tool input lifecycle
The stream SHALL support input-start/delta/end with independent tool IDs, preserving empty deltas. Deltas and ends SHALL match an open ID; IDs SHALL not reopen. Calls after input blocks SHALL match ID/name and follow input-end; standalone complete calls SHALL be accepted. ID tracking SHALL be bounded by the provider-part limit without accumulating input deltas. Response-metadata SHALL precede content and finish SHALL require every input block closed. Each tool-input-start/delta/end SHALL preserve bounded opaque providerMetadata at its registered ProviderWire position under gateway-provider-metadata; this SHALL NOT invent corresponding fields in UI chunks whose pinned schema does not represent them.

#### Scenario: Interleaved tool inputs
- **WHEN** two independent input blocks have interleaved deltas, including an empty delta, and then close before their matching calls
- **THEN** event order and required empty values SHALL be preserved

#### Scenario: Invalid or unfinished input
- **WHEN** an ID is reused, a delta/end lacks an open ID, names mismatch or finish arrives with an open input
- **THEN** the handler SHALL cancel and emit at most one safe synthetic terminal error

### Requirement: Basic streamed calls and results
Private DTOs SHALL encode function calls and correlated basic results with non-null JSON result and optional isError. Duplicate calls/results and unmatched results SHALL fail safely. Client-executed calls SHALL not require a result before finish. Provider-executed, dynamic and preliminary enabled behavior SHALL remain deferred to WP13; registered call/basic-result providerMetadata SHALL be preserved under gateway-provider-metadata's bounded opaque transport.

#### Scenario: Standalone call awaits client execution
- **WHEN** a complete function call with no incremental input events is followed by valid finish
- **THEN** the client SHALL receive the call and finish without requiring a server result

#### Scenario: Basic result transport
- **WHEN** a recording model emits a call followed by one matching basic JSON result
- **THEN** both clients SHALL receive equivalent call/result values and their distinct metadata in order

#### Scenario: Deferred execution marker
- **WHEN** providerExecuted, dynamic or preliminary is true in provider output
- **THEN** unsupported output SHALL produce a safe terminal failure without exposing provider metadata

#### Scenario: Distinct call and result metadata
- **WHEN** a supported function call and its correlated basic result carry different unknown namespace objects or empty objects
- **THEN** both clients SHALL receive each object at its original event without merging call and result metadata or enabling deferred execution markers

### Requirement: Existing streaming bounds and terminal semantics
Tool events SHALL use the existing part count, complete-frame budget, idle/total deadlines and single cleanup owner. Provider error parts SHALL remain ordered and non-terminal without changing tool state. Finish SHALL be the final event followed by immediate clean EOF; no DONE sentinel SHALL be emitted. Write failure SHALL stop without another write.

#### Scenario: Error during input then completion
- **WHEN** a provider error occurs between valid input deltas and the provider later closes the block, emits its call and finish
- **THEN** safe error and subsequent events SHALL remain ordered with no candidate switch

#### Scenario: Cancellation or oversized result
- **WHEN** cancellation occurs with open input or a result exceeds its full frame budget
- **THEN** provider work SHALL be canceled, owned cleanup SHALL remain bounded and no partial oversized event SHALL be written

### Requirement: Stateless multi-step client loops
Registered Vercel and independent Go client orchestration SHALL complete multi-step local function loops through independent real-handler requests with the existing supported continuation history on direct and configured-fallback routes. The Gateway SHALL execute no local function and retain no cross-request workflow state. Each request SHALL produce one canonical metadata-only logical observation. Configured fallback SHALL accept the same mapped definitions/history/choices/options as direct streaming invocation, with existing eligibility and cancellation. Every first provider part, including stream-start, error, tool input or call, SHALL irrevocably select that candidate; later failures SHALL NOT replay the selected stream or start a later candidate. Each new continuation request SHALL begin at the configured primary. Local execution evidence SHALL NOT imply an exactly-once provider guarantee.

#### Scenario: Two-client multi-step acceptance
- **WHEN** each client receives a function call on a direct or configured-fallback route, executes its deterministic local function once and sends the result in the next request
- **THEN** both SHALL receive final text, native request history SHALL match and logical records SHALL count one generation per request

#### Scenario: Secret-bearing tool data
- **WHEN** tool input/result/name/ID/schema or provider metadata contains hostile markers
- **THEN** public allowlisted tool content SHALL retain required semantics but exported logs/metrics/metadata-only AO SHALL omit those markers and all private provider metadata

#### Scenario: Streaming tool on fallback route
- **WHEN** a supported tool request or continuation targets a configured-fallback route and the primary has an eligible setup failure before any part
- **THEN** the next candidate SHALL receive the same mapped request and the existing bounded tool-input/call lifecycle SHALL reach the consumer without a blanket effect refusal

#### Scenario: Tool events commit without replay
- **WHEN** the selected stream emits tool input or a complete call and then errors, closes prematurely or exceeds an encoding bound
- **THEN** existing strict terminal behavior SHALL apply without invoking a later candidate, duplicating tool events or adding a Gateway executor

#### Scenario: Stream-start precedes tool failure
- **WHEN** a primary emits stream-start before a tool-related error and the secondary would succeed
- **THEN** the primary SHALL remain selected and the secondary SHALL NOT run even if the error is retryable
