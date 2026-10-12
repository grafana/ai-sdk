# Functional Options

## Purpose

Define the shared functional-option API for streaming and non-streaming orchestration, including message input, model settings, tools, callbacks, per-step preparation, and structured output.

## Requirements

### Requirement: StreamOption and GenerateOption are sealed interfaces
`StreamOption` and `GenerateOption` SHALL be interface types with unexported marker methods, preventing external implementation. `StreamOption` applies to streaming APIs. `GenerateOption` applies to non-streaming APIs. Options that are valid for both APIs SHALL implement both interfaces.

#### Scenario: Shared option accepted by both APIs
- **WHEN** an option implementing both `StreamOption` and `GenerateOption` (e.g., `WithTemperature`) is passed to `StreamText`
- **THEN** the option is accepted and applied to the stream configuration

#### Scenario: Shared option accepted by GenerateText
- **WHEN** an option implementing both `StreamOption` and `GenerateOption` (e.g., `WithTemperature`) is passed to `GenerateText`
- **THEN** the option is accepted and applied to the generation configuration

#### Scenario: Stream-only option rejected by GenerateText at compile time
- **WHEN** a stream-only option (e.g., `OnChunk`) is passed to `GenerateText`
- **THEN** the code SHALL fail to compile because the option does not implement `GenerateOption`

### Requirement: StreamText accepts model as positional argument
`StreamText` SHALL have the signature `func StreamText(ctx context.Context, model provider.LanguageModel, opts ...StreamOption) *StreamTextResult`. The model parameter is required and positional.

#### Scenario: StreamText called with model and options
- **WHEN** `StreamText` is called with a context, model, and zero or more `StreamOption` values
- **THEN** the model is used for streaming and all options are applied to the configuration

### Requirement: GenerateText accepts model as positional argument
`GenerateText` SHALL have the signature `func GenerateText(ctx context.Context, model provider.LanguageModel, opts ...GenerateOption) (*GenerateTextResult, error)`. The model parameter is required and positional.

#### Scenario: GenerateText called with model and options
- **WHEN** `GenerateText` is called with a context, model, and zero or more `GenerateOption` values
- **THEN** the model is used for generation, all options are applied, the stream is drained, and the result is returned

### Requirement: Message options
The library SHALL provide `WithMessages(msgs ...UIMessage)`, `WithModelMessages(msgs ...provider.Message)`, and `WithSystemMessages(msgs ...SystemModelMessage)` as shared options implementing both `StreamOption` and `GenerateOption`. `WithSystem(text string)` SHALL be a convenience that creates a single text system message.

#### Scenario: WithMessages sets UI messages
- **WHEN** `WithMessages` is passed with one or more `UIMessage` values
- **THEN** those messages are used as the conversation input, converted to model messages internally

#### Scenario: WithModelMessages sets provider messages directly
- **WHEN** `WithModelMessages` is passed with one or more `provider.Message` values
- **THEN** those messages are used directly as model input without conversion

#### Scenario: WithSystem sets a text system message
- **WHEN** `WithSystem("you are helpful")` is passed
- **THEN** a single text system message is prepended to the conversation

#### Scenario: WithSystemMessages sets multiple system messages
- **WHEN** `WithSystemMessages` is passed with one or more `SystemModelMessage` values
- **THEN** those system messages are prepended to the conversation

### Requirement: Model parameter options eliminate pointer indirection
The library SHALL provide option functions for all model tuning parameters: `WithTemperature(float64)`, `WithMaxOutputTokens(int)`, `WithTopP(float64)`, `WithTopK(int)`, `WithSeed(int)`, `WithPresencePenalty(float64)`, `WithFrequencyPenalty(float64)`, `WithStopSequences(...string)`. All SHALL be shared options. Callers SHALL NOT need to create pointer variables for optional scalar values.

#### Scenario: WithTemperature sets temperature without pointer
- **WHEN** `WithTemperature(0.7)` is passed as an option
- **THEN** the temperature is set to 0.7 in the resulting `CallOptions` sent to the provider

