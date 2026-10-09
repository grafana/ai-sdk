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

Without a bearer token, the provider SHALL SigV4-sign outbound POST requests with bodies using configured `aws.CredentialsProvider` credentials or the AWS SDK v2 default chain. Bodyless or non-POST requests MUST be unsigned. Per request, `WithSigningService` SHALL override host inference: `bedrock-mantle.<region>.api.aws` selects `bedrock-mantle`; other hosts select `bedrock`. Service resolution MUST NOT affect bearer authentication.

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

The provider SHALL translate `provider.CallOptions` into the AWS Bedrock Converse request shape (`system`, `messages`, `inferenceConfig`, `toolConfig`, `additionalModelRequestFields`, `additionalModelResponseFieldPaths`) before each call. When a user text or inline image part opts into selective guard content, its guarded form SHALL replace only that part's ordinary Converse content block.

#### Scenario: System messages

- **WHEN** the call's prompt begins with one or more `SystemMessage` parts
- **THEN** they are emitted under the `system` array as `{text: <content>}` blocks in order, before any user/assistant messages

#### Scenario: User text message

- **WHEN** the prompt contains a `UserMessage` with a text part without an enabled per-part `guardContent` option
- **THEN** the request includes `{role: "user", content: [{text: "<content>"}]}`

#### Scenario: Supported user document media type

- **WHEN** a user file part uses one of the supported document media types
- **THEN** the request emits a Converse document block using the mapping `application/pdf` to `pdf`, `text/csv` to `csv`, `application/msword` to `doc`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document` to `docx`, `application/vnd.ms-excel` to `xls`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` to `xlsx`, `text/html` to `html`, `text/plain` to `txt`, or `text/markdown` to `md`
- **AND** the document bytes are preserved and the filename-derived name is sanitized to the Bedrock-compatible form

#### Scenario: Top-level inline media type resolution

- **WHEN** an inline user file part supplies only the top-level media type `image` or `application`
- **THEN** request conversion detects the full media type from the inline bytes before selecting the image or document request shape
- **AND** recognized PNG and PDF signatures resolve to `image/png` and `application/pdf` respectively

#### Scenario: Inline text document data

- **WHEN** a user file part carries inline text data and its media type is not a full type/subtype value
- **THEN** request conversion treats the media type as `text/plain`, UTF-8 encodes the text as base64 document bytes, and sanitizes the filename-derived name
- **AND** a full unsupported media type still returns an error

#### Scenario: Document name sanitation and stable fallback

- **WHEN** a user or tool-result document has a filename such as `John's  report.txt`, a name over 200 characters, or `.txt`
- **THEN** Converse uses `Johns report`, the first 200 characters of the sanitized base name, or a generated `document-N` name respectively
- **AND** sanitation strips the extension, collapses whitespace, removes characters outside ASCII letters, digits, spaces, `()[]-`, trims and caps at 200 characters before the final trim; identical names remain stable for prompt caching

#### Scenario: Unsupported user document media type

- **WHEN** a user file part contains base64 data with an unsupported non-image media type such as `application/octet-stream`
- **THEN** request conversion returns an error identifying the unsupported media type before issuing an HTTP request
- **AND** the provider MUST NOT coerce the file to a `txt` document

#### Scenario: Unsupported user file data source

- **WHEN** a user file part carries unsupported URL data (for example, a non-S3 URL) or a provider reference
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

- **WHEN** the consumer sets `Temperature` outside `[0, 1]` for a model that accepts it and requires Bedrock temperature normalization
- **THEN** the request body clamps the value to the nearest bound and emits a `Warning{Type: "unsupported", Feature: "temperature", Details: "...clamped..."}`

#### Scenario: Newer Claude models reject sampling parameters

- **WHEN** a supported Claude model that rejects sampling parameters is called with temperature, topK, and topP, even with thinking disabled
- **THEN** the request omits all three and emits a model-specific unsupported warning for each; older models retain supported sampling settings

#### Scenario: OpenAI Converse models reject unsupported inference settings

- **WHEN** an OpenAI model is called with stop sequences, temperature, and topP
- **THEN** the request omits stop sequences for all OpenAI models and omits temperature and topP for non-GPT-OSS OpenAI models, with an unsupported warning per omitted field

### Requirement: Selective Converse guard-content per-part options

