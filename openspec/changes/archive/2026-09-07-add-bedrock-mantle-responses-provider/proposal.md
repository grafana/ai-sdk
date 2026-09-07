## Why

The registered upstream Bedrock package exposes a dedicated Mantle provider that owns authentication, routing, and provider identity while reusing OpenAI protocol implementations. The Go Bedrock module can now add the Responses half of that surface without duplicating either SigV4 security code or Responses conversion.

## What Changes

- Add a `providers/bedrock/mantle` package for Bedrock Mantle's OpenAI-compatible Responses API.
- Delegate bearer and AWS SigV4 authentication to the official `openai-go/v3/bedrock` client.
- Delegate request, response, streaming, and continuation behavior to `providers/openai` while reporting `bedrock-mantle.responses` for model and response attribution.
- Route native OpenAI model IDs through AWS-documented Mantle paths: `/v1/responses` by default and the model-specific `/openai/v1/responses` path required by `openai.gpt-5.6-luna`.
- Preserve bearer authentication as an explicit and environment-driven rollback mode while defaulting to the AWS credential chain.
- Narrow the recorded Mantle parity gap to the still-unimplemented Chat/default provider surface.

## Capabilities

### New Capabilities

- `bedrock-mantle-responses-provider`: Construct and authenticate Bedrock Mantle Responses models while preserving OpenAI Responses semantics.

### Modified Capabilities

None.

## Impact

The Bedrock module gains direct dependencies on the stacked `providers/openai` prerequisite and `openai-go/v3`. The new package is additive and does not change the existing Bedrock Converse provider. This change intentionally implements Responses only; Mantle Chat Completions and the upstream default Chat factory remain follow-up parity gaps.
