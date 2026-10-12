## MODIFIED Requirements

### Requirement: Reader tool updates preserve pinned lifecycle semantics

Both readers SHALL carry title/tool metadata across partial input updates, preserve supplied empty values, and follow the registered static/dynamic updater field-clearing rules for output, error, preliminary and raw input. Both static and dynamic tool-input-error SHALL store rejected input in Input and clear streaming RawInput. Input-streaming deltas SHALL retain the accumulated raw text as a JSON string in RawInput; available input and output-available transitions SHALL clear RawInput. Dynamic output-error SHALL clear RawInput, while static output-error SHALL retain legacy RawInput when supplied. Output-available and output-error chunks SHALL replace toolMetadata when a non-null object is supplied and inherit it when omitted. Preliminary and final outputs SHALL replace one matching tool part. Approval responses SHALL merge supported prior approval data. Static/dynamic tool updaters and approval responses SHALL apply supplied providerExecuted values, including decoded false clearing prior true, while an absent field SHALL inherit the prior value. Existing step-local identity matching, partial JSON repair and split progressive/blocking error contracts SHALL remain in force.

#### Scenario: Static and dynamic input errors retain rejected input
- **WHEN** otherwise equivalent tool-input-error chunks target static and dynamic tools
- **THEN** the static output-error part SHALL retain Input and omit RawInput
- **AND** the dynamic output-error part SHALL retain Input
- **AND** title, tool metadata and result-provider metadata SHALL remain associated with that part

#### Scenario: Static error continuations preserve rejected input
- **WHEN** a static output-error part receives a tool-output-error continuation
- **THEN** reader snapshots SHALL retain rejected Input or separately seeded legacy RawInput
- **AND** conversion SHALL still use that retained input through its nullish fallback

#### Scenario: Final output replaces preliminary output
- **WHEN** a tool receives a preliminary available output followed by a final available output
- **THEN** snapshots SHALL contain one tool part with the respective current output and preliminary presence
- **AND** a subsequent output-error transition SHALL clear stale output and preliminary as the pinned updater does

#### Scenario: Partial input retains supplied presentation metadata
- **WHEN** input-start includes title and toolMetadata, followed by input deltas and input-available without replacements
- **THEN** reader snapshots SHALL retain the earlier title/toolMetadata
- **AND** an explicitly supplied empty replacement SHALL replace rather than disappear

#### Scenario: Explicit false changes provider execution while omission inherits
- **WHEN** a static or dynamic tool whose providerExecuted is true receives a decoded approval response, output-available or output-error chunk
- **THEN** supplied providerExecuted false SHALL clear that prior true value, while an omitted providerExecuted SHALL retain it
- **AND** subsequent model conversion SHALL place the applicable output in the local tool-role message after false, versus provider-inline after inherited true

### Requirement: Readers resume from an isolated initial assistant message

The root package SHALL add `WithUIMessageReaderInitialMessage(message UIMessage) UIMessageReaderOption`. It SHALL clone the supplied message when the option is built and clone it for each reader invocation. Readers SHALL seed contents only from assistant messages; a supplied non-assistant message SHALL initialize an empty assistant with that supplied ID while ignoring its parts/metadata. Initial IDs and assistant tool/data identities SHALL be retained unless later chunks replace them according to the pinned rules. Active text and reasoning maps SHALL start empty. The partial-input map SHALL restore input-streaming tool parts from the last persisted step, using their string RawInput or empty text when absent. No initial progressive snapshot SHALL be emitted merely because an initial message was supplied. Generated-ID fallback SHALL apply only when the supplied ID is absent; without an initial message existing generated-ID and empty-stream behavior SHALL remain unchanged.

#### Scenario: Persisted tool output resumes without duplicates
- **WHEN** an initial assistant message contains a persisted tool call and the stream supplies an output or approval continuation for it
- **THEN** snapshots and blocking assembly SHALL update the existing matching part rather than append a duplicate
- **AND** unchanged parts, metadata, approval data and message ID SHALL be retained

#### Scenario: Initial-message option reuse and mutation are isolated
- **WHEN** a caller mutates its original message after constructing the option or reuses the option for independent readers
- **THEN** those readers SHALL start from isolated clones of the option's original state
- **AND** consumer mutation SHALL NOT affect another reader or later snapshot

#### Scenario: Empty resumed stream respects helper contracts
- **WHEN** an initial assistant message is supplied and the input channel closes without chunks
- **THEN** StreamUIMessage SHALL emit no snapshots
- **AND** AssembleUIMessage SHALL return the isolated initial message with nil error

#### Scenario: Non-assistant seed contents are ignored but its ID is retained
- **WHEN** an initial non-assistant message with a supplied ID is provided
- **THEN** readers SHALL initialize an empty assistant with that ID and no seed parts/metadata
- **AND** a later start chunk with a new messageId SHALL replace it

#### Scenario: Non-assistant seed without an ID uses the existing fallback
- **WHEN** an initial non-assistant message has no supplied ID
- **THEN** readers SHALL initialize empty assistant contents and use the existing generated-ID fallback when an ID is needed

#### Scenario: Persisted input resumes from the last step
- **WHEN** a tool-input-delta arrives for an input-streaming tool in the last persisted step
- **THEN** readers SHALL append to the persisted raw string and update the existing part
- **AND** text/reasoning deltas and tool deltas from older steps SHALL retain malformed-transition behavior without an active start

#### Scenario: Output chunks replace tool presentation metadata
- **WHEN** a static or dynamic tool receives output-available or output-error with a supplied toolMetadata object
- **THEN** readers and chunk serialization SHALL preserve that replacement, including an explicitly empty object

#### Scenario: Malformed persisted streaming input is rejected
- **WHEN** an initial assistant contains last-step input-streaming RawInput that is null or non-string JSON
- **THEN** blocking assembly SHALL return a contextual error and progressive assembly SHALL close without snapshots

#### Scenario: Persisted data identities and start IDs are respected
- **WHEN** an initial assistant contains an identified data part and receives a replacement data chunk for that ID
- **THEN** the existing data part SHALL be updated according to the target matching rules
- **AND** a later start chunk with a new messageId SHALL replace the initial message ID without mutating the caller's message