The Bedrock adapter SHALL accept typed user text and inline image `guardContent` controls under `amazonBedrock` or legacy `bedrock` in both DoGenerate and DoStream. Enabled text SHALL use `{guardContent:{text:{text:<part text>,qualifiers:<optional array>}}}`; enabled images SHALL use `{guardContent:{image:{format,source}}}`. Only enabled parts SHALL change; absent/false controls SHALL preserve ordinary conversion.

#### Scenario: Mixed protected and unprotected user text

- **WHEN** a user message has `guardContent:true` text with two valid qualifiers, followed by ordinary text and `guardContent:false` text, followed by guarded text with an explicitly empty qualifier array
- **THEN** the Converse request contains corresponding guarded, ordinary, ordinary and guarded text blocks in that order, with both qualifier arrays (including `[]`) retained and with no qualifiers on ordinary blocks

#### Scenario: Guarded inline image among ordinary image and text

- **WHEN** a user message contains an inline image with `guardContent:true`, an inline image with `guardContent:false` and an ordinary text part
- **THEN** the first image alone is wrapped under `guardContent.image` with identical format/base64 bytes, while the other image and text remain ordinary blocks in original order

#### Scenario: Legacy part options

- **WHEN** a user text or inline image part has `guardContent:true` under legacy `bedrock` only, or modern `amazonBedrock:null` with legacy `bedrock.guardContent:true`
- **THEN** that part is guarded

#### Scenario: Modern part options take precedence

- **WHEN** modern non-null `amazonBedrock.guardContent:false` and legacy `bedrock.guardContent:true` coexist on a user text or inline image part
- **THEN** the ordinary block is emitted without merging legacy controls
- **AND** a modern object containing only unrelated keys likewise suppresses legacy fallback

#### Scenario: Unrelated typed Bedrock options on target parts

- **WHEN** a user text or inline image part carries typed `FilePartOptions` with citations in its Bedrock namespace, or an inline image carries typed text-part qualifier options without an enabled image guard flag
- **THEN** conversion ignores controls unrelated to that part and emits its ordinary text or image block without a typed-option mismatch error
- **AND** recognized guard controls, if also present for that part, retain strict validation and modern/legacy precedence

#### Scenario: Typed guard controls on a non-target document

- **WHEN** a user document part carries typed image- or text-part guard options in its Bedrock namespace
- **THEN** it remains an ordinary document, with no `guardContent` and no typed-option mismatch error
- **AND** typed `FilePartOptions.Citations` on a document continue to enable citations

#### Scenario: Validate text controls even when guard flag is false

- **WHEN** a text part's recognized raw or typed guard control is not boolean, is explicitly null, or has non-array, non-string, null or unknown qualifier values (including when `guardContent:false`)
- **THEN** request conversion returns an error before either endpoint issues an HTTP request rather than silently emitting ordinary or guarded content

#### Scenario: Validate image controls without applying text qualifiers

- **WHEN** an inline image's recognized raw or typed guard control is not boolean or is explicitly null
- **THEN** request conversion returns an error before an HTTP request
- **AND** text-only qualifier properties do not turn image, document, video, S3 URL image, or tool-result image content into guarded content

#### Scenario: Existing non-user content and default shape

- **WHEN** selective controls are absent or false, or the prompt contains system, assistant, tool-result, document, video or S3 URL image parts
- **THEN** the existing request content/ordering and top-level `guardrailConfig` passthrough remain unchanged and no `guardContent` member is emitted for those parts, including when an image/text guard option is attached to a non-target part

#### Scenario: Both Converse endpoints

- **WHEN** the same guarded user prompt is sent via DoGenerate and DoStream
- **THEN** `/converse` and `/converse-stream` each receive the equivalent `messages[*].content[*].guardContent` shape before decoding the response

### Requirement: Unary guarded-input intervention fidelity

When a non-streaming Converse response reports `stopReason: guardrail_intervened` and a trace and usage, the provider SHALL expose the content-filter finish reason while preserving the provider trace and reported usage. An authentic upstream response fixture SHALL be used for parity verification; a recorded stream guardrail event SHALL NOT be treated as proof of selective guarded input.

#### Scenario: Registered upstream intervention response

- **WHEN** the exact registered `amazon-bedrock-guard-content-intervened.json` unary fixture is replayed for a guarded query text request with trace-enabled top-level guardrailConfig
- **THEN** the result has raw finish reason `guardrail_intervened`, unified `content-filter`, the reported response text, `providerMetadata.bedrock.trace.guardrail.inputAssessment` including the blocked topic policy, and the fixture's input/output/cache/total and raw usage values
- **AND** the provider request snapshot contains guarded query text and unchanged top-level guardrailConfig