#### Scenario: WithMaxOutputTokens sets token limit without pointer
- **WHEN** `WithMaxOutputTokens(4096)` is passed as an option
- **THEN** the max output tokens is set to 4096 in the resulting `CallOptions`

#### Scenario: Omitted parameter options remain nil in CallOptions
- **WHEN** `WithTemperature` is not included in the options
- **THEN** the temperature field in `CallOptions` remains nil (provider uses its default)

### Requirement: Tool options

The library SHALL provide shared `WithTools(ToolSet)`, `WithToolChoice(provider.ToolChoice)`, `WithActiveTools(...string)` and `WithStopWhen(...StopCondition)` options. Before each provider invocation, shared orchestration SHALL resolve tool choice independently of effective tools. Non-nil `PrepareStepResult.ToolChoice` overrides configuration for that step; otherwise configuration applies, then default auto.

#### Scenario: WithTools configures available tools
- **WHEN** `WithTools` is passed with a `ToolSet`
- **THEN** the tools are converted to provider tools and included in `CallOptions`

#### Scenario: WithActiveTools filters available tools
- **WHEN** `WithActiveTools("search", "calculate")` is passed alongside `WithTools`
- **THEN** only the named tools are included in the `CallOptions` sent to the provider

#### Scenario: WithStopWhen configures stop conditions
- **WHEN** `WithStopWhen` is passed with one or more `StopCondition` values
- **THEN** the step loop evaluates those conditions to determine when to stop

#### Scenario: Omitted choice without tools
- **WHEN** a shared text entry point is called without tool choice and with tools either absent or explicitly empty
- **THEN** the provider call SHALL contain a non-nil automatic tool choice and no tools

#### Scenario: Omitted choice with tools
- **WHEN** tools are configured and active but no tool choice is supplied
- **THEN** the provider call SHALL contain those tools and an automatic tool choice

#### Scenario: Active tools disable every tool
- **WHEN** configured tools are filtered to empty by an explicit empty global active-tools selection or a non-nil empty `PrepareStepResult.ActiveTools`
- **AND** neither configuration nor that step supplies a tool choice
- **THEN** the provider call SHALL contain no tools and a non-nil automatic choice

#### Scenario: Explicit choice survives empty tools
- **WHEN** the configured choice is auto, none, required, or a named tool and the effective tools list is empty
- **THEN** the provider call SHALL preserve that exact choice, including the named tool name

#### Scenario: Step choice takes precedence
- **WHEN** configuration supplies a choice and `PrepareStep` supplies a different non-nil choice for the current step
- **THEN** the provider call SHALL use the step choice regardless of the effective tool count
- **AND** a nil step choice SHALL instead use the configured choice or default auto if configuration omitted it

#### Scenario: Step override does not persist
- **WHEN** one step supplies a choice override and a later step omits the override
- **THEN** the later provider call SHALL use the configured choice or automatic default rather than the earlier override

#### Scenario: Shared generate and Agent calls prepare the same default
- **WHEN** `GenerateText`, `ToolLoopAgent.Stream`, or `ToolLoopAgent.Generate` is invoked without tools or choice
- **THEN** its shared provider streaming call SHALL contain the same automatic choice as `StreamText`
- **AND** configured, Agent per-call, and step-specific explicit choices SHALL retain their existing precedence

### Requirement: Tool choice presence and step isolation

Absent configured and step choices SHALL produce `provider.ToolChoice{Type: provider.ToolChoiceAuto}` in `CallOptions.ToolChoice`, even with absent, empty or filtered-empty tools. Explicit auto, none, required and named choices SHALL survive without replacement or tool-count omission. Step overrides SHALL NOT mutate configured choice or leak to later steps.

#### Scenario: Tool choice presence and step isolation
- **WHEN** a named configured choice has no effective tools
- **THEN** the provider call SHALL preserve the named choice rather than omit or replace it

### Requirement: Shared text tool-choice resolution

