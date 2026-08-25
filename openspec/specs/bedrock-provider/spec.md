## Purpose

Define the AWS Bedrock provider module, its `provider.LanguageModel` implementation driving the Bedrock Converse API, request/response conversion, authentication, streaming, error semantics, and registry integration.

## Requirements

### Requirement: Module location and naming

The Bedrock provider SHALL be implemented as a separate Go module located at `providers/bedrock/` with module path `github.com/grafana/ai-sdk/providers/bedrock`. It MUST NOT be a subpackage of the root `aisdk` module and MUST NOT depend on `providers/anthropic`.

#### Scenario: Module path

- **WHEN** a consumer runs `go get github.com/grafana/ai-sdk/providers/bedrock`
- **THEN** the module is fetched independently of the root `github.com/grafana/ai-sdk` module

#### Scenario: Dependency isolation

- **WHEN** a consumer imports only the root `aisdk` package
- **THEN** `github.com/aws/aws-sdk-go-v2` MUST NOT appear in their dependency graph

#### Scenario: Independent from anthropic module

- **WHEN** the `providers/bedrock` module is inspected
- **THEN** it MUST NOT import `github.com/grafana/ai-sdk/providers/anthropic` or any of its packages

### Requirement: LanguageModel interface implementation

The Bedrock provider SHALL implement `provider.LanguageModel` from `github.com/grafana/ai-sdk/provider`. The implementation MUST drive the AWS Bedrock Converse API (`/model/{modelID}/converse` for non-streaming, `/model/{modelID}/converse-stream` for streaming).

#### Scenario: Conformance with LanguageModel

- **WHEN** the provider returns a model from `New(modelID, opts...)`
- **THEN** the returned value implements `SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, and `DoGenerate`

#### Scenario: SpecificationVersion is v4

- **WHEN** a consumer calls `SpecificationVersion()` on a Bedrock model
- **THEN** the returned value is `"v4"`

#### Scenario: Provider name

- **WHEN** a consumer calls `Provider()` on a Bedrock model
- **THEN** the returned value is `"amazon-bedrock"`, matching the upstream provider key

### Requirement: Constructor and options

The provider SHALL expose a single constructor `New(modelID string, opts ...Option) provider.LanguageModel` that accepts functional options for region, credentials, base URL, SigV4 signing service, HTTP client, request headers, and ID generation.

#### Scenario: Basic construction

- **WHEN** a consumer calls `bedrock.New("anthropic.claude-sonnet-4-5-20250929-v1:0", bedrock.WithRegion("us-east-1"))`
- **THEN** the call returns a `provider.LanguageModel` whose `ModelID()` equals the supplied model ID

#### Scenario: Bearer token takes precedence over SigV4

- **WHEN** a consumer constructs the provider with both `WithBearerToken("token")` and credentials configured
- **THEN** outgoing requests carry `Authorization: Bearer token` and no SigV4 signature headers

#### Scenario: Custom HTTP client

- **WHEN** a consumer supplies `WithHTTPClient(client)` and makes a call
- **THEN** the request is dispatched through the supplied client

#### Scenario: Custom base URL

- **WHEN** a consumer supplies `WithBaseURL("https://custom.example.com")`
- **THEN** the provider issues requests against `https://custom.example.com/model/{modelID}/converse[-stream]` instead of the default AWS endpoint

#### Scenario: Custom signing service

- **WHEN** a consumer supplies `WithSigningService("bedrock-mantle")`
- **THEN** SigV4 signatures use `bedrock-mantle` as the credential-scope service name regardless of the endpoint host

#### Scenario: Application inference-profile ARN

- **WHEN** the model ID is an application inference-profile ARN
- **THEN** the Converse and ConverseStream request paths preserve the ARN `:` and `/` delimiters while ordinary model IDs remain URL-segment escaped

### Requirement: AWS authentication

When no bearer token is configured, the provider SHALL sign outbound `POST` requests using AWS Signature Version 4 with credentials obtained from a configured `aws.CredentialsProvider` or, if absent, the AWS SDK v2 default credential chain. Requests without a body or non-`POST` requests MUST be sent without signing.

The provider SHALL resolve the SigV4 credential-scope service name per request as follows: an explicit `WithSigningService` value takes precedence; otherwise, when the endpoint host is a Bedrock Mantle host (`bedrock-mantle.<region>.api.aws`) the service name SHALL be `bedrock-mantle`; otherwise the service name SHALL be `bedrock`. The resolved service name MUST NOT affect bearer-token authentication.