### Requirement: Top-level Converse provider option pass-through

The provider SHALL pass additional raw `amazonBedrock` properties to the Converse request top level. Absent/null `amazonBedrock` SHALL fall back to legacy `bedrock`; non-null modern options SHALL take complete precedence without merging. `reasoningConfig`, `additionalModelRequestFields`, and `serviceTier` SHALL be excluded from direct pass-through so specialized conversion remains authoritative.

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

When the model is Anthropic on Bedrock (an ID containing `anthropic`, an explicit Anthropic constructor family, or an application inference-profile ARN with explicitly configured `reasoningConfig.budgetTokens`), the provider SHALL route Anthropic-specific options through `additionalModelRequestFields`. For non-Anthropic models, Anthropic-only options MUST be ignored and a warning emitted.

#### Scenario: Thinking enabled with budget tokens

- **WHEN** the model ID is `anthropic.claude-sonnet-4-5-20250929-v1:0` and provider options include `reasoningConfig.type = "enabled"` with `budgetTokens = 2048`
- **THEN** the request body's `additionalModelRequestFields.thinking` is `{type: "enabled", budget_tokens: 2048}` and `inferenceConfig.maxTokens` is increased by the budget

#### Scenario: Application inference profile budget enables Anthropic thinking

- **WHEN** an application inference-profile ARN is called with explicitly configured `reasoningConfig.type = enabled` and `budgetTokens = 1024` but no explicit family
- **THEN** the request contains `thinking: {type: "enabled", budget_tokens: 1024}` and does not warn that `budgetTokens` is unsupported

#### Scenario: Anthropic effort level

- **WHEN** the model ID is Anthropic on Bedrock and provider options set `maxReasoningEffort = "high"`
- **THEN** the request body's `additionalModelRequestFields.output_config.effort` is `"high"`

#### Scenario: Anthropic betas

- **WHEN** Anthropic provider tools require beta flags and the caller supplies `anthropicBeta`
- **THEN** the request body's `additionalModelRequestFields.anthropic_beta` contains caller betas followed by tool-required betas, overriding any conflicting raw pass-through `anthropic_beta`

#### Scenario: Anthropic-only options on non-Anthropic model

- **WHEN** the model ID is `mistral.mistral-large-2407-v1:0` and provider options include `budgetTokens`
- **THEN** the request emits a `Warning{Type: "unsupported", Feature: "budgetTokens", Details: "applies only to Anthropic models on Bedrock"}` and omits `thinking` from the request

#### Scenario: Anthropic thinking disables temperature/topP/topK

- **WHEN** thinking is enabled and the consumer sets `Temperature`, `TopP`, or `TopK`
- **THEN** each is dropped from `inferenceConfig` and a `Warning{Type: "unsupported", Feature: "<param>", Details: "not supported when thinking is enabled"}` is emitted

### Requirement: Root reasoning resolution for Anthropic models

For custom `provider.CallOptions.Reasoning` other than `none`, Anthropic Bedrock models SHALL select thinking from registered upstream capabilities, including explicit constructor family and profile ARNs with explicit reasoning budget. Adaptive models SHALL use effort; budget models SHALL derive a budget from maximum output tokens and add it to `inferenceConfig.maxTokens`.

#### Scenario: Adaptive-capable model receives adaptive thinking and effort

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-6-v1:0`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "adaptive"}`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL equal `high`
- **AND** the request SHALL NOT include a reasoning budget or budget-derived `inferenceConfig.maxTokens`

#### Scenario: Dated and regional IDs retain capability-derived budgets

- **WHEN** root reasoning is `high` for `us.anthropic.claude-sonnet-4-5-20250929-v1:0` or `anthropic.claude-opus-4-1-20250805-v1:0`
- **THEN** budget-token thinking uses the registered capability maxima of 64000 or 32000 respectively and the corresponding 38400 or 19200 high budget; explicit nonzero budget overrides the derived budget

#### Scenario: Raw zero budget retains explicit presence

- **WHEN** an application inference-profile ARN has raw `reasoningConfig = {type: "enabled", budgetTokens: 0}`
- **THEN** the provider identifies the Anthropic family and emits `thinking = {type: "enabled", budget_tokens: 0}` with default `inferenceConfig.maxTokens = 4096`
- **AND** a raw zero budget overrides a derived root-reasoning budget on an Anthropic model

