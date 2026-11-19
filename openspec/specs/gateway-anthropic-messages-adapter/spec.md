# gateway-anthropic-messages-adapter Specification

## Purpose
Define the Anthropic Messages request subset, response and SSE mapping, error
documents and client evidence over the shared Gateway runtime and host contract.
The platform owns authentication, and the runtime owns selection, execution and
stream lifecycle. Fields that tune only the Anthropic wire are accepted and not
yet applied.

## Requirements

### Requirement: Messages route on the plugin route table

The Gateway SHALL serve the Anthropic Messages API as `POST v1/messages` under
`namespaces/{namespace}/`, registered as one route-table entry with the
`anthropic_messages` telemetry label and an Anthropic host-error writer. A
client SHALL address it by using `/apis/<group>/<version>/namespaces/<namespace>`
as the Anthropic base URL. Any query string, including `?beta=true`, and the
`anthropic-version` and `anthropic-beta` headers SHALL be accepted and SHALL NOT
change the result. Another method or path SHALL NOT reach the adapter.

#### Scenario: Official client addresses the namespaced route
- **WHEN** an official Anthropic client uses the namespaced API prefix as its base URL and creates a message
- **THEN** its request reaches the registered handler through the shared host access decision, runtime and telemetry

#### Scenario: Beta query and headers
- **WHEN** a request targets `v1/messages?beta=true` with `anthropic-beta` and `anthropic-version` headers
- **THEN** the adapter serves it exactly as it would without them

#### Scenario: Wrong method or media type
- **WHEN** a request uses a method other than `POST`, a non-JSON content type, or a content encoding
- **THEN** the adapter returns a fixed Anthropic error document with status 405, 415 or 400 respectively, and no model is selected or called

### Requirement: Platform authentication and credential separation

The Gateway SHALL NOT authenticate Messages callers itself. The Grafana platform
authenticates the caller before the plugin runs, and the Gateway SHALL apply the
host's access decision as for every other route. Clients SHALL send the Grafana
credential as an `Authorization: Bearer` token; the platform rejects `x-api-key`
before the plugin sees it. An incoming credential SHALL NOT be used as, or
forwarded as, a provider credential; provider credentials come from the
Gateway's configuration.

#### Scenario: Bearer token
- **WHEN** a client sends a valid Grafana service-account token as a bearer token
- **THEN** the request is served using the configured catalog models

#### Scenario: API-key header
- **WHEN** a client sends the Grafana token only in `x-api-key`
- **THEN** the platform refuses the request, the Gateway makes no provider call, and the client documentation tells Anthropic clients to use a bearer token instead

#### Scenario: Host refuses access
- **WHEN** the host's access decision refuses the caller
- **THEN** the adapter's host-error writer returns a fixed Anthropic permission error before the body is read

### Requirement: Messages request subset

The adapter SHALL decode the request with ordinary Go JSON behavior and SHALL
require `model`, a positive `max_tokens` and at least one message. It SHALL map
system text given as a string or text blocks, user and assistant turns given as
a string or content blocks, `text`, `tool_use` and `tool_result` blocks, client
function tools, tool choice, `stop_sequences`, `temperature`, `top_p`, `top_k`
and `stream`. The adapter SHALL apply `output_config.effort` as described below.
A `system`-role message after the first message SHALL be mapped to
a system message at that position. A tool declaration without an input schema
SHALL get an empty object schema. Unsupported requested semantics SHALL fail
with a fixed invalid-request error before model resolution.

#### Scenario: Parallel tool use disabled
- **WHEN** a request sets `disable_parallel_tool_use` in its tool choice
- **THEN** the adapter applies it through the call's option translator for each target, as the inverse of the OpenAI and OpenAI-compatible parallel-call options and as Anthropic's own option, and a request that does not set it adds no parallel-tool provider option

#### Scenario: Client tool continuation
- **WHEN** history contains an assistant `tool_use` block followed by a user `tool_result` block with the same ID
- **THEN** the adapter maps the result to that tool's name and ID, preserves message order, and leaves tool execution to the client