#### Scenario: Default credential chain

- **WHEN** the consumer constructs the provider without `WithCredentials` or `WithBearerToken`
- **THEN** the provider uses AWS SDK v2's default credential resolution (env vars, shared config, EC2/IRSA, etc.) at request time

#### Scenario: Explicit credentials provider

- **WHEN** the consumer passes `WithCredentials(cp)` where `cp` is an `aws.CredentialsProvider`
- **THEN** the provider signs requests using credentials returned by `cp.Retrieve(ctx)` for the configured region

#### Scenario: Bearer token via environment

- **WHEN** the env var `AWS_BEARER_TOKEN_BEDROCK` is set and `WithBearerToken` is not used
- **THEN** the provider sends `Authorization: Bearer <env value>` and skips SigV4

#### Scenario: SigV4 service and region

- **WHEN** the provider signs a request for the default Bedrock Runtime endpoint in region `us-east-1` without a signing-service override
- **THEN** the signature uses service name `bedrock` and the configured region

#### Scenario: Mantle endpoint infers bedrock-mantle service

- **WHEN** `WithBaseURL` targets a Bedrock Mantle host (`bedrock-mantle.<region>.api.aws`) and no signing-service override is configured
- **THEN** the signature uses service name `bedrock-mantle` and the configured region

#### Scenario: Explicit signing service overrides host inference

- **WHEN** a signing-service override is configured via `WithSigningService`
- **THEN** the signature uses the overriding service name even when the endpoint host would otherwise infer a different service (for example, a Mantle host forced to `bedrock`, or a non-Mantle proxy host forced to `bedrock-mantle`)

### Requirement: Request conversion to Converse format

The provider SHALL translate `provider.CallOptions` into the AWS Bedrock Converse request shape (`system`, `messages`, `inferenceConfig`, `toolConfig`, `additionalModelRequestFields`, `additionalModelResponseFieldPaths`) before each call.

#### Scenario: System messages

- **WHEN** the call's prompt begins with one or more `SystemMessage` parts
- **THEN** they are emitted under the `system` array as `{text: <content>}` blocks in order, before any user/assistant messages

#### Scenario: User text message

- **WHEN** the prompt contains a `UserMessage` with a text part
- **THEN** the request includes `{role: "user", content: [{text: "<content>"}]}`

#### Scenario: Supported user document media type

- **WHEN** a user file part uses one of the supported document media types
- **THEN** the request emits a Converse document block using the mapping `application/pdf` to `pdf`, `text/csv` to `csv`, `application/msword` to `doc`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document` to `docx`, `application/vnd.ms-excel` to `xls`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` to `xlsx`, `text/html` to `html`, `text/plain` to `txt`, or `text/markdown` to `md`
- **AND** the document bytes and filename-derived name are preserved

#### Scenario: Top-level inline media type resolution

- **WHEN** an inline user file part supplies only the top-level media type `image` or `application`
- **THEN** request conversion detects the full media type from the inline bytes before selecting the image or document request shape
- **AND** recognized PNG and PDF signatures resolve to `image/png` and `application/pdf` respectively

#### Scenario: Inline text document data

- **WHEN** a user file part carries inline text data and its media type is not a full type/subtype value
- **THEN** request conversion treats the media type as `text/plain`, UTF-8 encodes the text as base64 document bytes, and preserves the filename-derived name
- **AND** a full unsupported media type still returns an error

#### Scenario: Unsupported user document media type

- **WHEN** a user file part contains base64 data with an unsupported non-image media type such as `application/octet-stream`
- **THEN** request conversion returns an error identifying the unsupported media type before issuing an HTTP request
- **AND** the provider MUST NOT coerce the file to a `txt` document

#### Scenario: Unsupported user file data source

- **WHEN** a user file part carries URL data or a provider reference
- **THEN** request conversion returns an unsupported-functionality error before issuing an HTTP request
- **AND** the provider MUST NOT silently drop the file or degrade the error to a warning

#### Scenario: Supported image media types

- **WHEN** a user file part carries inline image data
- **THEN** only `image/jpeg`, `image/png`, `image/gif`, and `image/webp` are accepted as Bedrock image formats
- **AND** non-upstream aliases such as `image/jpg` and unsupported formats such as `image/avif` return an error

#### Scenario: Final assistant prefill whitespace

- **WHEN** the prompt ends with an assistant block whose final message's final content part is text containing surrounding whitespace
- **THEN** that final text is trimmed before it is emitted to Converse, while every other retained text part and signed reasoning text are emitted byte-for-byte unchanged

