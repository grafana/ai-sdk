## Why

Issue #871: a Chat Completions request whose only message has empty content, such as `{"role":"user","content":""}`, passes validation and reaches the provider. A provider that rejects empty text returns a 4xx, which the Gateway reports as `424 failed_dependency`. The caller made the mistake, but the response blames a dependency. The route already returns `400 invalid_request_error` for `messages: []`.

## What Changes

Refuse a request before model resolution when no message carries content: no non-empty text and no function call. The rule extends the existing empty `messages` refusal to a prompt that is empty in substance. A request that has some content, such as a system instruction beside an empty user message, is still forwarded, because OpenAI accepts empty user content and the pinned `ai` 7.0.173 `standardizePrompt` only refuses an empty message list.

## Capabilities

### Modified Capabilities
- `gateway-openai-chat-completions-adapter`: the explicit request subset refuses a prompt without content.

## Impact

Chat request mapping only (`ai-gateway/openai/chatcompletions/request.go`), for unary and streaming requests. The native ProviderWire route is a transport for an already built LanguageModelV4 prompt and is unchanged. No fixture, provider or wire format change. The shared error message for every validation failure (#872) is out of scope.
