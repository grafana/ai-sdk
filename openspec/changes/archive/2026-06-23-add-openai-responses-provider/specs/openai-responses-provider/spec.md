## ADDED Requirements

### Requirement: Provider construction and identity
The system SHALL provide a `providers/openai` Go module exposing
`NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel`
that returns a value implementing `provider.LanguageModel` backed by the OpenAI
Responses API via `github.com/openai/openai-go`. The model SHALL report
`SpecificationVersion() == "v4"`, `Provider() == "openai"`, and `ModelID()` equal
to the constructor `modelID`. The constructor SHALL accept functional options
including `WithRequestOptions(...)` to set transport-level concerns such as the
base URL and HTTP client for testing. Construction SHALL NOT panic and SHALL NOT
perform network calls.

#### Scenario: Construct a Responses model
- **WHEN** `NewResponses("test-key", "gpt-4o")` is called
- **THEN** it returns a non-nil `provider.LanguageModel`
- **AND** `Provider()` is `"openai"`, `ModelID()` is `"gpt-4o"`, and `SpecificationVersion()` is `"v4"`

#### Scenario: Base URL override for testing
- **WHEN** `NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL)))` is constructed
- **THEN** subsequent `DoGenerate`/`DoStream` calls target the overridden base URL

### Requirement: System message conversion
The provider SHALL convert system messages according to the resolved
`systemMessageMode`: `system` emits a `system` role input item, `developer`
emits a `developer` role input item, and `remove` drops the system message and
emits an `unsupported` warning. The mode SHALL default to `developer` for
reasoning models and `system` otherwise, and SHALL be overridable via the
`systemMessageMode` provider option.

#### Scenario: System mode emits system role
- **WHEN** the model resolves `systemMessageMode == system` and the prompt contains a system message
- **THEN** the request `input` contains an item with role `system` and the system text

#### Scenario: Developer mode for reasoning model
- **WHEN** the model id is a reasoning model and no explicit `systemMessageMode` is set, with a system message present
- **THEN** the request `input` contains an item with role `developer`

#### Scenario: Remove mode drops system message
- **WHEN** `systemMessageMode == remove` and a system message is present
- **THEN** the system message is not sent in `input`
- **AND** an `unsupported` warning for `system messages are removed for this model` is emitted

### Requirement: User and assistant message conversion
The provider SHALL convert user messages to `{ role: "user", content: [...] }`
input items mapping text parts to `input_text`, image parts to `input_image`
(via `image_url`, `file_id`, or data URI; honoring `imageDetail`), and file
parts to `input_file` (via `file_id`, `file_url`, or `filename` + `file_data`).
Assistant text parts SHALL be emitted as `output_text` content (or as an
`item_reference` when `store` is true and an item id is present). Unsupported
file media types SHALL emit a warning or error matching upstream behavior.

#### Scenario: User text and image
- **WHEN** a user message contains a text part and an image URL part
- **THEN** the input item content contains an `input_text` part and an `input_image` part with `image_url`

#### Scenario: PDF file part
- **WHEN** a user message contains an `application/pdf` file part with inline data
- **THEN** the input item content contains an `input_file` part carrying the file data and a derived filename

#### Scenario: Stored assistant text becomes an item reference
- **WHEN** `store` is true and an assistant text part carries an item id
- **THEN** the request emits an `item_reference` item for that id rather than inline `output_text`

### Requirement: Tool call and tool result conversion
The provider SHALL convert assistant tool-call parts to `function_call` items
(or built-in call items such as `local_shell_call`, `shell_call`,
`apply_patch_call`, `tool_search_call`, `custom_tool_call` when the corresponding
tool is present) and tool-role result parts to `function_call_output` (or the
matching built-in output item). Tool-call arguments SHALL serialize `undefined`
input to `"{}"`. When `store` is true and an item id is present, the provider
SHALL emit an `item_reference` instead of re-sending the call, except where
`previousResponseId` semantics dictate skipping.

#### Scenario: Function call round-trip
- **WHEN** an assistant message contains a tool-call with name `getWeather` and arguments and a following tool result is present
- **THEN** the request contains a `function_call` item and a `function_call_output` item sharing the same `call_id`

#### Scenario: Empty tool input serializes to empty object
- **WHEN** a tool-call part has no input
- **THEN** the serialized `arguments` is `"{}"`, not `"null"`

### Requirement: Reasoning conversion
The provider SHALL convert assistant reasoning parts to `reasoning` input items
carrying `encrypted_content` and `summary` entries, or to an `item_reference`
when `store` is true and a reasoning item id is present. When
`conversation`/`previousResponseId` is active and a reasoning id is present, the
reasoning item SHALL be skipped. Reasoning parts lacking an item id SHALL fall
back to `encrypted_content`, and when `store` is false, reasoning items lacking
encrypted content SHALL be filtered out with a warning.