#### Scenario: Assistant tool call

- **WHEN** the prompt contains an `AssistantMessage` with a `ToolCallPart` of name `lookup` and input `{"q": "x"}`
- **THEN** the assistant message content includes `{toolUse: {toolUseId, name: "lookup", input: {"q": "x"}}}`

#### Scenario: Tool result message

- **WHEN** the prompt contains a `ToolMessage` with a tool result for tool call id `abc`
- **THEN** the request emits a user-role message with `{toolResult: {toolUseId: "abc", content: [...]}}` content (Converse collapses tool results into the user role)

#### Scenario: Inference config mapping

- **WHEN** the consumer sets `MaxOutputTokens=512`, `Temperature=0.5`, `TopP=0.9`, `TopK=20`, `StopSequences=["END"]`
- **THEN** the request body's `inferenceConfig` is `{maxTokens: 512, temperature: 0.5, topP: 0.9, topK: 20, stopSequences: ["END"]}`

#### Scenario: Tools and tool choice

- **WHEN** the consumer supplies function tools and `ToolChoice = required`
- **THEN** the request body includes `toolConfig.tools` with each tool's name, description, and JSON schema, and `toolConfig.toolChoice = {any: {}}`

#### Scenario: Specific tool choice

- **WHEN** the consumer sets `ToolChoice` to a specific tool named `weather`
- **THEN** the request body includes `toolConfig.toolChoice = {tool: {name: "weather"}}`

#### Scenario: Auto tool choice

- **WHEN** the consumer sets `ToolChoice = auto`
- **THEN** the request body includes `toolConfig.toolChoice = {auto: {}}`

#### Scenario: Unsupported parameters produce warnings

- **WHEN** the consumer sets `FrequencyPenalty`, `PresencePenalty`, or `Seed`
- **THEN** the call returns a `Warning{Type: "unsupported", Feature: "<param>"}` for each unset Converse field and omits it from the request

#### Scenario: Temperature clamping

- **WHEN** the consumer sets `Temperature` outside `[0, 1]`
- **THEN** the request body clamps the value to the nearest bound and emits a `Warning{Type: "unsupported", Feature: "temperature", Details: "...clamped..."}`

### Requirement: Top-level Converse provider option pass-through

The provider SHALL preserve additional properties from raw provider options under `amazonBedrock` and emit them at the top level of the Converse request. It SHALL also accept the legacy `bedrock` namespace when `amazonBedrock` is absent or null. A non-null `amazonBedrock` value SHALL take precedence as a complete namespace without merging legacy properties. The provider SHALL exclude `reasoningConfig`, `additionalModelRequestFields`, and `serviceTier` from direct pass-through so their specialized request conversions remain authoritative.

#### Scenario: Guardrail configuration pass-through

- **WHEN** raw `amazonBedrock` provider options include `guardrailConfig` with identifier, version, trace, and stream processing mode fields
- **THEN** the request body includes the unchanged `guardrailConfig` object at the top level

#### Scenario: Legacy namespace pass-through

- **WHEN** `amazonBedrock` is absent and raw `bedrock` provider options include an additional top-level Converse property
- **THEN** the request body includes that property unchanged

#### Scenario: Modern namespace precedence

- **WHEN** both namespaces contain additional top-level Converse properties and `amazonBedrock` is non-null
- **THEN** only properties from `amazonBedrock` are used and legacy properties are not merged

#### Scenario: Null modern namespace falls back to legacy

- **WHEN** `amazonBedrock` is null and `bedrock` contains provider options
- **THEN** the request uses the legacy provider options

#### Scenario: Active tool configuration remains authoritative

- **WHEN** pass-through options contain `toolConfig` and the call has active tools
- **THEN** the generated `toolConfig` for the active tools overrides the pass-through value

### Requirement: Anthropic-specific pass-through via additionalModelRequestFields

When the model ID identifies an Anthropic model on Bedrock (model ID contains `anthropic`), the provider SHALL route Anthropic-specific options through `additionalModelRequestFields`. For non-Anthropic models, Anthropic-only options MUST be ignored and a warning emitted.

#### Scenario: Thinking enabled with budget tokens

- **WHEN** the model ID is `anthropic.claude-sonnet-4-5-20250929-v1:0` and provider options include `reasoningConfig.type = "enabled"` with `budgetTokens = 2048`
- **THEN** the request body's `additionalModelRequestFields.thinking` is `{type: "enabled", budget_tokens: 2048}` and `inferenceConfig.maxTokens` is increased by the budget

