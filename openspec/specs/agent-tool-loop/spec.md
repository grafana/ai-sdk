# Agent Tool Loop

## Purpose

Define the first-class Agent and ToolLoopAgent APIs, including reusable agent settings, agent-specific loop defaults, callback/runtime-context merge behavior, provider call-header metadata, and Agent UI/HTTP stream helpers that reuse existing StreamText, GenerateText, UIMessageChunk, and SSE paths.

## Requirements

### Requirement: Agent interface identity

The root package SHALL expose an Agent abstraction for reusable LLM agents. An Agent SHALL report specification version `agent-v1`, MAY report an ID, SHALL expose its configured `ToolSet`, and SHALL provide generate and stream methods that return the existing `GenerateTextResult` and `StreamTextResult` result types.

#### Scenario: Agent identity is observable
- **WHEN** a caller constructs a tool loop agent with ID `research-agent` and tools `search` and `summarize`
- **THEN** the returned value SHALL satisfy the Agent abstraction
- **AND** its version SHALL be `agent-v1`
- **AND** its ID SHALL be `research-agent`
- **AND** its tools SHALL include `search` and `summarize`

#### Scenario: Agent methods use existing result types
- **WHEN** a caller invokes the Agent generate method
- **THEN** the method SHALL return the same result type returned by `GenerateText`
- **AND** the method SHALL expose the same steps, content, usage, warnings, metadata, and error behavior as `GenerateText`

#### Scenario: Agent stream method uses existing stream result
- **WHEN** a caller invokes the Agent stream method
- **THEN** the method SHALL return the same result type returned by `StreamText`
- **AND** callers SHALL consume the same `TextStreamPart` and `UIMessageChunk` conversion APIs available on `StreamTextResult`

### Requirement: ToolLoopAgent construction

The root package SHALL provide a ToolLoopAgent constructor that accepts a required `provider.LanguageModel` plus reusable settings through idiomatic Go functional options. Construction SHALL NOT call the provider. The constructor SHALL support reusable settings for ID, instructions or system messages, tools, tool choice, active tools, stop conditions, tool approval, prepare-step, output, provider options, request headers, retry/model parameters, timeouts, Agent runtime context, and lifecycle callbacks when the corresponding behavior exists in the lower-level Go primitives or the Agent wrapper.

#### Scenario: Construction does not call provider
- **WHEN** a ToolLoopAgent is constructed with a model whose provider methods would fail if called
- **THEN** construction SHALL complete without calling `DoStream` or `DoGenerate`

#### Scenario: Reusable settings are applied to stream calls
- **WHEN** a ToolLoopAgent is constructed with instructions, tools, provider options, and request headers
- **AND** the caller invokes the Agent stream method with a user prompt or messages
- **THEN** the underlying provider call SHALL receive the configured instructions, tool declarations, provider options, and headers through the existing `StreamText` request path

#### Scenario: Reusable settings are applied to generate calls
- **WHEN** a ToolLoopAgent is constructed with tools and a tool approval policy
- **AND** the caller invokes the Agent generate method with a user prompt or messages
- **THEN** the generated result SHALL use the same tool approval behavior as `GenerateText` configured with those options directly

### Requirement: Agent call option merge

ToolLoopAgent SHALL merge reusable settings with per-call options before delegating to `StreamText` or `GenerateText`. Reusable settings SHALL be applied first, per-call options SHALL be applied second, and per-call options SHALL override reusable settings for singleton values. Collection values SHALL follow the existing lower-level option semantics for the corresponding option. The merge SHALL NOT mutate the reusable settings stored on the Agent.

#### Scenario: Per-call messages override reusable messages
- **WHEN** a ToolLoopAgent has reusable model messages
- **AND** a stream call supplies different model messages
- **THEN** the provider call SHALL use the per-call model messages
- **AND** a later stream call without per-call messages SHALL still use the reusable model messages

#### Scenario: Per-call tool choice overrides reusable tool choice
- **WHEN** a ToolLoopAgent is configured with reusable automatic tool choice
- **AND** a generate call supplies a different tool choice
- **THEN** the provider call SHALL use the per-call tool choice

#### Scenario: Reusable settings remain stable across calls
- **WHEN** two concurrent or sequential Agent stream calls supply different per-call headers or provider options
- **THEN** each provider call SHALL receive only its own merged settings
- **AND** later calls SHALL NOT inherit per-call settings from earlier calls

### Requirement: ToolLoopAgent stop condition default