#### Scenario: Stored reasoning becomes item reference
- **WHEN** `store` is true and a reasoning part carries an item id
- **THEN** the request emits a single `item_reference` for that reasoning id

#### Scenario: Non-stored reasoning without encrypted content is dropped
- **WHEN** `store` is false and a reasoning part has no encrypted content
- **THEN** the reasoning item is omitted from the request and an `unsupported` warning is emitted

### Requirement: Request parameters and structured output
The provider SHALL map `temperature`, `topP`, and `maxOutputTokens` to the
Responses request, and emit `unsupported` warnings (without erroring) for
`topK`, `seed`, `presencePenalty`, `frequencyPenalty`, and `stopSequences`. For
`responseFormat` of type `json` it SHALL set `text.format` to a `json_schema`
format (honoring `strictJsonSchema`, name, description, schema) or `json_object`
when no schema is provided.

#### Scenario: Unsupported sampling parameter warning
- **WHEN** `CallOptions` sets `seed`
- **THEN** the request omits a seed and an `unsupported` warning for `seed` is emitted

#### Scenario: JSON schema structured output
- **WHEN** `responseFormat` is `json` with a schema and name
- **THEN** the request `text.format` is a `json_schema` format carrying the schema, name, and `strict` flag

### Requirement: Model capability gating
The provider SHALL detect model capabilities via prefix matching mirroring
upstream `getOpenAILanguageModelCapabilities` (`isReasoningModel`,
`systemMessageMode`, `supportsFlexProcessing`, `supportsPriorityProcessing`,
`supportsNonReasoningParameters`). For reasoning models it SHALL strip
`temperature` and `topP` (unless reasoning effort is `none` and the model
supports non-reasoning parameters), emitting `unsupported` warnings. For
non-reasoning models it SHALL emit warnings when `reasoningEffort` or
`reasoningSummary` are set. It SHALL strip `serviceTier` `flex`/`priority` when
unsupported, with a warning.

#### Scenario: Temperature stripped on reasoning model
- **WHEN** the model is a reasoning model and `temperature` is set
- **THEN** the request omits `temperature` and emits an `unsupported` warning

#### Scenario: Flex tier unsupported
- **WHEN** `serviceTier == flex` on a model that does not support flex processing
- **THEN** the request omits `service_tier` and emits an `unsupported` warning

### Requirement: Provider options
The provider SHALL parse typed provider options under the `openai` key
(`provider.ResolveOption[OpenAIResponsesOptions]`) and apply them to the request,
including `previousResponseId`, `conversation`, `instructions`, `reasoningEffort`,
`reasoningSummary`, `truncation`, `store`, `metadata`, `include`, `maxToolCalls`,
`parallelToolCalls`, `serviceTier`, `textVerbosity`, `user`, `logprobs`,
`strictJsonSchema`, `systemMessageMode`, `forceReasoning`, `allowedTools`,
`promptCacheKey`, `promptCacheRetention`, `safetyIdentifier`,
`passThroughUnsupportedFiles`, and `contextManagement`. Setting both
`conversation` and `previousResponseId` SHALL emit an `unsupported` warning.

#### Scenario: previousResponseId continuation
- **WHEN** the `openai` provider option `previousResponseId` is set
- **THEN** the request includes `previous_response_id` with that value

#### Scenario: Conversation and previousResponseId conflict
- **WHEN** both `conversation` and `previousResponseId` are set
- **THEN** an `unsupported` warning is emitted

#### Scenario: Logprobs auto-include
- **WHEN** the `logprobs` option is set to a positive number
- **THEN** the request `include` contains `message.output_text.logprobs` and `top_logprobs` is set

### Requirement: Tool preparation and tool choice
The provider SHALL prepare function tools as `function` tool declarations and
provider tools by their OpenAI tool id (`openai.web_search`,
`openai.web_search_preview`, `openai.code_interpreter`, `openai.file_search`,
`openai.image_generation`, `openai.local_shell`, `openai.shell`,
`openai.apply_patch`, `openai.mcp`, `openai.tool_search`, `openai.custom`) into
the corresponding Responses tool objects. It SHALL resolve `toolChoice` of
`auto`/`none`/`required` as pass-through strings and `tool` as the typed/object
choice, with `allowedTools` overriding `toolChoice` as an `allowed_tools`
choice. Unknown tools SHALL emit an `unsupported` warning rather than erroring.

#### Scenario: Function tool declaration
- **WHEN** a function tool is provided
- **THEN** the request `tools` contains a `function` declaration with name, description, and parameters schema

#### Scenario: Web search tool auto-includes sources
- **WHEN** a `openai.web_search` provider tool is provided
- **THEN** the request `tools` contains a `web_search` tool and `include` contains `web_search_call.action.sources`