#### Scenario: Older Opus models use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4.1 or another older Claude Opus 4.x model with a `32000` capability maximum
- **THEN** the reasoning budget SHALL equal `19200`, derived from that maximum

#### Scenario: Older model retains budget-token thinking

- **WHEN** root reasoning is `high` for `anthropic.claude-sonnet-4-5-20250929-v1:0`
- **THEN** `additionalModelRequestFields.thinking.type` SHALL equal `enabled`
- **AND** `additionalModelRequestFields.thinking.budget_tokens` SHALL equal `38400`
- **AND** `inferenceConfig.maxTokens` SHALL equal `42496`
- **AND** `additionalModelRequestFields.output_config.effort` SHALL be omitted

#### Scenario: Unknown Claude ID uses adaptive fallback

- **WHEN** root reasoning is `high` for an unrecognized Anthropic model ID containing `claude-` but not matching a known or legacy Claude family
- **THEN** the pinned capability fallback has a 128000-token maximum and the request uses adaptive thinking with `output_config.effort = high`, not a derived token budget

#### Scenario: Unknown non-Claude Anthropic profile uses conservative fallback

- **WHEN** root reasoning is `high` for an opaque non-Claude application inference-profile ARN with explicit Anthropic constructor family and no explicit budget
- **THEN** the pinned capability fallback has a 4096-token maximum and the request uses enabled budget-token thinking with a high-level budget derived from that maximum, not adaptive thinking

#### Scenario: Known legacy Claude retains conservative budget fallback

- **WHEN** root reasoning is `high` for a legacy Claude 3 model ID that does not match a newer Claude capability family
- **THEN** the pinned capability maximum is 4096 and the request uses enabled budget-token thinking rather than the unknown-Claude adaptive fallback

#### Scenario: Older Sonnet models use their capability maximum

- **WHEN** root reasoning is `high` for a non-adaptive Claude Sonnet 4.x model such as `anthropic.claude-sonnet-4-20250514-v1:0` (not Sonnet 4.6)
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum

#### Scenario: Opus 4 and 4.1 use their capability maximum

- **WHEN** root reasoning is `high` for Claude Opus 4 or 4.1, such as `anthropic.claude-opus-4-20250514-v1:0` or `anthropic.claude-opus-4-1-20250805-v1:0`
- **THEN** the reasoning budget SHALL equal `19200`, derived from a `32000` maximum

#### Scenario: Opus 4.5 uses its larger capability maximum

- **WHEN** root reasoning is `high` for `anthropic.claude-opus-4-5-20251101-v1:0`
- **THEN** the reasoning budget SHALL equal `38400`, derived from a `64000` maximum, without adaptive thinking

#### Scenario: Adaptive effort compatibility mapping

- **WHEN** root reasoning is `minimal` for an adaptive-capable Anthropic Bedrock model
- **THEN** `additionalModelRequestFields.output_config.effort` SHALL equal `low`
- **AND** the provider SHALL emit a compatibility warning for `reasoning`

#### Scenario: Provider-default reasoning is omitted

- **WHEN** root reasoning is unset or `provider-default`
- **THEN** the provider SHALL NOT derive thinking or effort configuration from root reasoning

#### Scenario: Reasoning none disables Anthropic thinking

- **WHEN** root reasoning is `none` for an Anthropic Bedrock model that does not support `between_tools` thinking
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

#### Scenario: Reasoning none on Sonnet 5.5 uses between_tools thinking

