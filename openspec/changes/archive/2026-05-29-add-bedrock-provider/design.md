## Context

The Grafana ai-sdk Go port ships two provider modules today: `providers/anthropic` (Anthropic direct + Vertex) and `providers/grafana` (a transparent hosted transport). The assistant team's deployed setup also uses AWS Bedrock as a failover target. Bedrock is intentionally *not* a thin re-skin of the Anthropic provider:

- Different wire API: AWS Bedrock Converse (`/model/{id}/converse`, `/model/{id}/converse-stream`) rather than Anthropic Messages.
- Different message shape: `system` array of system blocks + `messages` with content blocks (`text`, `toolUse`, `toolResult`, `image`, `document`, `reasoningContent`, `cachePoint`).
- Different streaming protocol: AWS Smithy event-stream binary framing, not SSE. Each frame carries `:event-type` (e.g. `messageStart`, `contentBlockDelta`, `messageStop`, `metadata`) and a JSON payload.
- Different auth: SigV4 (canonical AWS) or `AWS_BEARER_TOKEN_BEDROCK` bearer token. No API key model.
- Different model namespace: Bedrock model IDs like `anthropic.claude-sonnet-4-5-20250929-v1:0`, `mistral.mistral-large-2407-v1:0`, `amazon.nova-lite-v1:0`. The provider must work for any of them, with Anthropic-specific knobs gated behind an "is Anthropic model" check.

Upstream `@ai-sdk/amazon-bedrock` reflects this -- it has zero imports from `@ai-sdk/anthropic`. We mirror that boundary.

The conformance harness (`test/conformance/`) already supports per-provider fixture directories, multi-step replay, deterministic IDs, tool mocking, and approval seeding. It currently assumes SSE on the wire. Adding Bedrock requires teaching the replay server how to serve AWS event-stream binary frames so the Go provider exercises its real decoder against fixture data.

## Goals / Non-Goals

**Goals:**

- Independent Go module `providers/bedrock` implementing `provider.LanguageModel` against AWS Bedrock Converse.
- Functional behavior parity with upstream `@ai-sdk/amazon-bedrock` for text, tool calls, reasoning/thinking, streaming, and error handling -- proven by byte-identical conformance output.
- Pluggable auth: AWS SDK v2 default credential chain, explicit static credentials, bearer token, custom credential provider injection.
- Anthropic-on-Bedrock features (thinking, effort, betas, native structured output) routed through `additionalModelRequestFields`. Non-Anthropic models produce warnings instead of silently dropping.
- Conformance: Bedrock entries under `test/conformance/bedrock/{upstream,recorded}/` with `expected.jsonl` byte-identical to upstream TypeScript output, runnable via `make test-conformance`.
- Recording/generation tools (`record.mts`, `generate.mts`) understand Bedrock provider directories.

**Non-Goals:**

- Embeddings, image generation, reranking. The upstream package supports them; the assistant doesn't use them. Out of scope to keep the module small; revisit if needed.
- Bedrock Guardrails configuration plumbing. Not required by the assistant today.
- Custom token caching. The AWS SDK v2 credential provider chain handles caching/refresh.
- Non-Anthropic provider tools (Mistral function calling has identical shape via Converse; Nova/OpenAI on Bedrock have model-specific quirks documented but not implemented exhaustively).
- A `provider.LanguageModel` middleware just for Bedrock. Standard middleware works at the wrapping layer.

## Decisions

### 1. Separate Go module at `providers/bedrock/`

- **Decision**: New module with its own `go.mod`, module path `github.com/grafana/ai-sdk/providers/bedrock`. Mirror the `providers/anthropic` layout (model.go, options.go, convert_request.go, convert_response.go, convert_stream.go, plus event-stream decoder and SigV4 fetch round-tripper).
- **Why**: AWS SDK v2 brings a large dependency surface (credentials, signer, http) that root consumers must opt into. Mirroring the Anthropic boundary keeps `go get github.com/grafana/ai-sdk` lean.
- **Alternative considered**: Add to `providers/anthropic` as an "auth mode". Rejected: API surface differs (different model ID format, different message shape, supports non-Anthropic models). Would force every Anthropic user to pull in AWS SDK.
- **Alternative considered**: Use AWS SDK v2's `bedrockruntime` typed client. Rejected: the typed client adds heavy proto/Smithy machinery and would couple us to its breaking changes; we already need to translate to/from `provider.CallOptions`. Hand-rolling a thin HTTP layer plus reusing the SDK's credentials chain + signer keeps things explicit and matches upstream's approach (which builds its own fetch wrapper).

### 2. Construction surface