#### Scenario: allowedTools overrides tool choice
- **WHEN** the `allowedTools` option lists tool names and `toolChoice` is also set
- **THEN** the request `tool_choice` is an `allowed_tools` choice listing those tools

### Requirement: Non-streaming response conversion
`DoGenerate` SHALL convert every Responses output item to provider content:
`message` text (with annotations -> `source` parts), `reasoning` summaries,
`function_call` / `custom_tool_call` tool-calls, and provider-executed built-in
calls (`web_search_call`, `file_search_call`, `code_interpreter_call`,
`image_generation_call`, `local_shell_call`, `shell_call` + `shell_call_output`,
`apply_patch_call`, `tool_search_call` + `tool_search_output`, `computer_call`,
`mcp_call`, `mcp_approval_request`, `compaction`). It SHALL map usage and finish
reason, set provider metadata (`responseId`, logprobs, `serviceTier`), and carry
warnings.

#### Scenario: Text and url citation
- **WHEN** the response contains a `message` item with text and a `url_citation` annotation
- **THEN** the result contains a text content part and a `source` content part of type `url`

#### Scenario: Provider-executed web search
- **WHEN** the response contains a `web_search_call` item
- **THEN** the result contains a tool-call and a tool-result content part with `ProviderExecuted` set

#### Scenario: MCP approval request
- **WHEN** the response contains an `mcp_approval_request` item
- **THEN** the result contains a provider-executed dynamic tool-call and a `tool-approval-request` content part

#### Scenario: Finish reason with function call
- **WHEN** the response has no incomplete reason but contains a function call
- **THEN** the unified finish reason is `tool-calls`

### Requirement: Streaming response conversion
`DoStream` SHALL drive a stateful event consumer over the Responses SSE event
stream and emit `provider.StreamPart`s with ordering matching upstream:
`response.created` -> stream start + response metadata; message
`output_item.added`/`output_text.delta`/`output_item.done` -> text start/delta/end
(carrying phase and annotations); `function_call_arguments.delta`/`output_item.done`
-> tool-input start/delta/end + tool-call; reasoning summary events ->
reasoning start/delta/end; provider-executed tools -> their lifecycle parts with
`ProviderExecuted`; `response.completed`/`response.incomplete`/`response.failed`
-> finish with usage and finish reason; `error` -> error part. Unknown events
SHALL NOT error.

#### Scenario: Text streaming lifecycle
- **WHEN** the stream contains message added, two `output_text.delta` events, and message done
- **THEN** the parts are text-start, text-delta, text-delta, text-end in order

#### Scenario: Function call streaming
- **WHEN** the stream contains a function_call `output_item.added`, arguments deltas, and `output_item.done`
- **THEN** the parts are tool-input-start, tool-input-delta(s), tool-input-end, tool-call in order

#### Scenario: Web search streaming emits provider-executed call eagerly
- **WHEN** a `web_search_call` `output_item.added` event arrives
- **THEN** tool-input-start, tool-input-end, and a provider-executed tool-call are emitted before the result

#### Scenario: Unknown event is ignored
- **WHEN** an unrecognized SSE event type arrives
- **THEN** no error part is emitted and the stream continues

#### Scenario: Stream finish with usage
- **WHEN** a `response.completed` event arrives with usage
- **THEN** a finish part is emitted carrying mapped usage and the finish reason

### Requirement: Usage conversion
The provider SHALL convert Responses usage to `provider.Usage`: input tokens
total/noCache/cacheRead derived from `input_tokens` and
`input_tokens_details.cached_tokens`; output tokens total/text/reasoning derived
from `output_tokens` and `output_tokens_details.reasoning_tokens`; with the raw
usage retained.

#### Scenario: Cached and reasoning token split
- **WHEN** usage reports `input_tokens=100`, `cached_tokens=30`, `output_tokens=50`, `reasoning_tokens=20`
- **THEN** input noCache is 70, cacheRead is 30, output text is 30, output reasoning is 20

### Requirement: Error mapping
The provider SHALL translate OpenAI SDK API errors into
`provider.APICallError` carrying message, URL, request body, status code,
response headers, raw body, retryability, and parsed structured error data. In
streaming, errors SHALL be emitted as a `PartError` carrying an
`APICallError`; non-API errors during streaming SHALL also be wrapped as
`APICallError` parts. Construction and conversion SHALL never panic across the
API boundary.

#### Scenario: API error in DoGenerate
- **WHEN** the API returns a 400 error body
- **THEN** `DoGenerate` returns a `provider.APICallError` with status 400 and the parsed error data

#### Scenario: API error during streaming
- **WHEN** the API returns an error status while opening or reading a stream
- **THEN** the stream emits a `PartError` carrying an `APICallError`
