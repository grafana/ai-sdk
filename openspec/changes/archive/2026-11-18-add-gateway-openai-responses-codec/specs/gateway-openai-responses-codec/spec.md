# Spec Delta

## ADDED Requirements

### Requirement: Responses request subset
The codec SHALL accept a `POST /v1/responses` JSON body with only these fields: `model`, `input`, `stream`, `tools`, `tool_choice`, `parallel_tool_calls`, `reasoning`, `text`, `store` (absent or `false`), `include` (only `reasoning.encrypted_content`), `prompt_cache_key` and `client_metadata`. It SHALL refuse any other field with a fixed `400` error document and SHALL NOT drop a field silently.

#### Scenario: Codex request fields
- **WHEN** a request carries `store: false`, `include: ["reasoning.encrypted_content"]`, `prompt_cache_key` and `client_metadata`
- **THEN** the codec SHALL accept it
- **AND** `prompt_cache_key` and `client_metadata` SHALL NOT change the provider call

#### Scenario: Refused fields
- **WHEN** a request carries `previous_response_id`, `store: true`, `instructions`, `max_output_tokens` or any other unlisted field
- **THEN** the codec SHALL respond `400` with the fixed `invalid_request` document and SHALL NOT call a provider

### Requirement: Responses input items
The codec SHALL accept `input` as a string or as a list of `message`, `function_call`, `function_call_output` and `reasoning` items. Message content SHALL be `input_text`, `output_text` or `input_image` parts. `system` and `developer` messages SHALL become provider system messages in their input position. Every `function_call_output` SHALL answer an earlier `function_call` of the same `call_id`. Other items and parts SHALL be refused.

#### Scenario: Developer message is the system prompt
- **WHEN** the first input item is a `developer` message followed by a `user` message
- **THEN** the provider prompt SHALL start with a system message holding the developer text

#### Scenario: Function call continuation
- **WHEN** input holds a `function_call` and a `function_call_output` with the same `call_id`
- **THEN** the provider prompt SHALL hold an assistant tool call and a tool result for that call

#### Scenario: Image parts
- **WHEN** a user message holds an `input_image` part
- **THEN** the codec SHALL accept an `https`, `http` or base64 image data URL with `detail` absent or `auto`, and refuse any other URL or detail

#### Scenario: Refused input items
- **WHEN** input holds a `custom_tool_call`, a `custom_tool_call_output`, an `input_file` part or a `function_call_output` with no matching call
- **THEN** the codec SHALL respond `400` with the fixed `invalid_request` document

#### Scenario: Reasoning replay
- **WHEN** input holds a `reasoning` item with summaries and `encrypted_content`
- **THEN** the summaries SHALL become reasoning parts of an assistant message and `encrypted_content` SHALL NOT be applied

### Requirement: Responses tools and settings
The codec SHALL accept only `function` tools with a valid unique name, `tool_choice` of `auto`, `none`, `required` or a named declared function, `reasoning.effort` and `reasoning.summary`, and `text.format` of `text`, `json_object` or `json_schema` with `text.verbosity`. It SHALL refuse `custom` tools, hosted tools such as `web_search`, and every other tool type.

#### Scenario: Hosted and custom tools
- **WHEN** a request declares a `web_search` tool or a `custom` tool such as `apply_patch`
- **THEN** the codec SHALL respond `400` with the fixed `invalid_request` document

#### Scenario: Backend-specific settings
- **WHEN** a request with `parallel_tool_calls: false` and `reasoning.summary: "auto"` reaches an OpenAI target
- **THEN** the OpenAI provider options SHALL set `store: false`, `parallelToolCalls: false` and `reasoningSummary: "auto"`
- **AND** an Anthropic target SHALL instead receive `disableParallelToolUse: true`

### Requirement: Responses stream event order
A streaming response SHALL be SSE whose events follow the OpenAI API order: `response.created`, then `response.in_progress`, then output items, then one terminal event. Each item SHALL start with `response.output_item.added` and end with `response.output_item.done` carrying the complete item, and no delta SHALL precede its item's added event. Events SHALL carry increasing `sequence_number` values.

#### Scenario: Text item
- **WHEN** a provider streams a text block
- **THEN** the codec SHALL emit `response.output_item.added` for a `message` item, `response.content_part.added`, one `response.output_text.delta` per non-empty delta, `response.output_text.done`, `response.content_part.done` and `response.output_item.done`

#### Scenario: Function call item
- **WHEN** a provider streams tool input and a final tool call
- **THEN** the codec SHALL emit `response.output_item.added` for a `function_call` item, `response.function_call_arguments.delta` events, `response.function_call_arguments.done` and `response.output_item.done`