```go
bedrock.New(modelID string, opts ...Option) provider.LanguageModel

// Options
bedrock.WithRegion(string)
bedrock.WithBaseURL(string)               // override endpoint (e.g. VPCE)
bedrock.WithCredentials(aws.CredentialsProvider) // AWS SDK v2 typed provider
bedrock.WithBearerToken(string)           // takes precedence over SigV4
bedrock.WithHTTPClient(*http.Client)
bedrock.WithHeaders(map[string]string)    // extra static request headers
bedrock.WithGenerateID(func() string)     // for tests
```

- **Why one constructor**: Bedrock has a single API. Auth differences are configured via options, not separate constructors. This matches Vercel's `createAmazonBedrock(settings)`. Differs from our Anthropic module where `New` vs `NewVertex` reflect actually-different APIs (api-key vs Vertex AI auth + endpoint), but here a single constructor is the right shape.
- **Why explicit credentials provider type**: AWS SDK v2's `aws.CredentialsProvider` is the standard. Callers can use `config.LoadDefaultConfig(ctx)` to get one and pass it in. Reduces our surface to learn vs. re-inventing knobs for access key / secret / session token.
- **Bearer token**: optional, mirrors upstream. When set, SigV4 is skipped and `Authorization: Bearer <token>` is sent. Env var fallback `AWS_BEARER_TOKEN_BEDROCK` is read once at construction; we do not poll.

### 3. Request transport: hand-rolled HTTP + AWS SDK v2 signer

- **Decision**: Use `net/http` plus AWS SDK v2's `aws-sdk-go-v2/aws/signer/v4` for SigV4. Build a `RoundTripper` that signs `POST` requests with non-empty bodies before delegating to a wrapped `http.RoundTripper`.
- **Why**: Mirrors upstream's `createSigV4FetchFunction` shape. Lets us keep request/response handling in plain Go and decouples us from `bedrockruntime`'s typed surface.
- **Endpoint resolution**: default `https://bedrock-runtime.<region>.amazonaws.com`. `WithBaseURL` overrides for VPC endpoints, test servers, or alternate partitions.

### 4. Request conversion (`convert_request.go`)

Mapping table (provider type -> Bedrock Converse JSON):

| Provider input | Bedrock Converse |
|---|---|
| `provider.CallOptions.Prompt` system messages | `system: [{text}, {cachePoint?}]` (cachePoint only when `providerOptions.amazonBedrock.cachePoint` is set) |
| User text | `{role: "user", content: [{text}]}` |
| User file (image) | `{image: {format, source: {bytes}}}` (base64 of `data:` URLs; `url` -> warning unsupported) |
| User file (document) | `{document: {format, name, source: {bytes}}}` |
| Assistant text | `{role: "assistant", content: [{text}]}` |
| Tool call | `{toolUse: {toolUseId, name, input}}` -- `toolUseId` normalized for Mistral models |
| Tool result | `{role: "user", content: [{toolResult: {toolUseId, content}}]}` -- tool messages collapse into user role per Converse contract |
| Reasoning content (assistant) | `{reasoningContent: {reasoningText: {text, signature?}}}` -- or `{redactedReasoning: {data}}` |
| Cache control on parts | `{cachePoint: {type: "default", ttl?}}` injected after the part |
| `Tools` | `toolConfig.tools: [{toolSpec: {name, description?, inputSchema: {json}}}]` |
| `ToolChoice` | `toolConfig.toolChoice: {auto: {}} | {any: {}} | {tool: {name}}` |
| Anthropic provider tools | empty tools entry + `additionalModelRequestFields.tool_choice` (Anthropic models on Bedrock only) |
| `MaxOutputTokens`, `Temperature`, `TopP`, `TopK`, `StopSequences` | `inferenceConfig.{maxTokens,temperature,topP,topK,stopSequences}` |
| `ResponseFormat: json` with schema, Anthropic model with structured-output support | `additionalModelRequestFields.output_config.format = {type:"json_schema", schema}` (native) |
| `ResponseFormat: json` with schema, other models | synthetic `json` tool injection + `toolChoice: required` (json-response tool pattern, upstream parity) |
| `Reasoning` config | `additionalModelRequestFields.thinking` (Anthropic), `output_config.effort` (Anthropic), `reasoning_effort` (OpenAI), `reasoningConfig` (Nova) -- routed by model-ID prefix |
| Unsupported (`FrequencyPenalty`, `PresencePenalty`, `Seed`) | append `provider.Warning{Type: "unsupported", Feature: ...}` |
| `temperature` out of `[0, 1]` | clamp + warning (upstream behavior) |