Tool-choice resolution rules SHALL apply through shared orchestration to `StreamText`, `GenerateText`, `ToolLoopAgent.Stream` and `ToolLoopAgent.Generate`.

#### Scenario: Shared text tool-choice resolution
- **WHEN** `ToolLoopAgent.Generate` prepares a step with no configured or overridden choice
- **THEN** its shared provider call SHALL use automatic choice, including with no tools

### Requirement: Completed responses satisfy the effective tool choice

Shared text orchestration SHALL validate completed responses against effective per-step choice, including `PrepareStepResult.ToolChoice` precedence and later reset to configured choice/default auto. This SHALL apply to `StreamText`, `GenerateText`, `ToolLoopAgent.Stream` and `ToolLoopAgent.Generate` without option/result signature changes. Unsatisfied completed responses SHALL be terminal semantic failures before new local approval or execution processing.

#### Scenario: Required choice with no parsed call
- **WHEN** a model call completes with required choice and reasoning/text but no parsed tool call
- **THEN** the response SHALL fail with a tool-choice semantic error
- **AND** tool-call-shaped text SHALL not satisfy the choice

#### Scenario: Named choice with only an unrelated call
- **WHEN** a model call completes under named choice lookup and contains only a call to another configured tool
- **THEN** the response SHALL fail before that tool's local execution or approval processing

#### Scenario: Valid required and named controls
- **WHEN** a completed required response contains a parsed call or a completed named response contains the selected parsed call
- **THEN** tool-choice validation SHALL succeed
- **AND** existing tool validation, approval, finish eligibility and stop conditions SHALL still apply

#### Scenario: Matching and unrelated calls coexist
- **WHEN** a named-choice response contains the selected call and another tool call
- **THEN** the choice SHALL be satisfied
- **AND** the additional call SHALL not be rejected merely because its name differs

#### Scenario: Invalid or provider-executed call satisfies presence
- **WHEN** a required response contains an invalid or provider-executed parsed call, or a named response contains such a call with the selected name
- **THEN** that call SHALL satisfy choice presence
- **AND** its existing invalid-input error or provider-executed handling SHALL remain unchanged

#### Scenario: PrepareStep strengthens or relaxes enforcement
- **WHEN** configured auto is overridden by required or named choice for the current step
- **THEN** completed-response validation SHALL enforce the override
- **AND** when configured required or named is overridden by auto or none, the configured requirement SHALL not be enforced on that step

#### Scenario: Choice override resets on later step
- **WHEN** a successful step overrides choice and a subsequent step supplies nil choice
- **THEN** the later response SHALL be validated against configured choice or default auto, not the previous override

#### Scenario: Empty effective tools do not disable response validation
- **WHEN** a required or named choice is forwarded with absent, empty or filtered-empty tools and the model completes without a satisfying call
- **THEN** the response SHALL still fail choice validation

#### Scenario: Auto and none remain unchanged
- **WHEN** a completed response uses auto or none choice
- **THEN** this validation SHALL not introduce a missing-call error or a new prohibition on returned calls

### Requirement: Required and named choice satisfaction

Required choice SHALL need at least one parsed tool call; named choice SHALL need a parsed call with the selected name. Invalid and provider-executed parsed calls SHALL count while retaining validity/execution semantics. Named choice SHALL NOT prohibit additional calls if a selected call exists. Auto/none SHALL retain existing response behavior. Tool-call-like text SHALL NOT count.

#### Scenario: Required and named choice satisfaction
- **WHEN** a completed named response has the selected parsed call and an unrelated parsed call
- **THEN** presence SHALL satisfy choice without rejecting the unrelated call solely for its name

### Requirement: Tool choice enforcement boundaries

Enforcement SHALL NOT depend on effective tool count, local executability, provider request rewriting or continuation eligibility. Incomplete/canceled calls and existing provider errors SHALL retain their behavior rather than acquire a missing-required-call error.

#### Scenario: Tool choice enforcement boundaries
- **WHEN** a required-choice model call is canceled before completion
- **THEN** it SHALL retain cancellation behavior and SHALL NOT gain a missing-required-call error

