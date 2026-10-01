## MODIFIED Requirements

### Requirement: Agent UI stream helper

The root package SHALL provide an Agent UI stream helper equivalent in intent to upstream `createAgentUIStream`. The helper SHALL accept an Agent and UI message history, validate and normalize the represented UI history against the registered Agent-specific validation behavior on an isolated clone, convert that normalized history to model messages through the existing conversion path, call the Agent stream method, and return a `UIMessageChunk` stream produced by `StreamTextResult.ToUIMessageStream` with the normalized messages preserved as original messages in `UIMessageStreamOptions`.

The helper SHALL reject invalid tool history before starting a provider stream when represented by the current Go UI model: static `ToolInvocationPart` tool names that are empty or absent from the Agent tool set in nonterminal states, unknown tool invocation states, and final-state tool parts with missing `ToolCallID`, tool name, or state-required fields. A static `ToolInvocationPart` or `DynamicToolUIPart` in a final state (`output-available`, `output-error`, `output-denied`, or `approval-responded`) SHALL be treated as representing the tool call itself when those required fields are present. The helper SHALL NOT require a separate prior input part, approval-request part, or prior `ToolCallID` reference for those current lifecycle parts. If a future UI model introduces separate result or approval-response parts that do not themselves represent the tool call, only those separate parts SHALL require cross-reference validation against a prior represented tool call or approval request with the same ID and tool name. Missing terminal static tools in output-available, output-error or output-denied states SHALL normalize to DynamicToolUIPart with all represented fields preserved, matching validateUIMessagesForAgent. Caller history SHALL NOT be mutated. The normalized slice SHALL be used consistently for conversion and original-message response assembly. Schema and state constraints below SHALL run before Agent.Stream, and failures SHALL NOT invoke a provider. Optional application metadata/data schemas, unrepresented provider-tool schemas and existing provider-domain optional-empty serialization SHALL remain explicit support/evidence boundaries, not claims of exhaustive validation parity.

#### Scenario: UI messages are converted before Agent stream
- **WHEN** the Agent UI stream helper receives valid UI messages
- **THEN** it SHALL convert them to model messages with the existing UI-to-model conversion behavior
- **AND** it SHALL call the Agent stream method with those model messages

#### Scenario: Conversion error is returned before streaming
- **WHEN** the Agent UI stream helper receives UI messages that cannot be converted or validated
- **THEN** it SHALL return an error before starting a provider stream
- **AND** it SHALL NOT emit partial UI message chunks

#### Scenario: Invalid tool name is rejected before streaming
- **WHEN** the Agent UI stream helper receives a static tool invocation part for tool `missingTool` in input-available state
- **AND** the Agent tool set does not contain `missingTool`
- **THEN** the helper SHALL return an error before starting a provider stream

#### Scenario: Single final-state tool invocation is accepted before streaming
- **WHEN** the Agent UI stream helper receives a persisted assistant message containing a single static `ToolInvocationPart` with state `ToolStateOutputAvailable`, tool call ID `call-1`, tool name `search`, and required output fields
- **AND** the supplied UI history has no separate prior input or approval-request part for `call-1`
- **THEN** the helper SHALL accept the message as a represented tool call
- **AND** it SHALL convert the UI history to model messages before starting the provider stream

#### Scenario: Missing final-state required fields are rejected before streaming
- **WHEN** the Agent UI stream helper receives a final-state tool invocation part with an empty tool call ID, empty tool name, or missing fields required by that state
- **THEN** the helper SHALL return an error before starting a provider stream

#### Scenario: UI chunks match existing conversion
- **WHEN** an Agent stream produces the same `TextStreamPart` sequence as a direct `StreamText` call
- **THEN** the Agent UI stream helper SHALL emit the same `UIMessageChunk` sequence as `StreamTextResult.ToUIMessageStream` for the same `UIMessageStreamOptions`

#### Scenario: Original messages are preserved for response assembly
- **WHEN** the Agent UI stream helper is called with UI message history
- **THEN** the resulting UI stream SHALL use its isolated validated/normalized history as original messages for message ID and response assembly behavior in the same manner as `ToUIMessageStream`

## ADDED Requirements

### Requirement: Persisted tool state constraints are validated before Agent invocation

