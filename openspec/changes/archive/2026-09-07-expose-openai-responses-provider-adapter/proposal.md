## Why

The pinned Vercel AI SDK baseline lets provider integrations reuse the OpenAI Responses implementation through `@ai-sdk/openai/internal`; the Go OpenAI module has no equivalent composition seam. This blocks an aligned port of `@ai-sdk/amazon-bedrock/mantle` without duplicating Responses behavior or moving Bedrock concerns into the public OpenAI provider.

## What Changes

- Add an additive constructor for provider integrations to supply a preconfigured official OpenAI Go client.
- Add an option for integrations to set the provider identity used by `Provider()` and provider metadata.
- Preserve the existing API-key constructor and OpenAI-specific call-option namespace.
- Record the dedicated Bedrock Mantle provider as an explicit, still-open parity gap.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Allow provider integrations to reuse the Responses implementation with provider-owned clients and identities.

## Impact

The public API of `providers/openai` gains provider-integration plumbing. Existing callers, dependencies, request conversion, response mapping, and wire behavior remain unchanged. This change does not itself expose a Bedrock Mantle provider; that remains a follow-up under `providers/bedrock`.
