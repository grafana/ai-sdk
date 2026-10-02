## Why

OpenAI SDK users need a Chat Completions adapter without depending on ProviderWire headers or payloads. The adapter must preserve authentication, catalog identity, middleware ownership, and bounded execution while explicitly rejecting features the underlying provider routes cannot preserve.

## What Changes

- Add independent AGPL Chat DTOs, strict request mapping, safe OpenAI errors, unary serialization and SSE lifecycle.
- Add POST `/v1/chat/completions`, adapter Bearer authentication and OpenAI 404/405 responses.
- Freeze a conservative finite backend capability matrix at catalog composition; preserve the text-only fallback guard.
- Force Responses `store:false` and translate Chat's omitted/null function strict to false.
- Add deterministic official SDK consumption tests and documented limits/non-goals.

## Capabilities

### New Capabilities

- `gateway-openai-chat-completions-adapter`: bounded OpenAI Chat adapter subset with shared Gateway service foundations.

### Modified Capabilities

None. Existing ProviderWire behavior remains unchanged.

## Impact

AGPL Gateway module only. No Apache dependency on Gateway, root workspace membership, protocol codec reuse, provider credentials, server tool execution, persistence, automatic retry, or new public model discovery endpoint.
