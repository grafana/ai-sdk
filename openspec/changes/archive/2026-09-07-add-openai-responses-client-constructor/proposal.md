## Why

The OpenAI Responses provider always constructs its own API-key client, which prevents callers from using provider-specific OpenAI clients that configure authentication at construction time. In particular, Amazon Bedrock Mantle requires the OpenAI Go SDK's Bedrock client to apply SigV4 signing to Responses requests.

## What Changes

- Add an additive constructor that accepts a preconfigured `openai-go` client.
- Preserve the existing API-key constructor and all model behavior.
- Document and test Amazon Bedrock Mantle SigV4 as the motivating client configuration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: Extend provider construction to support preconfigured OpenAI clients while retaining the existing API-key constructor.

## Impact

The public API of `providers/openai` gains one constructor. Existing callers and wire behavior are unchanged. Consumers can configure alternate authentication, endpoint, middleware, retry, and transport behavior through `github.com/openai/openai-go/v3` before creating the language model.
