## Why

The Grafana assistant team's multi-backend failover setup currently spans Anthropic direct, Vertex AI, AWS Bedrock, and Azure. The Go ai-sdk only covers the first two through the `providers/anthropic` module, so Bedrock cannot participate in failover today. Upstream `@ai-sdk/amazon-bedrock` is a separate package because Bedrock uses AWS Converse -- a different API, auth scheme, message shape, and streaming protocol from the Anthropic Messages API. We need the same separation in Go, plus conformance coverage that proves byte-identical UI output to upstream.

## What Changes

- Add a new Go module `providers/bedrock/` at `github.com/grafana/ai-sdk/providers/bedrock` with its own `go.mod` (AWS SDK v2 dependency), independent from `providers/anthropic`.
- Constructor `bedrock.New(modelID string, opts ...Option) provider.LanguageModel` plus functional options for region, credentials provider, base URL, custom HTTP client, and request headers.
- Implement `provider.LanguageModel` (`SpecificationVersion`, `Provider`, `ModelID`, `SupportedURLs`, `DoStream`, `DoGenerate`) against the AWS Bedrock Converse API (`/model/{id}/converse` and `/model/{id}/converse-stream`).
- Request conversion: map `provider.CallOptions` (Prompt, Tools, ToolChoice, InferenceConfig params) to Converse `system`, `messages`, `toolConfig`, `inferenceConfig`.
- Anthropic-specific pass-through via `additionalModelRequestFields` (thinking, effort, betas, native structured output) when the model ID identifies an Anthropic model on Bedrock.
- Response and stream conversion: Bedrock event-stream binary format -> `provider.StreamPart` (text, reasoning, tool calls, errors, finish reason, usage). Decode binary event-stream frames (Smithy event-stream encoding) for `converse-stream`.
- AWS authentication: SigV4 signing via AWS SDK v2 credentials chain by default, with optional Bearer-token (`AWS_BEARER_TOKEN_BEDROCK`) and custom credential provider injection. No proprietary token cache -- delegate to the AWS SDK.
- Conformance coverage: add `test/conformance/bedrock/` with `upstream/` fixtures imported from `@ai-sdk/amazon-bedrock` (`__fixtures__`) and at least one `recorded/` fixture per major code path; share `runner.go` infrastructure with Anthropic, including AWS event-stream-aware replay where needed.
- Provider conformance test wired into `make test-conformance`, asserting byte-identical `UIMessageChunk` output against upstream `expected.jsonl`.

## Capabilities

### New Capabilities
- `bedrock-provider`: a separate Go module that implements `provider.LanguageModel` against the AWS Bedrock Converse API, including SigV4 auth, request/response conversion, event-stream decoding, and Anthropic-feature pass-through via `additionalModelRequestFields`.

### Modified Capabilities
- `conformance-testing`: extend the conformance harness to support a Bedrock provider directory, the AWS event-stream replay shape, and Bedrock-specific fixture import from upstream `__fixtures__`. The fixture format and `expected.jsonl` contract stay the same; the replay server gains a Bedrock mode that serves AWS event-stream binary frames instead of (or in addition to) SSE.

## Impact

- **New module**: `providers/bedrock/` with its own `go.mod`. Adds AWS SDK v2 (`github.com/aws/aws-sdk-go-v2`, `aws-sdk-go-v2/config`, `aws-sdk-go-v2/credentials`, `aws-sdk-go-v2/aws/signer/v4`) and a Smithy event-stream codec dependency (either via AWS SDK v2 or a small in-tree decoder).
- **No changes to root `aisdk` module dependencies**: Bedrock stays isolated like Anthropic does today.
- **Conformance harness**: `test/conformance/runner.go` grows a Bedrock-aware replay server mode. A new per-provider Go test file `test/conformance/bedrock/conformance_test.go`. `Makefile` `test-conformance` target picks it up via build tag.
- **Tooling**: `test/conformance/tools/record.mts` and `generate.mts` learn to operate on `bedrock/` directories using the upstream `@ai-sdk/amazon-bedrock` package.
- **Docs**: provider listed in root `README.md` and a new `providers/bedrock/README.md`. Issue #30 closed when this lands.
- **Out of scope**: embeddings, image generation, reranking (covered by upstream but not used by the assistant today); Bedrock guardrails; deferred -- can be added later without redesigning the module.