- **WHEN** root reasoning is `none` for `global.anthropic.claude-sonnet-5-5`, with or without a partial provider reasoning config
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "between_tools"}`
- **AND** the request SHALL NOT include a derived reasoning budget or effort

### Requirement: Native structured output for supported Anthropic models

For JSON with a schema, the provider SHALL select native `additionalModelRequestFields.output_config.format`, synthetic JSON tool, or JSON instruction using effective `structuredOutputMode`. Default `auto` native use SHALL require Anthropic family, reliable native support, and model capability, thinking, or explicit Anthropic family. Explicit `outputFormat` SHALL override the auto reliability gate for Anthropic schema responses.

#### Scenario: Native JSON output on supported Anthropic model

- **WHEN** the model supports structured output and `ResponseFormat.Type = json` with a schema
- **THEN** the request body includes `additionalModelRequestFields.output_config.format = {type: "json_schema", schema: <schema>}` and no synthetic `json` tool

#### Scenario: JSON-tool fallback on unsupported model

- **WHEN** the model does not support native structured output, there are no caller tools, and `ResponseFormat.Type = json` with a schema
- **THEN** the provider injects a synthetic tool named `json` with the schema as inputSchema, sets `toolChoice = required`, and translates the tool call into the final text in the response

#### Scenario: Opus 4.7/4.8 structured output with user tools

- **WHEN** Claude Opus 4.7 or 4.8 receives a JSON response schema, at least one user tool, and `auto` mode
- **THEN** the provider keeps the user tools selectable, omits `output_config.format`, and injects the JSON schema instruction into the system prompt

#### Scenario: Thinking does not override native-output rejection

- **WHEN** thinking is enabled for a model that rejects reliable native structured output in `auto`
- **THEN** thinking fields remain enabled while `output_config.format` stays absent

#### Scenario: JSON-tool mode removes supplied format

- **WHEN** `structuredOutputMode = jsonTool` and the caller supplies `additionalModelRequestFields.output_config` with `format` and `effort`
- **THEN** the synthetic `json` tool and required choice are emitted, the supplied `format` is removed, and `effort` is retained; the tool-use response is converted to JSON text in generate and stream

#### Scenario: Explicit output-format override

- **WHEN** an Anthropic model otherwise using a fallback in `auto` receives `structuredOutputMode = outputFormat` with a JSON schema
- **THEN** `output_config.format` contains the sanitized JSON schema and no synthetic JSON tool is added

#### Scenario: Mode namespace precedence

- **WHEN** `amazonBedrock.structuredOutputMode` or legacy `bedrock.structuredOutputMode` is set alongside `anthropic.structuredOutputMode`
- **THEN** the selected Bedrock namespace's setting takes precedence; `anthropic.structuredOutputMode` is only used when the selected Bedrock option does not set a mode
- **AND** the mode is not emitted as a top-level Converse property

#### Scenario: Explicit profile family selects native output

- **WHEN** an application inference-profile ARN uses explicit Anthropic constructor family, JSON schema, and `auto` mode
- **THEN** native JSON output is selected despite the unrecognized profile ID

### Requirement: Explicit Converse Anthropic model family

The Bedrock constructor SHALL allow `WithModelFamily` with the typed Anthropic family setting, without changing the public `provider.LanguageModel` interface. Anthropic identity SHALL also be inferred from an ID containing `anthropic` or from an application inference-profile ARN with an explicitly configured Bedrock reasoning budget; an opaque profile ARN without either signal SHALL NOT be presumed Anthropic.

#### Scenario: Opaque application profile with explicit family

- **WHEN** an opaque application inference-profile ARN is constructed with Anthropic model family
- **THEN** Anthropic thinking, tool preparation and additional response-field paths follow the Anthropic Converse route

#### Scenario: Opaque application profile without Anthropic signal

- **WHEN** an opaque application inference-profile ARN has neither explicit family nor explicitly configured budget
- **THEN** the request does not infer Anthropic-specific thinking or native structured output

### Requirement: Strict tools and effective Anthropic tool choice

The Converse adapter SHALL keep strict-tool support independent of native JSON support. It SHALL omit `strict` with an unsupported warning when the model disallows either boolean value or when `strict:true` has an object schema lacking `additionalProperties:false` in any nested branch. Boolean schemas SHALL be accepted. Unsupported Anthropic web search/fetch provider tools SHALL be filtered with warnings; Anthropic provider-tool choices SHALL route through additional fields.

#### Scenario: Separate Sonnet 4.6 strict and native gates

- **WHEN** a Sonnet 4.6 function tool with nested object schema containing `additionalProperties:false` requests `strict:true` and JSON `auto` response
- **THEN** strict remains on the tool while the JSON response uses the non-native route

#### Scenario: Unsupported strict on newest models

- **WHEN** an Opus 4.7/4.8, Opus 5, Fable 5, or Sonnet 5 tool requests `strict:false` or `strict:true`
- **THEN** `strict` is omitted and an unsupported strict warning identifies the tool and value

#### Scenario: Incompatible nested strict schema

- **WHEN** `strict:true` is set but any nested object in `properties`, `patternProperties`, `definitions`, `$defs`, `dependencies`, `items`, `anyOf`, `allOf`, `oneOf`, `if/then/else`, `not`, `contains`, or `propertyNames` lacks `additionalProperties:false`
- **THEN** `strict` is omitted with an unsupported schema-compatibility warning, rather than an error

#### Scenario: Parallel choice replaces Converse choice

- **WHEN** an Anthropic function-tool request has `anthropic.disableParallelToolUse:true` and choice `auto`, `required`, or named tool
- **THEN** `additionalModelRequestFields.tool_choice` contains the equivalent `auto`, `any`, or `tool` type with `disable_parallel_tool_use:true`, while `toolConfig.toolChoice` is absent

#### Scenario: None choice with function-only tools

- **WHEN** choice is `none` with only function tools
- **THEN** no parallel-disabling choice, Converse tool choice, or tool definitions are sent

#### Scenario: None choice with Anthropic provider tools

- **WHEN** choice is `none` and supported Anthropic provider tools are present, with or without function tools
- **THEN** the supported provider-tool definitions and any accompanying function-tool definitions remain in `toolConfig.tools` with their required betas, but neither `additionalModelRequestFields.tool_choice` nor `toolConfig.toolChoice` is sent

#### Scenario: All tools filtered out

- **WHEN** all configured tools are filtered out as unsupported
- **THEN** no parallel-disabling choice or inactive tool definitions are sent

#### Scenario: JSON response tool and Anthropic provider tools

- **WHEN** JSON-tool fallback is selected with caller tools, including Anthropic provider tools, and parallel disabling is requested
- **THEN** the `json` function tool is appended, the effective choice is required, and exactly one compatible choice location is emitted; supported provider tools keep their pinned schema and required beta flags

### Requirement: Converse OpenAI effort routing

The provider SHALL classify OpenAI IDs by anchored `openai.` with optional regional prefix, not arbitrary substring. GPT-OSS SHALL use flat `additionalModelRequestFields.reasoning_effort`; other OpenAI IDs SHALL use nested `additionalModelRequestFields.reasoning.effort`, preserving unrelated reasoning fields. Anthropic SHALL retain family routing; Nova 2 Lite SHALL use `reasoningConfig` with `type: enabled`.

#### Scenario: GPT-OSS versus newer regional OpenAI

- **WHEN** `maxReasoningEffort = medium` is sent to `openai.gpt-oss-120b-1:0` or `us.openai.gpt-5.2`
- **THEN** GPT-OSS sends `reasoning_effort: medium` and the newer OpenAI request sends `reasoning: {effort: medium}`, without sending the other effort shape

#### Scenario: Embedded OpenAI substring is not an OpenAI ID

- **WHEN** a custom model ID embeds `openai.` away from the anchored vendor position
- **THEN** portable reasoning is ignored with an unsupported warning rather than using an OpenAI-specific shape or an unrequested `reasoningConfig`

#### Scenario: Nova 2 Lite receives portable reasoning

- **WHEN** `us.amazon.nova-2-lite-v1:0` receives portable reasoning `medium`
- **THEN** the additional request fields contain `reasoningConfig: {type: "enabled", maxReasoningEffort: "medium"}`

#### Scenario: Other Nova models require explicit reasoning configuration

- **WHEN** `amazon.nova-micro-v1:0` receives portable reasoning `high` without an explicit `reasoningConfig`
- **THEN** no reasoning field is sent and an unsupported warning is returned; with an explicit config its fields are forwarded and merged

### Requirement: Converse option merge preserves cache and beta precedence

Converse prompt conversion SHALL preserve cache-point placement on system and user/assistant message blocks. Explicit `anthropicBeta` and required provider-tool betas SHALL override a conflicting raw `additionalModelRequestFields.anthropic_beta` after merging, while unrelated caller fields and derived `output_config.effort` SHALL be preserved unless a selected mode forbids `output_config.format`.

#### Scenario: Cache point remains at selected message boundary

- **WHEN** a system block and a user or assistant message have cache-point options under `amazonBedrock` or legacy `bedrock`
- **THEN** their cache points remain at the corresponding Converse content boundaries after structured-output and tool routing

#### Scenario: Explicit beta and tool-required beta win over raw pass-through

- **WHEN** raw `anthropic_beta` conflicts with `anthropicBeta` and a selected provider tool requires a beta
- **THEN** the additional request field carries caller betas followed by tool-required betas instead of the conflicting raw value

### Requirement: Mistral tool call id normalization

For Mistral models on Bedrock, the provider SHALL preserve valid 9-character alphanumeric tool call IDs and deterministically hash incompatible IDs into 9-character base62 values before sending or emitting them. Different IDs sharing an initial prefix SHALL not collapse to the same normalized value in the tested cases.

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

### Requirement: Converse Claude models that reject disabled thinking and forced tool use

For Anthropic Bedrock IDs containing `claude-sonnet-5-5`, the adapter SHALL follow `@ai-sdk/amazon-bedrock` 5.0.99 unless noted: required choice SHALL become auto; named choice SHALL become auto with only the named tool; each SHALL warn unsupported `toolChoice`. JSON schema responses SHALL use system JSON instruction instead of forced JSON tool in every mode unless native `outputFormat` is selected.

#### Scenario: Required tool choice on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` is called with two function tools and tool choice `required`
- **THEN** `toolConfig.toolChoice` SHALL be `auto` and both tools SHALL be sent
- **AND** an unsupported `toolChoice` warning SHALL be emitted