ToolLoopAgent SHALL apply `StepCountIs(20)` as its default stop condition when neither reusable settings nor per-call options specify stop conditions. This Agent-specific default SHALL NOT change the existing `StreamText` or `GenerateText` default behavior outside ToolLoopAgent.

#### Scenario: Agent defaults to twenty steps
- **WHEN** a ToolLoopAgent streams a conversation where each completed step produces executable client tool calls and no explicit stop condition is configured
- **THEN** the Agent stream SHALL allow continuation until the twentieth step or another existing loop termination condition occurs

#### Scenario: Explicit reusable stop condition overrides Agent default
- **WHEN** a ToolLoopAgent is constructed with `StepCountIs(3)`
- **AND** a stream call does not supply a stop condition
- **THEN** the Agent stream SHALL stop after the third step when normal loop continuation would otherwise continue

#### Scenario: Explicit per-call stop condition overrides Agent default and reusable default
- **WHEN** a ToolLoopAgent is constructed with no stop condition or with a reusable stop condition
- **AND** a stream call supplies `StepCountIs(2)`
- **THEN** the Agent stream SHALL stop after the second step when normal loop continuation would otherwise continue

#### Scenario: StreamText default is preserved
- **WHEN** a caller invokes `StreamText` directly without `WithStopWhen`
- **THEN** the direct call SHALL keep the existing one-step default
- **AND** it SHALL NOT inherit ToolLoopAgent's twenty-step default

### Requirement: ToolLoopAgent delegation to existing orchestration

ToolLoopAgent generate and stream methods SHALL delegate to the existing `GenerateText` and `StreamText` orchestration paths. Agent calls SHALL inherit existing behavior for model message conversion, system message prepending, prepare-step overrides, retries, timeouts, local tool execution, concurrent tool execution, provider-executed tools, external tools, structured output, stop conditions, pending approval termination, approval response resumption, warnings, stream errors, abort handling, and result accessors.

#### Scenario: Pending approval stops Agent stream
- **WHEN** an Agent stream call produces a local tool call that requires user approval
- **THEN** the stream SHALL emit the same tool approval request stream part and UI chunk as an equivalent `StreamText` call
- **AND** the Agent stream SHALL finish the current invocation without starting another model step

#### Scenario: Approved approval resumes through Agent generate
- **WHEN** an Agent generate call receives messages containing a prior approval request and an approved approval response
- **THEN** it SHALL execute the approved local tool before the model call using the same approval response collection semantics as `GenerateText`

#### Scenario: External tools remain unresolved
- **WHEN** an Agent stream call produces a client tool call for a known tool without an execute function
- **THEN** the stream SHALL finish after that step using the same unresolved external tool behavior as `StreamText`

#### Scenario: Structured output uses existing output parser
- **WHEN** a ToolLoopAgent is configured with an output parser supported by `WithOutput`
- **THEN** Agent generate and stream result accessors SHALL expose parsed output using the same success and error behavior as direct lower-level calls

### Requirement: Callback merging

ToolLoopAgent SHALL merge reusable lifecycle callbacks with per-call lifecycle callbacks. For each lifecycle event supported by the lower-level Go API, if both reusable and per-call callbacks are present, both SHALL be invoked in reusable-then-per-call order with the same callback state the lower-level call would provide. Agent callbacks SHALL inherit existing callback concurrency semantics from `StreamText`.

#### Scenario: OnStart callbacks are composed
- **WHEN** a ToolLoopAgent has a reusable start callback
- **AND** a stream call supplies a per-call start callback
- **THEN** the stream SHALL invoke the reusable callback once
- **AND** it SHALL invoke the per-call callback once after the reusable callback

#### Scenario: OnStepEnd alias composes with step finish callback
- **WHEN** a caller uses the step-end alias callback supported by the Go API together with a reusable step-finish callback
- **THEN** both callbacks SHALL be invoked for each completed step with `OnStepFinishState`

#### Scenario: Tool execution callbacks inherit concurrency
- **WHEN** one model step causes multiple local tools to execute concurrently
- **THEN** reusable and per-call tool execution callbacks MAY be invoked concurrently in the same manner as direct `StreamText` callbacks
- **AND** ToolLoopAgent SHALL NOT serialize those callbacks beyond the existing lower-level behavior

### Requirement: Agent runtime context composition