- **Model-family routing**: helper `isAnthropicModel(modelID)` matches `contains("anthropic")`, `isOpenAIModel` matches prefix `openai.`, `isMistralModel` matches prefix `mistral.`. Centralized in one file; keeps the language model body uncluttered.
- **Why this shape**: byte-equivalent to upstream's `convertToAmazonBedrockChatMessages` and `getArgs` output, which is the ground truth our conformance fixtures compare against.

### 5. Streaming: AWS event-stream binary decoder (`convert_stream.go` + `eventstream.go`)

- **Decision**: Implement a small Smithy event-stream frame decoder inline. Each frame has a 4-byte total length, 4-byte headers length, prelude CRC, headers, payload, message CRC. We parse `:event-type` and the JSON payload from each frame.
- **Why inline**: AWS SDK v2's `eventstream` package is internal-ish (under `aws/protocol/eventstream`) and ties events to typed structs. We only need to extract `(eventType, json)` pairs. An ~150-line decoder is simpler than wrestling with the typed pipeline.
- **Mapping events to `provider.StreamPart`**: pattern-match on event type:
  - `messageStart` -> `PartResponseMetadata` (id from `x-amzn-requestid` header)
  - `contentBlockStart` with `toolUse` -> `PartToolInputStart`
  - `contentBlockDelta` with `text` -> `PartTextDelta`
  - `contentBlockDelta` with `toolUse.input` -> `PartToolInputDelta` (accumulate JSON fragment per index)
  - `contentBlockDelta` with `reasoningContent` -> `PartReasoningDelta`
  - `contentBlockStop` -> `PartTextEnd` / `PartToolInputEnd` / `PartReasoningEnd` (based on block kind tracked per index)
  - `messageStop` with `stopReason` -> finish-reason set
  - `metadata` with `usage`, `metrics` -> usage + provider metadata
  - `internalServerException`, `throttlingException`, `validationException`, `modelStreamErrorException` -> `PartError` with `*provider.APICallError` (`IsRetryable` true for throttling and 5xx)
- **Channel buffer**: 64 events to match Anthropic provider behavior.
- **Cancellation**: respect `ctx.Done()`; cancel the in-flight HTTP request, drain on close.

### 6. Response conversion (`convert_response.go`)

- Non-streaming response is plain JSON; decoded into a shape mirroring the upstream `AmazonBedrockResponseSchema`. Walk `output.message.content` for `text`, `reasoningContent` (with redacted variant), `toolUse`. Map `stopReason` via `mapFinishReason`. Build usage from `usage.{inputTokens, outputTokens, cacheReadInputTokens, cacheWriteInputTokens}` mapping to `provider.Usage` with cache token detail.
- JSON-response-tool collapse: when the synthetic `json` tool fires, the tool call becomes the final text (matching upstream).

### 7. Errors

- Non-2xx generate -> decode `{message, type}` JSON, wrap in `*provider.APICallError`. `IsRetryable` true for 429, 503; false otherwise (best-effort; specific Bedrock error names like `ThrottlingException` set retryable=true regardless of status).
- Streaming errors arrive as event-stream frames with `:message-type=exception` and `:exception-type=<name>`. Translate to `provider.PartError` with the same retryable rules. Final channel send before close.
- Transport errors mid-stream (network drop after 2xx) emit a synthesized retryable `*provider.APICallError`.

### 8. Conformance harness updates

- **Replay server gains a Bedrock mode**: detect at construction time (passed by the test factory) whether to wrap each fixture JSON line as an AWS Smithy event-stream binary frame instead of SSE. Implementation: encode each line with `:event-type=<outerKey>` (e.g. `contentBlockDelta`, `messageStop`) and a JSON payload that mirrors the inner object. Response content type: `application/vnd.amazon.eventstream`.
- **Why this approach**: fixtures stay human-readable JSON-per-line, same as Anthropic. Only the wire framing differs. This keeps the recording tools and review experience consistent.
- **`runner.go`**: extract the framing strategy behind a `Framing` interface or per-provider `ReplayServerOption`. Anthropic keeps the existing SSE path; Bedrock gets the new binary path.
- **Provider factory wiring**: `test/conformance/bedrock/conformance_test.go` constructs a `bedrock.New(...)` pointing at the replay server's `BaseURL`. Uses a stub credentials provider since SigV4 is not validated by the replay server (it doesn't check signatures).
- **Fixture import**:
  - `test/conformance/bedrock/upstream/` -- import from upstream `__fixtures__` (text, reasoning, tool-call, json-tool, json-with-tools, tool-no-args, …). Each gets a `config.yaml` with `model:` plus `tools/responseFormat` where needed and an `INDEX.yaml` mapping name -> imported directory.
  - `test/conformance/bedrock/recorded/` -- a handful of fixtures recorded via `tools/record.mts` against real Bedrock endpoints in our sandbox account (cost-minimised: small models, short prompts). At minimum: simple-text, tool-call, parallel-tool-calls.