#### Scenario: Anthropic effort level

- **WHEN** the model ID is Anthropic on Bedrock and provider options set `maxReasoningEffort = "high"`
- **THEN** the request body's `additionalModelRequestFields.output_config.effort` is `"high"`

#### Scenario: Anthropic betas

- **WHEN** Anthropic provider tools require beta flags
- **THEN** the request body's `additionalModelRequestFields.anthropic_beta` includes the union of caller-supplied and tool-required betas

#### Scenario: Anthropic-only options on non-Anthropic model

- **WHEN** the model ID is `mistral.mistral-large-2407-v1:0` and provider options include `budgetTokens`
- **THEN** the request emits a `Warning{Type: "unsupported", Feature: "budgetTokens", Details: "applies only to Anthropic models on Bedrock"}` and omits `thinking` from the request

#### Scenario: Anthropic thinking disables temperature/topP/topK

- **WHEN** thinking is enabled and the consumer sets `Temperature`, `TopP`, or `TopK`
- **THEN** each is dropped from `inferenceConfig` and a `Warning{Type: "unsupported", Feature: "<param>", Details: "not supported when thinking is enabled"}` is emitted

### Requirement: Root reasoning resolution for Anthropic models

When `provider.CallOptions.Reasoning` is a custom level other than `none` and the Bedrock model ID identifies an Anthropic model, the provider SHALL select thinking behavior from the registered upstream model capability set. Models whose IDs contain `claude-opus-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-fable-5`, or `claude-sonnet-5` SHALL use adaptive thinking; older and unknown Anthropic models SHALL use budget-token thinking.

For adaptive models, reasoning levels SHALL map to `additionalModelRequestFields.output_config.effort` as follows: `minimal` to `low`, `low` to `low`, `medium` to `medium`, `high` to `high`, and `xhigh` to `max`. A mapping that changes the level name SHALL emit a compatibility warning. For budget-based models, the provider SHALL derive a token budget from the model's maximum output tokens and increase `inferenceConfig.maxTokens` by that budget.

For custom reasoning other than `none`, non-zero fields from an explicit provider `reasoningConfig` SHALL override the corresponding derived fields while unspecified fields remain derived. If the merged type is `disabled`, derived budget and effort SHALL be removed. Anthropic root reasoning `none` SHALL replace an explicit partial reasoning config with disabled thinking.

#### Scenario: Adaptive-capable model receives adaptive thinking and effort

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-6-v1:0`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "adaptive"}`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`
- **AND** the request SHALL NOT include a reasoning budget or budget-derived `inferenceConfig.maxTokens`

#### Scenario: Older model retains budget-token thinking

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-5-20250929-v1:0`
- **THEN** `additionalModelRequestFields.thinking.type` SHALL equal `enabled`
- **AND** `additionalModelRequestFields.thinking.budget_tokens` SHALL equal `38400`
- **AND** `inferenceConfig.maxTokens` SHALL equal `42496`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL be omitted

#### Scenario: Older Sonnet models use their capability maximum

- **WHEN** root reasoning is `high` for another Claude Sonnet 4.x model
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum

#### Scenario: Older Opus models use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4.1 or another Claude Opus 4.x model
- **THEN** the reasoning budget SHALL equal `19200`, derived from a `32000` maximum

#### Scenario: Adaptive effort compatibility mapping

- **WHEN** root reasoning is `minimal` for an adaptive-capable Anthropic Bedrock model
- **THEN** `additionalModelRequestFields.output_config.effort` SHALL equal `low`
- **AND** the provider SHALL emit a compatibility warning for `reasoning`

#### Scenario: Provider-default reasoning is omitted

- **WHEN** root reasoning is unset or `provider-default`
- **THEN** the provider SHALL NOT derive thinking or effort configuration from root reasoning

#### Scenario: Reasoning none disables Anthropic thinking

- **WHEN** root reasoning is `none` for an Anthropic Bedrock model
- **THEN** the derived reasoning configuration SHALL disable thinking
- **AND** the request SHALL NOT include a derived reasoning budget or effort

#### Scenario: Partial provider config preserves adaptive derivation

- **WHEN** root reasoning is `high` for an adaptive-capable Anthropic model and provider `reasoningConfig.display` is `summarized`
- **THEN** the request SHALL use adaptive thinking with display `summarized`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`

#### Scenario: Explicit enabled config retains derived effort

- **WHEN** root reasoning is `high` for an adaptive-capable Anthropic model and provider reasoning config sets type `enabled` with a token budget
- **THEN** the request SHALL use the explicit enabled type and token budget
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`