ToolLoopAgent SHALL support a reusable Agent runtime context value and per-call Agent runtime context override. Runtime context support SHALL be implemented in the Agent wrapper by composing with the existing `PrepareStepResult.Context` and `ToolExecutionOptions.Context` paths; this change SHALL NOT require a lower-level `StreamText` runtime-context option. The resolved Agent runtime context SHALL be reusable context first, overridden by per-call context only when per-call context is supplied. The Agent wrapper SHALL run any user `PrepareStep`, preserve its errors and non-context overrides unchanged, and let a non-nil `PrepareStepResult.Context` override the resolved Agent runtime context for that step. A nil `PrepareStepResult.Context` SHALL NOT clear the resolved Agent runtime context.

#### Scenario: Reusable runtime context reaches tool execution
- **WHEN** a ToolLoopAgent is configured with a reusable runtime context value
- **AND** an Agent stream call executes a local tool
- **THEN** the tool's execution options SHALL include that runtime context value

#### Scenario: Per-call runtime context overrides reusable context
- **WHEN** a ToolLoopAgent has reusable runtime context `A`
- **AND** a generate call supplies runtime context `B`
- **THEN** tools executed for that call SHALL receive runtime context `B`

#### Scenario: PrepareStep context overrides call context
- **WHEN** an Agent stream call has resolved runtime context `A`
- **AND** `PrepareStep` returns non-nil context `B` for a step
- **THEN** tools executed in that step SHALL receive context `B`

#### Scenario: PrepareStep without context preserves call context
- **WHEN** an Agent stream call has resolved runtime context `A`
- **AND** `PrepareStep` returns other overrides with nil context
- **THEN** tools executed in that step SHALL still receive context `A`
- **AND** the other prepare-step overrides SHALL be preserved

### Requirement: Agent provider call-header metadata

ToolLoopAgent SHALL add the upstream Agent marker `ai-sdk-agent/tool-loop` to `provider.CallOptions.Headers` before delegating to the provider. The marker SHALL be appended to any caller-supplied `User-Agent` header value and SHALL NOT discard unrelated caller-supplied headers. This requirement applies to the root provider-call boundary only; it SHALL NOT require provider implementation changes and SHALL NOT claim that every provider's actual outgoing network request includes the marker.

#### Scenario: Agent marker is visible in provider call options
- **WHEN** a ToolLoopAgent stream call reaches a mock provider that inspects `provider.CallOptions.Headers`
- **THEN** the call headers SHALL include a `User-Agent` value containing `ai-sdk-agent/tool-loop`

#### Scenario: Existing headers are preserved in call options
- **WHEN** a ToolLoopAgent is configured with custom headers and a stream call is made
- **THEN** the provider call options SHALL include those custom headers
- **AND** the `User-Agent` call header SHALL include the Agent marker without replacing unrelated headers

#### Scenario: Provider network header gaps are documented
- **WHEN** implementation documents Agent header behavior
- **THEN** it SHALL state that providers only send the marker on actual network requests if they honor `provider.CallOptions.Headers`
- **AND** it SHALL document OpenAI Responses ignoring call headers and provider user-agent append/replacement behavior as parity gaps unless provider work is later scoped

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

### Requirement: Persisted tool state constraints are validated before Agent invocation

The Agent UI helper SHALL reject nil/empty histories and validate represented role/part structure and tool-state required/forbidden fields before invoking Agent.Stream. On its isolated normalized clone for both tool kinds, it SHALL remove RawInput outside output-error, Preliminary outside output-available and ResultProviderMetadata outside output-available/output-error, matching pinned field projection without changing caller history, skipping represented JSON validation or weakening output/error/approval prohibitions. Available input, approval states, output-available and output-denied SHALL require Input; streaming/error input SHALL be optional. Output SHALL be required only on output-available; errorText SHALL be required only on output-error and SHALL accept a present empty string. Approval SHALL be forbidden on input-streaming/input-available, required with ID and absent Approved/decision Reason on approval-requested, required with Approved on approval-responded, required with Approved false on output-denied, and if present SHALL have Approved true on output-available/output-error. Unknown states, missing state-required fields, forbidden output/error/approval fields, unsupported parts and invalid represented JSON SHALL fail with message/part context. Existing nonempty tool identity checks SHALL remain. Whole lifecycle parts SHALL NOT require separate preceding input/approval-request parts. No general exported validator or application metadata/data schema API SHALL be introduced.

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