#### Scenario: Orphan or misplaced tool blocks
- **WHEN** a `tool_result` names no earlier `tool_use`, or a `tool_use` block appears outside an assistant turn
- **THEN** the request fails with a fixed invalid-request error and no provider is called

#### Scenario: System message inside the conversation
- **WHEN** a request carries a `role: "system"` message after a user message
- **THEN** the adapter maps it to a system message at that position instead of failing

#### Scenario: Unsupported content
- **WHEN** a request contains an image or document block, a server or built-in tool, or a tool choice type the adapter does not know
- **THEN** the request fails with a fixed invalid-request error and no provider is called

#### Scenario: Required fields
- **WHEN** `model`, `max_tokens` or `messages` is missing or invalid, or the body is not valid UTF-8 JSON
- **THEN** the request fails with a fixed invalid-request error

### Requirement: Output effort

The adapter SHALL accept `output_config.effort` values `low`, `medium`, `high`,
`xhigh` and `max` and translate them for each native candidate, including fallback.
Anthropic SHALL receive its `effort` provider option; OpenAI Responses and
OpenAI-compatible targets SHALL receive their `reasoningEffort` provider option.
The target provider retains responsibility for model-specific support.
#### Scenario: Effort on a mixed fallback chain
- **WHEN** a request sets `output_config.effort` to `high` and falls back from Anthropic to OpenAI
- **THEN** each candidate receives `high` under its own provider option key without inheriting the previous candidate's translated options

#### Scenario: Effort preserves other settings
- **WHEN** a request supplies effort alongside existing provider options and explicit parallel-tool settings
- **THEN** translation preserves those options without mutating shared candidate state, enabling Anthropic thinking, or changing the requested output-token limit

#### Scenario: Effort omitted or null
- **WHEN** effort is omitted or null
- **THEN** translation leaves any existing effort option unchanged

#### Scenario: Invalid effort
- **WHEN** a request supplies an empty, unknown, differently cased or non-string effort value
- **THEN** the adapter returns HTTP 400 with its fixed invalid-request error before model resolution or provider execution

### Requirement: Accepted fields that are not applied

The adapter SHALL accept `cache_control` on the request, system blocks, content
blocks and tools; `metadata`; `thinking`; `context_management`; `output_config`
fields other than `effort`; `service_tier`; and `thinking` and `redacted_thinking`
blocks in history. It SHALL NOT apply them to the provider call, and SHALL drop
thinking blocks from history. Documentation SHALL list each as a known limitation.
The Gateway SHALL NOT claim Claude Code compatibility, cache-control preservation
or thinking round trips on the strength of this requirement.

#### Scenario: Claude Code request shape
- **WHEN** a request carries cache-control markers, metadata, a thinking configuration, context management, a supported output effort and beta headers
- **THEN** the adapter serves it and translates effort for the target, while the other listed fields remain unapplied

#### Scenario: Limitations are documented
- **WHEN** a reader consults the client guide
- **THEN** each accepted-but-not-applied field and each refused content type is listed with its current behavior

### Requirement: Unary message responses

A non-streaming request SHALL return one Anthropic `message` with `assistant`
role, text and `tool_use` content blocks in provider order, the canonical
catalog model ID, a generated message ID when the provider supplies none,
a stop reason and provider-reported usage. `stop_sequence` SHALL be null. Reasoning content SHALL be omitted.
Stop reasons SHALL map: a tool-call finish to `tool_use`, a length finish to
`max_tokens`, a stop finish carrying the provider reason `pause_turn` or
`stop_sequence` to that reason, and any other stop to `end_turn`. An
unrepresented finish reason or content part SHALL fail with a fixed upstream
error. Usage SHALL report uncached input, output and, separately, cache read and
cache creation tokens, and SHALL NOT estimate counts the provider omits.

#### Scenario: Text response
- **WHEN** the provider returns text and a normal stop
- **THEN** the response has one `text` block, stop reason `end_turn`, and the provider's token counts

