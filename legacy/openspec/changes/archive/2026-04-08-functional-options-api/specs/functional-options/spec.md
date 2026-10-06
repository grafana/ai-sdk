## ADDED Requirements

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
The library SHALL provide `WithProviderOptions(map[string]json.RawMessage)`, `WithHeaders(map[string]string)`, and `WithResponseFormat(provider.ResponseFormat)` as shared options.

#### Scenario: WithProviderOptions passes provider-specific config
- **WHEN** `WithProviderOptions` is passed with a provider options map
- **THEN** the options are forwarded to `CallOptions.ProviderOptions`

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
`WithPrepareStep(PrepareStepFunc)` and `WithOutput(Output)` SHALL be shared options.

#### Scenario: WithPrepareStep enables per-step configuration
- **WHEN** `WithPrepareStep(fn)` is passed
- **THEN** the function `fn` is called before each step to allow overriding model, messages, tool choice, active tools, and provider options

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