#### Scenario: Named tool choice on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` is called with tools `weather` and `search` and tool choice `tool` named `search`
- **THEN** `toolConfig.toolChoice` SHALL be `auto` and only `search` SHALL be sent

#### Scenario: JSON response without native output on Sonnet 5.5

- **WHEN** `global.anthropic.claude-sonnet-5-5` receives a JSON schema response with no caller tools and `structuredOutputMode` `jsonTool`
- **THEN** the JSON schema instruction SHALL be injected into the system prompt and no `json` tool or forced tool choice SHALL be sent

#### Scenario: between_tools effort limit

- **WHEN** provider options set `reasoningConfig: {type: "between_tools", maxReasoningEffort: "max"}` for `global.anthropic.claude-sonnet-5-5`
- **THEN** `additionalModelRequestFields.thinking` SHALL equal `{type: "between_tools"}` and `output_config.effort` SHALL equal `high`
- **AND** an unsupported warning for `providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort` SHALL be emitted

#### Scenario: Claude Sonnet 5 keeps forced tool use

- **WHEN** `anthropic.claude-sonnet-5` is called with tool choice `required`
- **THEN** `toolConfig.toolChoice` SHALL be `any`

### Requirement: Redacted reasoning continuation

The provider SHALL preserve native redactedContent as reasoning metadata under both amazonBedrock and bedrock. Streaming fragments SHALL accumulate per native block and be published as the complete opaque value on reasoning-end. Assistant replay SHALL select signature, then redactedContent, then legacy redactedData, preserving present empty raw values and signed whitespace. Existing public typed string options SHALL remain compatible.

