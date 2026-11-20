# Proposal

## Why

Codex reaches a custom provider only through the OpenAI Responses API, and the Gateway has no Responses adapter (#554). This change adds the codec that maps a Responses request to a provider call and provider stream parts back to Responses JSON and SSE, the first slice of #554 (#858). The shared runtime and host contract from #481, #482 and #483 exist, so the codec follows `ai-gateway/openai/chatcompletions`.

## What Changes

- A new package, `ai-gateway/openai/responses`, holds the codec and an HTTP handler for `POST /v1/responses`. Nothing mounts the handler yet. Mounting it on a Gateway route is the next slice.
- The codec accepts an explicit subset of request fields, input items and tools, and refuses everything else with a fixed error document. It refuses `previous_response_id`, `store: true`, `custom` tools, hosted tools such as `web_search`, and custom tool call input items.
- Streaming output follows the OpenAI API event order, so the official `openai` Node SDK accumulator accepts it. A stream ends in `response.completed`, `response.incomplete` or `response.failed`.
- Failures use a fixed mapping from `execution.Category` to an HTTP status and error code. Provider error text never reaches a response body or event.
- Output reasoning items carry summaries and no `encrypted_content`, and an input reasoning item's `encrypted_content` is not applied, until the Codex slice settles reasoning replay. `prompt_cache_key` and `client_metadata` are accepted and not applied.

## Capabilities

### New Capabilities

- `gateway-openai-responses-codec`: the Responses request subset, refusals, stream event order, terminal events, failure mapping and unary response.

### Modified Capabilities

None.

## Impact

- **Code:** `ai-gateway/openai/responses`.
- **Tests:** table tests in the package. `ai-gateway/test/openai-responses-codec` replays the in-scope recorded OpenAI inputs to `openai-go` and the `openai` Node SDK, once directly and once through the Go OpenAI provider and the codec, and compares the results. A `mise` task runs it.
- **Fixtures:** none added or changed. `web-search`, `code-interpreter` and `computer-call` are left out because they use hosted tools.
- **Gaps for #554:** there is no runtime category for context length, so the codec never emits `context_length_exceeded`.