#### Scenario: Disabled provider config clears derived values

- **WHEN** custom root reasoning is combined with provider reasoning config type `disabled`
- **THEN** the request SHALL omit derived reasoning budget and effort

#### Scenario: Reasoning none overrides partial provider config

- **WHEN** root reasoning is `none` for an Anthropic model and provider reasoning config only sets display
- **THEN** the request SHALL omit thinking and effort fields

### Requirement: Native structured output for supported Anthropic models

When the consumer requests JSON response format with a schema and the model supports native structured output, the provider SHALL configure native JSON-schema output via `additionalModelRequestFields.output_config.format` instead of injecting a synthetic JSON tool. Models known to reject that field, including Claude Opus 4.7 and 4.8, SHALL not use native structured output even when thinking is enabled.

#### Scenario: Native JSON output on supported Anthropic model

- **WHEN** the model supports structured output and `ResponseFormat.Type = json` with a schema
- **THEN** the request body includes `additionalModelRequestFields.output_config.format = {type: "json_schema", schema: <schema>}` and no synthetic `json` tool

#### Scenario: JSON-tool fallback on unsupported model

- **WHEN** the model does not support native structured output and `ResponseFormat.Type = json` with a schema
- **THEN** the provider injects a synthetic tool named `json` with the schema as inputSchema, sets `toolChoice = required`, and translates the tool call into the final text in the response

#### Scenario: Opus 4.7/4.8 structured output with user tools

- **WHEN** Claude Opus 4.7 or 4.8 receives a JSON response schema and at least one user tool
- **THEN** the provider keeps the user tools selectable, omits `output_config.format`, and injects the JSON schema instruction into the system prompt

#### Scenario: Thinking does not override native-output rejection

- **WHEN** thinking is enabled for a model that rejects native structured output
- **THEN** thinking fields remain enabled while `output_config.format` stays absent

### Requirement: Mistral tool call id normalization

For Mistral models on Bedrock, the provider SHALL normalize tool call IDs to match Mistral's expectations (no underscores, length-bounded numeric form) before emitting them downstream.

#### Scenario: Mistral tool call id

- **WHEN** the model ID starts with `mistral.` and a tool call ID contains characters Mistral does not accept
- **THEN** the emitted `toolCallId` is normalized via the same algorithm used by upstream `normalizeToolCallId`

### Requirement: Non-streaming response conversion

For `DoGenerate`, the provider SHALL decode the JSON response body and convert it into `provider.GenerateResult` with `Content`, `FinishReason`, `Usage`, `Response`, and optional `ProviderMetadata`.

#### Scenario: Text response

- **WHEN** the response `output.message.content` contains a `{text: "hello"}` block
- **THEN** the `Content` array includes a `ContentPart{Type: text, Text: "hello"}`

#### Scenario: Tool call response

- **WHEN** the response contains a `{toolUse: {toolUseId, name, input}}` block
- **THEN** the `Content` array includes a `ToolCallPart` with the same id, name, and JSON-stringified input

#### Scenario: Reasoning content with signature

- **WHEN** the response contains `reasoningContent.reasoningText` with `text` and `signature`
- **THEN** the `Content` array includes a reasoning part carrying the text, and provider metadata records the signature under `amazonBedrock`

#### Scenario: Redacted reasoning

- **WHEN** the response contains `reasoningContent.redactedReasoning.data`
- **THEN** the `Content` array includes a reasoning part with empty text and provider metadata carrying `redactedData`

#### Scenario: Usage with cache tokens

- **WHEN** the response usage has `inputTokens=10`, `outputTokens=20`, `cacheReadInputTokens=3`, `cacheWriteInputTokens=5`
- **THEN** `Usage.InputTokens.Total = 18`, `noCache = 10`, `cacheRead = 3`, `cacheWrite = 5`, and `OutputTokens.Total = 20`

#### Scenario: Finish reason mapping

- **WHEN** the response `stopReason` is `end_turn`, `max_tokens`, `tool_use`, `content_filtered`, or `guardrail_intervened`
- **THEN** the `FinishReason.Unified` is `stop`, `length`, `tool-calls`, `content-filter`, `content-filter` respectively

### Requirement: Streaming response decoding via Smithy event stream

For `DoStream`, the provider SHALL decode the AWS Smithy event-stream binary response, extracting `(:event-type, JSON payload)` pairs from each frame, and emit corresponding `provider.StreamPart` values on a buffered channel.