#### Scenario: Fragmented opaque continuation
- **WHEN** one native reasoning block emits multiple redactedContent fragments
- **THEN** its final metadata SHALL contain their concatenation, not merely the last fragment
- **AND** replay SHALL reconstruct the native redactedContent value

### Requirement: Converse text guard qualifier domain

Guard qualifiers SHALL be accepted only on user text parts and SHALL be restricted to `grounding_source`, `query`, and `guard_content`.

#### Scenario: Converse text guard qualifier domain

- **WHEN** guarded user text supplies each of the three valid qualifiers
- **THEN** conversion SHALL preserve the qualifiers under `guardContent.text.qualifiers`; unknown qualifier values SHALL fail before HTTP, and image parts SHALL NOT apply text qualifiers

### Requirement: Converse adaptive Claude family selection

For Bedrock models identified as Anthropic, IDs containing `claude-opus-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-sonnet-4-6`, `claude-fable-5`, or `claude-sonnet-5` SHALL use adaptive thinking for custom root reasoning other than `none`.

#### Scenario: Converse adaptive Claude family selection

- **WHEN** a Bedrock model identified as Anthropic has root reasoning `high` on each listed adaptive Claude family
- **THEN** the request SHALL use adaptive thinking with effort `high` rather than a derived budget

### Requirement: Converse unknown and legacy Anthropic reasoning fallback

For Bedrock models identified as Anthropic with custom root reasoning other than `none`, unknown `claude-` IDs not matching known/legacy families SHALL use adaptive thinking with a pinned 128000-token maximum. Known older/legacy Claude SHALL use pinned budget-token maxima. Unknown non-Claude Anthropic IDs, including explicitly Anthropic opaque profiles, SHALL use budget thinking with a 4096-token maximum.

#### Scenario: Converse unknown and legacy Anthropic reasoning fallback

- **WHEN** custom root reasoning `high` is supplied for Bedrock models identified as Anthropic with unknown Claude, legacy Claude, and opaque non-Claude profile IDs
- **THEN** the unknown Claude SHALL use adaptive fallback; the legacy Claude and non-Claude profile SHALL use their pinned budget-based fallbacks