#### Scenario: Tool call response
- **WHEN** the provider returns a client function call
- **THEN** the response has a `tool_use` block with the call's ID, name and JSON input, and stop reason `tool_use`

#### Scenario: Cached input
- **WHEN** the provider reports total input tokens with cache read tokens
- **THEN** `input_tokens` excludes the cache read count and `cache_read_input_tokens` reports it

### Requirement: Incremental Messages streaming

A streaming request SHALL emit Anthropic SSE events as provider parts arrive:
`message_start`, content block start, delta and stop events, then
`message_delta` carrying the stop reason and cumulative usage, then
`message_stop`. Text SHALL stream as `text_delta` and client tool input as
`input_json_delta`, in provider order, one block at a time. A lifecycle the
adapter cannot represent, a provider error part, or a runtime failure after the
stream starts SHALL end the stream with an `error` event and no `message_stop`.
Cancellation SHALL cancel provider execution and SHALL NOT synthesize a
successful completion. Frames SHALL be flushed as they are written.

#### Scenario: Text then tool use
- **WHEN** the provider streams text followed by a tool call
- **THEN** the client receives a text block with its deltas, then a `tool_use` block with its partial JSON deltas, then `message_delta` with stop reason `tool_use` and `message_stop`

#### Scenario: Failure after the stream starts
- **WHEN** the provider reports an error after content has been sent
- **THEN** the stream ends with an `error` event, no further candidate is tried, and no `message_stop` is sent

#### Scenario: Caller cancels
- **WHEN** the client disconnects mid-stream
- **THEN** provider execution is canceled and the adapter writes no completion

#### Scenario: Setup failure
- **WHEN** the model cannot be selected or the stream cannot be established
- **THEN** the adapter returns a fixed Anthropic error with the mapped status instead of an event stream

### Requirement: Fixed Anthropic error documents

Every adapter-owned failure SHALL return a fixed document of the form
`{"type":"error","error":{"type":<type>,"message":<fixed text>}}` with no
provider message, response body or header. The runtime's closed failure
categories SHALL map to statuses 400, 403, 404, 424, 429, 499, 500, 502, 503 and
504 with the Anthropic error types `invalid_request_error`, `permission_error`,
`not_found_error`, `api_error`, `rate_limit_error`, `overloaded_error` and
`timeout_error`. Failures that the platform produces before the plugin runs need
not use this document.

#### Scenario: Provider failure carries a secret
- **WHEN** a provider error message or body contains a credential-like value
- **THEN** neither the unary response nor an `error` event contains it

#### Scenario: Unknown model
- **WHEN** the requested model is not in the caller's catalog
- **THEN** the response is a 404 `not_found_error` document that names no provider

### Requirement: Shared limits and no adapter budgets

The adapter SHALL obtain a call from the shared execution runtime and SHALL add
no request-size, output-size, part-count, selection or total-duration limit of
its own. Request size, deadlines and admission belong to the platform; attempts,
non-stream response bytes and stream idle and drain time belong to the shared
runtime.

#### Scenario: Large body without an adapter limit
- **WHEN** a request body is multiple megabytes but within the platform's limit
- **THEN** the adapter reads and maps the whole body and passes it to the shared runtime

### Requirement: Official-client and Claude Code evidence

The adapter SHALL have deterministic tests using the pinned official Anthropic
Go client for unary, streaming, tool-result continuation and error behavior.
Evidence for Claude Code SHALL come from a pinned client version and SHALL name
that version. Tests and documentation SHALL distinguish synthetic evidence,
live-provider evidence and actual Claude Code evidence, and SHALL NOT present one
as another.

#### Scenario: Go client consumption
- **WHEN** the official Go client creates unary and streaming messages through the handler
- **THEN** it assembles text, tool-use blocks, stop reasons and usage as the provider produced them

#### Scenario: Claude Code smoke evidence
- **WHEN** Claude Code runs a multi-turn task with tools against a local Gateway
- **THEN** every `/v1/messages` call succeeds and the recorded evidence names the client version, the backend and whether the model was a stand-in or a live provider
