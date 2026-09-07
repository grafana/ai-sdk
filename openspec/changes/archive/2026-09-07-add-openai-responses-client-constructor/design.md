## Context

The provider model stores an `openai-go` `responses.ResponseService`, but its only public constructor creates a standard API-key client internally. The official `openai-go/v3/bedrock` package returns a normal `openai.Client` whose Responses service carries endpoint resolution and SigV4 middleware.

The registered upstream `@ai-sdk/openai@4.0.41` baseline supports provider-level transport configuration but has no Go client-injection equivalent. This change is an additive, parity-preserving Go adaptation and does not alter request conversion or wire output.

## Goals / Non-Goals

**Goals:**

- Allow callers to supply an official preconfigured OpenAI client.
- Preserve all existing model options and API-key constructor behavior.
- Verify that client-level Bedrock Mantle SigV4 configuration reaches Responses requests unchanged.

**Non-Goals:**

- Implement SigV4 signing in this provider.
- Add Bedrock-specific configuration to the provider API.
- Change request conversion, response mapping, or streaming behavior.

## Decisions

### Accept the official `openai.Client` value

Expose `NewResponsesWithClient(client openai.Client, modelID string, opts ...Option)`. This matches the value returned by both `openai.NewClient` and `bedrock.NewClient`, keeps authentication ownership in the official SDK, and avoids duplicating security-sensitive signing middleware.

Accepting only `responses.ResponseService` was considered but rejected because callers naturally construct the public top-level client, and the service value is primarily an implementation detail of that client. Adding provider-specific SigV4 options was rejected because this adapter should remain authentication-agnostic.

### Share model initialization

Both constructors use a private helper for model identity, ID generation, and functional options. The existing constructor continues to build its client exactly as before; the new constructor assigns the supplied client's Responses service.

### Use focused transport coverage

A unit test supplies deterministic fake AWS credentials and an in-memory HTTP transport to the official Bedrock client. It verifies endpoint selection and SigV4 credential scope without network access or real credentials. No conformance fixture changes are needed because request bodies and provider output remain unchanged.

## Risks / Trade-offs

- **Public API depends on the concrete `openai-go` client type** → This module already exposes and depends on that SDK for raw request options, and Go module version selection keeps consumers on one compatible version.
- **Only generation is exercised by the new signing test** → Generation and streaming share the same stored Responses service and request-option path; existing tests cover both paths.