### Requirement: Provider integration options
The library SHALL provide `WithProviderOptions(opts ...provider.ProviderOption)`, `WithHeaders(map[string]string)`, and `WithResponseFormat(provider.ResponseFormat)` as shared options. `WithProviderOptions` accepts variadic typed provider option values and builds the options map internally using each value's `ProviderKey()`.

#### Scenario: WithProviderOptions passes provider-specific config
- **WHEN** `WithProviderOptions` is passed with one or more typed `provider.ProviderOption` values
- **THEN** the options are built into a map keyed by `ProviderKey()` and forwarded to `CallOptions.ProviderOptions`

#### Scenario: WithHeaders sets request headers
- **WHEN** `WithHeaders` is passed with a headers map
- **THEN** the headers are forwarded to `CallOptions.Headers`

### Requirement: Shared callback options
The library SHALL provide `OnStart`, `OnStepStart`, `OnStepFinish`, `OnError`, `OnToolCallStart`, and `OnToolCallFinish` as shared options implementing both `StreamOption` and `GenerateOption`.

#### Scenario: OnStepFinish callback invoked after each step
- **WHEN** `OnStepFinish(fn)` is passed and a step completes
- **THEN** the callback `fn` is invoked with the step finish state

#### Scenario: OnError callback invoked on error
- **WHEN** `OnError(fn)` is passed and an error occurs during processing
- **THEN** the callback `fn` is invoked with the error

### Requirement: Stream-only options
`OnChunk` and `WithIncludeRawChunks` SHALL implement only `StreamOption`, not `GenerateOption`. They are exclusive to the streaming API.

#### Scenario: OnChunk invoked for each streaming chunk
- **WHEN** `OnChunk(fn)` is passed to `StreamText` and chunks arrive
- **THEN** the callback `fn` is invoked for each chunk

#### Scenario: WithIncludeRawChunks enables raw chunk forwarding
- **WHEN** `WithIncludeRawChunks()` is passed to `StreamText`
- **THEN** raw provider chunks are included in the stream

### Requirement: PrepareStep and Output options

`WithPrepareStep(PrepareStepFunc)` and `WithOutput(Output)` SHALL be shared options. Each preparation SHALL receive a zero-based step number equal to completed steps. Before each call, `Messages` SHALL expose current provider messages, `InitialMessages` original converted inputs, and `ResponseMessages` ordered accumulated responses. Non-nil result `Messages` overrides SHALL become the current base for that and later steps while responses accumulate independently.

#### Scenario: WithPrepareStep enables per-step configuration
- **WHEN** `WithPrepareStep(fn)` is passed
- **THEN** the function `fn` is called before each step to allow overriding model, messages, tool choice, active tools, and provider options

#### Scenario: PrepareStep uses zero-based step numbers
- **WHEN** `PrepareStepFunc` is invoked for the first and subsequent steps
- **THEN** `PrepareStepState.StepNumber` is `0` for the first step and increments by one for each subsequent step
- **AND** `PrepareStepState.StepNumber` equals the length of `PrepareStepState.Steps`

#### Scenario: First step separates initial and response messages
- **WHEN** `StreamText` or `GenerateText` starts with model messages and no resumed approval results
- **THEN** the first `PrepareStepState.InitialMessages` SHALL equal the original converted input messages
- **AND** `PrepareStepState.ResponseMessages` SHALL be empty
- **AND** `PrepareStepState.Messages` SHALL contain the provider messages for the first call, including any configured system messages

#### Scenario: Later steps expose accumulated response messages
- **WHEN** a completed step produces response messages and orchestration starts another step
- **THEN** `PrepareStepState.InitialMessages` SHALL remain equal to the original converted input messages
- **AND** `PrepareStepState.ResponseMessages` SHALL contain the response messages from every prior completed step in order
- **AND** `PrepareStepState.Messages` SHALL contain the current carry-forward prompt