- **`generate.mts` / `record.mts`**: add `bedrock` provider switch that wires up `createAmazonBedrock` from `@ai-sdk/amazon-bedrock`. The TypeScript replay path for `generate.mts` already runs against a local test server; we serve the same Bedrock binary framing from a TS-side helper so upstream's decoder happily consumes it. (Alternative: ship JSON-passthrough mode in upstream tests via `__fixtures__` style direct pipe; investigate which is less work.)

### 9. Provider name and registry integration

- `Provider()` returns `"amazon-bedrock"` (matches upstream string used by `providerOptions.amazonBedrock` and `providerMetadata.amazonBedrock`).
- Registry: the module exports a `Provider` constructor returning a `registry.Provider` so consumers can register it as `"bedrock"` or any custom id and resolve `<id>:<modelID>` model ids. Same pattern as Grafana provider.

## Risks / Trade-offs

- **Risk**: AWS event-stream binary format is finicky (CRC, header types, payload boundaries). → Mitigation: write a focused decoder with table-driven tests covering every header type we care about and every event we handle. Validate end-to-end against `__fixtures__` re-encoded into binary frames in the replay server.
- **Risk**: SigV4 signing differs from request to request and is order-sensitive; a wrong canonical request silently 403s. → Mitigation: rely on AWS SDK v2's `signer/v4` (battle-tested) rather than hand-rolling. Round-tripper signs immediately before sending; integration tests cover header order and body hashing.
- **Risk**: Upstream evolves Bedrock features (new content block types, new reasoning shapes). → Mitigation: keep the convert/decode files small and focused; flag anything we don't recognize as a `Warning` rather than dropping silently. Pin a Vercel version in `AGENTS.md`'s upstream reference and bump deliberately.
- **Risk**: `additionalModelRequestFields` is a free-form map; getting the Anthropic-on-Bedrock thinking/effort/beta shape wrong is undetectable until a real call. → Mitigation: conformance fixtures for `thinking-text`, `thinking-tool-use`, and `combined-context-editing` exercise the request shape. The `request.json` snapshot in `__snapshots__` in upstream is the ground truth; we add a request-body assertion mode to the replay server for `recorded/` Bedrock fixtures.
- **Risk**: Mistral tool call IDs deviate from the rest (no underscores; numeric). → Mitigation: port upstream's `normalizeToolCallId` verbatim with the `isMistralModel` gate.
- **Risk**: Conformance binary framing diverges between Go and the TS replay server. → Mitigation: a single shared encoder spec described in this design; Go test asserts the same bytes the TS tool produces by round-tripping one fixture through both.
- **Trade-off**: Hand-rolling the event-stream decoder vs. depending on `aws-sdk-go-v2`'s internals. Chose hand-rolled for surface control. Cost: ~150 LoC + tests we own.
- **Trade-off**: Not implementing embeddings/image/reranking up front. Cost: harder to land them later as drive-bys; benefit: smaller diff, faster review.

## Migration Plan

This is a new module. No migration required. Existing consumers unaffected. Issue #30 closes when:
1. `providers/bedrock` builds, `make test` passes for it.
2. `make test-conformance` passes for `bedrock/` `upstream/` fixtures.
3. At least 3 `recorded/` fixtures cover representative paths.
4. README points to the new provider.

Rollout in the assistant: consumers add `providers/bedrock` to their failover chain. No SDK-side flag.

## Open Questions

- **Q**: For `recorded/` Bedrock fixtures, do we capture request bodies (`request.json`) as well as response chunks for stricter assertion? Upstream's snapshot tests do. → Default: yes for `recorded/`, no for `upstream/` (which doesn't ship request snapshots). Confirm during implementation when scaffolding the runner option.
- **Q**: Should `bedrock.New` accept `context.Context` (like `anthropic.NewVertex`) for credential loading, or defer credentials to the first call? → Lean: defer; AWS SDK v2 credentials providers are lazy by design. Easier ergonomics. Confirm by ensuring the round-tripper can take a `context.Context` from the per-request call.
- **Q**: Do we surface `additionalModelResponseFieldPaths` for non-Anthropic models? Upstream sets it only for Anthropic. → Match upstream behavior; revisit if a non-Anthropic feature lands that needs it.
