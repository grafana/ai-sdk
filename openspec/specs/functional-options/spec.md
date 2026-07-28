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
The library SHALL provide `WithTools(ToolSet)`, `WithToolChoice(provider.ToolChoice)`, `WithActiveTools(...string)`, and `WithStopWhen(...StopCondition)` as shared options.

#### Scenario: WithTools configures available tools
- **WHEN** `WithTools` is passed with a `ToolSet`
- **THEN** the tools are converted to provider tools and included in `CallOptions`

#### Scenario: WithActiveTools filters available tools
- **WHEN** `WithActiveTools("search", "calculate")` is passed alongside `WithTools`
- **THEN** only the named tools are included in the `CallOptions` sent to the provider

#### Scenario: WithStopWhen configures stop conditions
- **WHEN** `WithStopWhen` is passed with one or more `StopCondition` values
- **THEN** the step loop evaluates those conditions to determine when to stop

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
`WithPrepareStep(PrepareStepFunc)` and `WithOutput(Output)` SHALL be shared options. Each `PrepareStepFunc` invocation SHALL receive a zero-based step number equal to the number of completed steps. Before each provider call, `PrepareStepState` SHALL expose the current provider messages in `Messages`, the original converted input messages in `InitialMessages`, and the ordered response messages accumulated before the current step in `ResponseMessages`. `InitialMessages` SHALL exclude configured system messages and approval-generated tool results. `ResponseMessages` SHALL include approval-generated tool results from the initial input followed by each completed step's response messages. A non-nil `PrepareStepResult.Messages` override SHALL become the current message base for that step and later steps, while later response messages continue to accumulate independently.

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