The Agent UI helper SHALL reject nil/empty histories and validate represented role/part structure and tool-state required/forbidden fields before invoking Agent.Stream. On its isolated normalized clone, it SHALL remove RawInput retained by dynamic reader updates outside output-error, matching pinned validation without changing caller history or weakening other state constraints. Available input, approval states, output-available and output-denied SHALL require Input; streaming/error input SHALL be optional. Output SHALL be required only on output-available; errorText SHALL be required only on output-error and SHALL accept a present empty string. Approval SHALL be forbidden on input-streaming/input-available, required with ID and absent Approved/decision Reason on approval-requested, required with Approved on approval-responded, required with Approved false on output-denied, and if present SHALL have Approved true on output-available/output-error. Unknown states, missing state-required fields, forbidden output/error/approval fields, unsupported parts and invalid represented JSON SHALL fail with message/part context. Existing nonempty tool identity checks SHALL remain. Whole lifecycle parts SHALL NOT require separate preceding input/approval-request parts. No general exported validator or application metadata/data schema API SHALL be introduced.

#### Scenario: Invalid state combinations never reach a provider
- **WHEN** persisted messages contain unknown states, missing required fields or contradictory output/error/approval combinations
- **THEN** the helper SHALL return a contextual error before Agent.Stream
- **AND** fake provider invocation count SHALL be zero

#### Scenario: Empty histories do not invoke the Agent
- **WHEN** the UI helper receives nil or empty message history
- **THEN** it SHALL return a validation error before Agent.Stream
- **AND** provider invocation count SHALL be zero

#### Scenario: Dynamic reader history remains loadable after success
- **WHEN** a seeded dynamic output-error with RawInput receives a successful output and is persisted
- **THEN** Agent validation SHALL accept the reader-produced history and remove RawInput from its normalized clone
- **AND** conversion and response assembly SHALL use that clone without mutating the persisted caller history

#### Scenario: Explicit empty errors are valid but absent errors are not
- **WHEN** otherwise valid output-error parts contain present empty errorText or absent errorText
- **THEN** the present empty string SHALL pass state validation
- **AND** the absent field SHALL fail before Agent invocation

#### Scenario: Denied approval is required
- **WHEN** output-denied history lacks approval or carries Approved true
- **THEN** validation SHALL fail before Agent/provider invocation
- **AND** otherwise valid approval ID and Approved false SHALL pass state checks

### Requirement: Static tool schemas use pinned Agent normalization gates

The Agent UI helper SHALL reuse configured static Tool.InputSchema/OutputSchema with the registered validateUIMessagesForAgent gates. Input-available, approval-requested/responded and output-denied SHALL validate input. Input-streaming SHALL skip input-schema checks. Output-error with present input invalid under the current schema SHALL normalize to dynamic instead of reject; absent input and legacy raw input SHALL remain loadable. Output-available SHALL validate input, normalize incompatible empty-object input to dynamic, reject incompatible nonempty input, and validate configured output schema before normalization. Missing terminal static tools SHALL normalize to dynamic; missing nonterminal static tools SHALL fail. Dynamic parts SHALL receive state/shape checks but not configured tool schemas. Provider-defined Go tools without a represented schema SHALL NOT acquire invented schemas. Normalization SHALL preserve tool identity, title, metadata, raw input, preliminary, outputs and approval data on a deep clone.

#### Scenario: Completed obsolete tool history stays loadable
- **WHEN** a terminal static tool part refers to a tool no longer configured on the Agent
- **THEN** it SHALL become a dynamic part retaining its supported data
- **AND** conversion and response assembly SHALL use the same normalized representation without mutating the caller's history

#### Scenario: Failed incompatible input normalizes instead of rejecting
- **WHEN** a configured static output-error part contains input incompatible with its current input schema
- **THEN** validation SHALL normalize it to dynamic
- **AND** its input/error/metadata SHALL remain intact for conversion

#### Scenario: Empty terminal input differs from nonempty schema failure
- **WHEN** output-available parts have incompatible empty-object input or incompatible nonempty input
- **THEN** the empty-object case SHALL normalize to dynamic
- **AND** the nonempty case SHALL fail before provider invocation
- **AND** an invalid configured output SHALL still fail in the empty-object case before dynamic normalization

#### Scenario: Partial and dynamic data avoid unrelated schema checks
- **WHEN** input-streaming static input is partial or a structurally valid dynamic part has input outside a same-named configured schema
- **THEN** tool input-schema validation SHALL NOT reject those parts
- **AND** state/shape constraints SHALL still be enforced

#### Scenario: Configured schema failure blocks provider calls
- **WHEN** an input-available or approval/denied static input fails its schema, or output-available output fails its configured output schema
- **THEN** the helper SHALL return an indexed validation error
- **AND** neither Agent.Stream nor the provider SHALL be invoked