#### Scenario: Reasoning item
- **WHEN** a provider streams consecutive reasoning blocks
- **THEN** the codec SHALL emit one `reasoning` item whose non-empty blocks are summary parts, each wrapped in `response.reasoning_summary_part.added` and `response.reasoning_summary_part.done` and ended by `response.reasoning_summary_text.done`
- **AND** the item SHALL carry no `encrypted_content`

### Requirement: Responses terminal events
A stream SHALL end in `response.completed` for a stop or tool-calls finish, `response.incomplete` with `incomplete_details.reason` of `max_output_tokens` for a length finish or `content_filter` for a content-filter finish, and `response.failed` for any failure while the request context is live. The terminal response SHALL carry the complete output and, when known, its `usage`.

#### Scenario: Length finish
- **WHEN** the provider finishes with a length reason
- **THEN** the last event SHALL be `response.incomplete` with `incomplete_details.reason: "max_output_tokens"`

#### Scenario: Failure after a finish
- **WHEN** the provider finishes and then fails before it closes the stream
- **THEN** the stream SHALL end in `response.failed` alone, with every added item done and no `incomplete_details`

#### Scenario: Request context ends
- **WHEN** the client cancels or the request deadline passes during a stream
- **THEN** the codec SHALL stop writing without a terminal event, as the Chat Completions adapter does

#### Scenario: Usage
- **WHEN** the provider reports input, cached, output and reasoning tokens
- **THEN** `response.completed` SHALL carry `input_tokens`, `input_tokens_details.cached_tokens`, `output_tokens`, `output_tokens_details.reasoning_tokens` and `total_tokens`, equal to input plus output tokens, in `usage`

### Requirement: Responses failure mapping
Failures SHALL use a fixed mapping from `execution.Category` to an HTTP status and an error code with a fixed message. A failure before the stream starts SHALL be an HTTP error document. A failure after `response.created` SHALL be a `response.failed` event whose `response.error` carries the code. Provider error text SHALL NOT appear in any body or event.

#### Scenario: Category mapping
- **WHEN** the runtime classifies a failure
- **THEN** the status and code SHALL be:
  - invalid request, BYOK credentials, BYOK selector and gateway control: `400` `invalid_request`
  - permission: `403` `permission_denied`
  - model not found: `404` `model_not_found`
  - failed dependency and catalog configuration: `424` `failed_dependency`
  - rate limit: `429` `rate_limit_exceeded`
  - cancellation: `499` `canceled`
  - upstream and provider stream: `502` `upstream_error`
  - overload and unavailable: `503` `unavailable`
  - timeout: `504` `timeout`
  - internal and anything else: `500` `internal_error`

#### Scenario: Provider error text is withheld
- **WHEN** a provider fails with a message that names a secret
- **THEN** no response body or event SHALL contain that message

#### Scenario: Invalid provider output
- **WHEN** a provider calls an undeclared tool, breaks a strict tool schema or ends without a finish
- **THEN** a stream SHALL end in `response.failed` with `upstream_error` and a unary call SHALL respond `502` `upstream_error`

#### Scenario: Runtime lifecycle violation
- **WHEN** the runtime refuses a stream part that breaks an output lifecycle rule, such as a finish while text is open
- **THEN** the stream SHALL end in `response.failed` with the code for the runtime's category, `internal_error`

### Requirement: Responses unary response
A request without `stream: true` SHALL return one `response` object with `status`, the complete `output` items in provider order, `incomplete_details` for a length or content-filter finish, and `usage` when known, using the same item shapes as the stream's `response.output_item.done` events.

#### Scenario: Unary text and function call
- **WHEN** a provider returns text and a declared tool call
- **THEN** the response SHALL hold a `message` item and a `function_call` item with the call's `call_id`, `name` and `arguments`

### Requirement: Responses codec handler
The package SHALL expose a handler for exactly `POST /v1/responses` with a JSON content type, built over the host execution runtime. No Gateway route SHALL mount it in this change.

#### Scenario: Handler route boundary
- **WHEN** the handler receives another path, a query string, another method or a non-JSON body
- **THEN** it SHALL respond with the fixed `404`, `405` or `415` document

### Requirement: Official client comparison over recordings
Each in-scope recorded OpenAI input SHALL be replayed to `openai-go` and to the `openai` Node SDK twice, once directly and once through the Go OpenAI provider and the codec. The two results SHALL match on output item types and order, message text, function call names and arguments, reasoning summaries and usage, excluding IDs and timestamps.

#### Scenario: Recorded inputs in scope
- **WHEN** the comparison runs
- **THEN** it SHALL cover `simple-text`, `tool-call`, `parallel-tool-calls`, `multi-tool-chain`, `reasoning-text`, `reasoning-tool-use` and the `structured-*` recordings
- **AND** it SHALL NOT add or change any recording