#### Scenario: Channel buffer size

- **WHEN** `DoStream` returns a `StreamResult`
- **THEN** the channel buffer is at least 64 elements

#### Scenario: Text delta

- **WHEN** the stream emits `contentBlockDelta` with `delta.text = "foo"` at index `0`
- **THEN** the channel receives a `PartTextDelta` with `Delta: "foo"` and ID derived from block index `0`

#### Scenario: Tool input streaming

- **WHEN** the stream emits `contentBlockStart` with `toolUse{toolUseId, name}` followed by one or more `contentBlockDelta` with `toolUse.input` fragments and a final `contentBlockStop`
- **THEN** the channel receives `PartToolInputStart`, `PartToolInputDelta` for each fragment, and `PartToolInputEnd` carrying the accumulated JSON

#### Scenario: Reasoning delta

- **WHEN** the stream emits `contentBlockDelta` with `delta.reasoningContent.text`
- **THEN** the channel receives a `PartReasoningDelta` with the text fragment

#### Scenario: Finish reason and usage

- **WHEN** the stream emits `messageStop` with `stopReason` followed by `metadata` with `usage`
- **THEN** the channel emits a finish part carrying the mapped unified finish reason and a usage part built from the metadata

#### Scenario: Stream-level exception

- **WHEN** the stream emits a frame whose `:exception-type` is `throttlingException`
- **THEN** the channel receives a `PartError` carrying a `*provider.APICallError` with `IsRetryable = true`, and is then closed

#### Scenario: Mid-stream transport failure

- **WHEN** the stream fails after a 2xx response has begun
- **THEN** the channel emits a final `PartError` with a synthesized retryable `*provider.APICallError` and is then closed

#### Scenario: Context cancellation

- **WHEN** the call's context is cancelled mid-stream
- **THEN** the provider cancels the underlying HTTP request and closes the channel without panicking

### Requirement: Error handling preserves retry semantics

The provider SHALL surface server and transport errors as `*provider.APICallError` with correct `IsRetryable` semantics so `aisdk.StreamText`'s retry layer behaves identically to direct Anthropic provider behavior.

#### Scenario: Throttling is retryable

- **WHEN** the API returns HTTP 429 or a `throttlingException` event
- **THEN** the surfaced `APICallError` has `IsRetryable = true`

#### Scenario: 5xx is retryable

- **WHEN** the API returns HTTP 500 or 503, or an `internalServerException` event
- **THEN** the surfaced `APICallError` has `IsRetryable = true`

#### Scenario: Validation errors are not retryable

- **WHEN** the API returns HTTP 400 or a `validationException` event
- **THEN** the surfaced `APICallError` has `IsRetryable = false`

#### Scenario: Error body decoded into APICallError

- **WHEN** the API returns a JSON error body like `{"message": "...", "type": "ValidationException"}`
- **THEN** the surfaced `APICallError.Message` includes the upstream message and `StatusCode` matches the HTTP status

### Requirement: Registry integration

The Bedrock provider package SHALL expose a constructor returning a value that satisfies `registry.Provider` from `github.com/grafana/ai-sdk/registry`. Consumers MUST be able to register it under any provider id and resolve `<id>:<modelID>` model identifiers.

#### Scenario: Composite ID resolution

- **WHEN** the consumer registers the provider as `"bedrock"` and asks for `"bedrock:anthropic.claude-sonnet-4-5-20250929-v1:0"`
- **THEN** the registry returns a `provider.LanguageModel` whose `ModelID()` is `"anthropic.claude-sonnet-4-5-20250929-v1:0"`

#### Scenario: Compose with middleware

- **WHEN** the consumer composes the provider with `registry.WithLanguageModelMiddleware`
- **THEN** middleware wraps Bedrock provider models the same way it wraps any other provider

### Requirement: Identity reporting

The provider's `Provider()` method SHALL return `"amazon-bedrock"`. Its `ModelID()` SHALL return the exact `modelID` passed to `New(modelID, ...)`.

#### Scenario: Stable provider name

- **WHEN** a consumer calls `Provider()` on a Bedrock model
- **THEN** the returned identifier is `"amazon-bedrock"`, useable for logging and telemetry

#### Scenario: Pass-through model id

- **WHEN** a consumer constructs `bedrock.New("amazon.nova-lite-v1:0")`
- **THEN** `ModelID()` returns `"amazon.nova-lite-v1:0"` verbatim