#### Scenario: Message override carries forward without replacing response history
- **WHEN** `PrepareStep` returns a non-nil `Messages` override for a step that produces response messages
- **AND** orchestration starts another step
- **THEN** the next step's `Messages` SHALL contain the override followed by the produced response messages
- **AND** the next step's `ResponseMessages` SHALL contain the produced response messages independently of the override
- **AND** `InitialMessages` SHALL remain unchanged

#### Scenario: Resumed approval results are initial response messages
- **WHEN** the initial input contains a tool approval response that causes a tool result to be generated before the first provider call
- **THEN** `PrepareStepState.InitialMessages` SHALL remain equal to the submitted model messages
- **AND** `PrepareStepState.ResponseMessages` SHALL contain the approval-generated tool-result message
- **AND** `PrepareStepState.Messages` SHALL contain both the submitted messages and the approval-generated result

#### Scenario: WithOutput sets structured output
- **WHEN** `WithOutput(out)` is passed
- **THEN** the output's response format overrides any explicit response format, and output processing is applied to results

### Requirement: PrepareStep initial and response history boundaries

`PrepareStepState.InitialMessages` SHALL exclude configured system messages and approval-generated tool results. `ResponseMessages` SHALL include initial approval-generated tool results followed by each completed step's response messages in order.

#### Scenario: PrepareStep initial and response history boundaries
- **WHEN** approval generates a tool result before the first provider call
- **THEN** initial messages SHALL exclude the generated result and configured system messages; response history SHALL start with the generated result before later step responses

### Requirement: Output package accepts functional options
`output.GenerateObject` SHALL have the signature `func GenerateObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.GenerateOption) (*ObjectResult[T], error)`. `output.StreamObject` SHALL have the signature `func StreamObject[T any](ctx context.Context, model provider.LanguageModel, out aisdk.Output, opts ...aisdk.StreamOption) *StreamObjectResult[T]`.

#### Scenario: GenerateObject with functional options
- **WHEN** `GenerateObject` is called with a model, output, and generation options
- **THEN** the output is injected, options are applied, and a typed result is returned

#### Scenario: StreamObject with functional options
- **WHEN** `StreamObject` is called with a model, output, and stream options
- **THEN** the output is injected, options are applied, and a typed streaming result is returned

### Requirement: StreamTextParams is removed
The `StreamTextParams` struct SHALL be removed from the public API. All functionality previously configured through `StreamTextParams` fields SHALL be available through the corresponding option functions.

#### Scenario: All StreamTextParams fields have option equivalents
- **WHEN** comparing the removed `StreamTextParams` fields to available option functions
- **THEN** every field (except `Model`, which becomes positional) has a corresponding `With*` or `On*` option function

### Requirement: CallOptions remains unchanged
The `provider.CallOptions` struct SHALL NOT be modified. The functional options pattern applies only to the user-facing orchestration API. Options are translated internally to `CallOptions` before calling the provider.

#### Scenario: Provider receives identical CallOptions
- **WHEN** equivalent configuration is expressed via functional options instead of `StreamTextParams`
- **THEN** the resulting `CallOptions` passed to `provider.LanguageModel.DoStream` is identical

### Requirement: Stop on any named tool call

The library SHALL provide `HasToolCall(toolNames ...string) StopCondition`. The condition SHALL report true when the most recent step contains a tool call whose name is any of `toolNames`. Tool calls in earlier steps SHALL NOT satisfy it, it SHALL report false when there are no steps, and with no names it SHALL never report true. A one-name call SHALL keep its previous behavior.

#### Scenario: Any of several names in the latest step

- **WHEN** the condition is `HasToolCall("search", "finalAnswer")` and the latest step called `weather` and `finalAnswer`
- **THEN** the condition reports true

#### Scenario: Name only in an earlier step

- **WHEN** the condition is `HasToolCall("finalAnswer")` and only an earlier step called `finalAnswer`
- **THEN** the condition reports false

#### Scenario: No names

- **WHEN** the condition is `HasToolCall()` and the latest step called any tool
- **THEN** the condition reports false
