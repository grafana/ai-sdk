# Chat client schemas

## Why
Official structured-output helpers produce unions and local references. Chat rejected these valid schemas and functions that omit parameters before invoking a provider.

## What Changes
Compile self-contained schemas with the installed JSON Schema validator, with external resource loading disabled. Retain the original schema on provider requests. Supply an empty object schema for omitted function parameters.

## Capabilities
Modified: gateway-openai-chat-completions-adapter.

## Impact
Only Chat request mapping and its official-client tests change. No provider recordings or shared runtime behavior change. The registered Vercel reference is ai 7.0.159 / @ai-sdk/openai 4.0.106; Chat ingress is a Gateway extension. OpenAI's documented omitted-parameters semantics and the pinned npm openai 6.27.0 zodResponseFormat helper supply the client contract.
