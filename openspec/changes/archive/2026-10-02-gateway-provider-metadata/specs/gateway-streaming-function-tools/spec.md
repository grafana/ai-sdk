## MODIFIED Requirements

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