### Requirement: Converse adaptive effort mapping

Adaptive reasoning SHALL map `minimal`/`low` to `low`, `medium` to `medium`, `high` to `high`, and `xhigh` to `max` in `additionalModelRequestFields.output_config.effort`. Changed level names SHALL emit a compatibility warning.

#### Scenario: Converse adaptive effort mapping

- **WHEN** custom root reasoning is `minimal`, `low`, `medium`, `high`, or `xhigh` on an adaptive model
- **THEN** effort SHALL be `low`, `low`, `medium`, `high`, or `max` respectively; only changed names SHALL warn

### Requirement: Converse explicit reasoning configuration merge

For custom reasoning other than `none`, non-zero explicit `reasoningConfig` fields SHALL override derived fields; unspecified fields SHALL remain derived. Raw `budgetTokens:0` SHALL override to `thinking.budget_tokens=0`; a typed zero field with `omitempty` SHALL represent omission. Merged type `disabled` SHALL remove derived budget and effort.

#### Scenario: Converse explicit reasoning configuration merge

- **WHEN** root reasoning supplies a derived budget and raw config explicitly sets zero, or a typed config leaves budget zero
- **THEN** raw zero SHALL override the budget while typed zero SHALL leave it derived; merged disabled thinking SHALL clear derived budget and effort

### Requirement: Converse none thinking and pinned deviation

Anthropic root reasoning `none` SHALL replace explicit partial config with disabled thinking, except IDs containing `claude-sonnet-5-5`, which SHALL use `between_tools`. This intentional deviation from `@ai-sdk/amazon-bedrock` 5.0.99 is recorded in `test/conformance/upstream.yaml`: upstream disabled thinking omits `thinking`, allowing default-effort adaptive thinking.

#### Scenario: Converse none thinking and pinned deviation

- **WHEN** root reasoning `none` accompanies partial provider config on Sonnet 5.5 and on another Anthropic model
- **THEN** Sonnet 5.5 SHALL send between-tools thinking; the other model SHALL use disabled thinking with no derived budget or effort

### Requirement: Converse auto native-output exclusions

Claude Opus 4.7/4.8 and other pinned newest-model strict exclusions SHALL NOT use native output in `auto`, even with thinking. Sonnet 4.6 and Haiku 4.5 SHALL use fallback in `auto` regardless of thinking, retaining independent strict-tool support.

#### Scenario: Converse auto native-output exclusions

- **WHEN** an auto JSON-schema request enables thinking on Opus 4.7/4.8, Sonnet 4.6, or Haiku 4.5
- **THEN** the request SHALL omit native output format and select fallback without disabling supported strict tools

### Requirement: Converse parallel disabling choice location

With `anthropic.disableParallelToolUse` enabled and active Anthropic function/provider tools, the adapter SHALL put the effective choice with `disable_parallel_tool_use:true` in `additionalModelRequestFields.tool_choice`, without conflicting `toolConfig.toolChoice`.

#### Scenario: Converse parallel disabling choice location

- **WHEN** active Anthropic tools use required choice and parallel disabling
- **THEN** exactly the additional-fields choice SHALL be sent with type `any` and `disable_parallel_tool_use:true`

### Requirement: Converse reasoning on other model families

For non-OpenAI, non-Anthropic models other than Nova 2 Lite, portable reasoning without explicit provider `reasoningConfig` SHALL warn unsupported and SHALL NOT add a reasoning field. Explicit provider `reasoningConfig` SHALL still be forwarded and merged with portable reasoning.

#### Scenario: Converse reasoning on other model families

- **WHEN** Nova Micro receives portable reasoning first without and then with explicit provider reasoning config
- **THEN** the first request SHALL omit reasoning and warn; the second SHALL forward and merge the explicit config

### Requirement: Converse between-tools thinking extension

As a Go extension, `reasoningConfig.type` SHALL accept `between_tools`, sent as `thinking:{type:"between_tools"}` without `display` or `budget_tokens`, and treated as active thinking for sampling removal. It SHALL lower `maxReasoningEffort` `xhigh`/`max` to `high` with unsupported warning feature `providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort`. Upstream 5.0.99 has no such type.

#### Scenario: Converse between-tools thinking extension

- **WHEN** Sonnet 5.5 requests between-tools thinking with display, budget, sampling, and effort `xhigh` or `max`
- **THEN** thinking SHALL contain only its type, sampling SHALL be removed, and effort SHALL be capped to high with the specified warning