#### Scenario: State-inapplicable fields normalize without losing persisted history
- **WHEN** otherwise valid static/dynamic history contains RawInput, Preliminary or result metadata outside their supported states, including result metadata retained through retry/approval reader updates
- **THEN** validation SHALL remove only the inapplicable fields from its isolated clone and preserve applicable values
- **AND** conversion and response assembly SHALL use that clone without mutating caller history

#### Scenario: Explicit empty errors are valid but absent errors are not
- **WHEN** otherwise valid output-error parts contain present empty errorText or absent errorText
- **THEN** the present empty string SHALL pass state validation
- **AND** the absent field SHALL fail before Agent invocation

#### Scenario: Denied approval is required
- **WHEN** output-denied history lacks approval or carries Approved true
- **THEN** validation SHALL fail before Agent/provider invocation
- **AND** otherwise valid approval ID and Approved false SHALL pass state checks

### Requirement: Static tool schemas use pinned Agent normalization gates

The Agent UI helper SHALL reuse configured static Tool.InputSchema/OutputSchema with the registered validateUIMessagesForAgent gates. Input-available, approval-requested/responded and output-denied SHALL validate input. Input-streaming SHALL skip input-schema checks. Output-error with present input invalid under the current schema SHALL normalize to dynamic instead of reject; absent input and legacy raw input SHALL remain loadable. Output-available SHALL validate input, normalize incompatible empty-object input to dynamic, reject incompatible nonempty input, and validate configured output schema before normalization. Missing terminal static tools SHALL normalize to dynamic; missing nonterminal static tools SHALL fail. Dynamic parts SHALL receive state/shape checks but not configured tool schemas. Provider-defined Go tools without a represented schema SHALL NOT acquire invented schemas. Normalization SHALL preserve tool identity, title and state-applicable metadata, raw input, preliminary, outputs and approval data on a deep clone.

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

### Requirement: Agent HTTP response helper

The root package SHALL provide an HTTP helper that pipes an Agent UI message stream to an `http.ResponseWriter` using the existing UI SSE writer. The helper SHALL set the same SSE headers and `[DONE]` sentinel as `PipeUIMessageStreamToResponse` and SHALL NOT define a new wire format.

#### Scenario: Agent UI response uses existing SSE framing
- **WHEN** a caller pipes an Agent UI stream to an HTTP response
- **THEN** the response SHALL have `Content-Type: text/event-stream`
- **AND** it SHALL have `x-vercel-ai-ui-message-stream: v1`
- **AND** each event SHALL be encoded as `data: <UIMessageChunk JSON>\n\n`
- **AND** the stream SHALL terminate with `data: [DONE]\n\n`

#### Scenario: SSE write errors are returned
- **WHEN** the response writer returns an error while writing an Agent UI stream
- **THEN** the helper SHALL return that error with context in the same manner as `PipeUIMessageStreamToResponse`

### Requirement: Upstream gap documentation

The Agent proposal implementation SHALL document upstream `ai@7.0.19` Agent settings that are intentionally unsupported, adapted to existing Go primitives, or intentionally divergent because no current Go primitive exists. Unsupported upstream-only features SHALL NOT be exposed as silent no-op options. New parity-sensitive Agent and Agent UI surfaces SHALL be classified in the repository parity coverage documentation when implementation adds the public API.

#### Scenario: Pinned upstream gaps are classified
- **WHEN** implementation documents Agent support
- **THEN** unsupported or adapted upstream fields SHALL be classified against `ai@7.0.19`, including `prepareCall`, `allowSystemInMessages`, `experimental_download`, `include`, `_internal` ID generators, `toolsContext` and call-options-template behavior, telemetry, stream transforms, sandbox sessions, repair hooks, refine hooks, `toolOrder`, TypeScript generic/schema inference, and `callOptionsSchema`
- **AND** each item SHALL be classified as supported, parity-preserving Go adaptation, intentional deviation, or gap

#### Scenario: Unsupported upstream-only fields are not silent no-ops
- **WHEN** implementation documents Agent support
- **THEN** unsupported upstream-only fields SHALL be listed as non-goals or gaps
- **AND** the public API SHALL NOT include options for those fields unless they have implemented behavior

#### Scenario: Parity coverage records Agent surface
- **WHEN** the Agent API and Agent UI helper are implemented
- **THEN** `test/conformance/PARITY.md` SHALL classify the new core orchestration and frontend interop surfaces, their verification strategy, provider call-header gaps, Agent UI validation gaps, and unsupported upstream-only fields
