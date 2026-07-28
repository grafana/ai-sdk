## 1. Module scaffold

- [x] 1.1 Create `providers/bedrock/` directory with `go.mod` declaring module `github.com/grafana/ai-sdk/providers/bedrock`, Go 1.26, `replace github.com/grafana/ai-sdk => ../../` (mirroring `providers/anthropic/go.mod`)
- [x] 1.2 Add AWS SDK v2 dependencies: `github.com/aws/aws-sdk-go-v2`, `aws-sdk-go-v2/aws`, `aws-sdk-go-v2/aws/signer/v4`, `aws-sdk-go-v2/config`, `aws-sdk-go-v2/credentials`; pin compatible versions and run `go mod tidy`
- [x] 1.3 Add `doc.go` describing the module's purpose and provider name `amazon-bedrock`
- [x] 1.4 Add the module to the root `Makefile` `test` and `build` targets (and any `lint`/`vet` aggregations) so `make test` runs Bedrock tests too
- [x] 1.5 Wire `providers/bedrock` into `AGENTS.md` project structure section

## 2. Provider construction and options

- [x] 2.1 Add `options.go` with `Option func(*model)` and option helpers: `WithRegion`, `WithBaseURL`, `WithCredentials(aws.CredentialsProvider)`, `WithBearerToken`, `WithHTTPClient`, `WithHeaders`, `WithGenerateID`
- [x] 2.2 Add typed provider option struct `BedrockOptions` carrying `ReasoningConfig`, `MaxReasoningEffort`, `AnthropicBeta`, `CachePoint`, `ServiceTier`, `AdditionalModelRequestFields` with `ProviderKey() = "amazonBedrock"` (and legacy alias `"bedrock"` parsed at request build time)
- [x] 2.3 Add `model.go` with `model` struct (modelID, providerName, baseURL resolver, credentials, http client, signer, generateID, headers) and `New(modelID, opts...) provider.LanguageModel` constructor
- [x] 2.4 Implement `SpecificationVersion()` returning `"v4"`, `Provider()` returning `"amazon-bedrock"`, `ModelID()`, `SupportedURLs()` returning nil
- [x] 2.5 Read `AWS_BEARER_TOKEN_BEDROCK` at construction when `WithBearerToken` is not used; document precedence over SigV4
- [x] 2.6 Add unit tests for construction, option application, and provider identity

## 3. AWS authentication (SigV4 + bearer)

- [x] 3.1 Implement `signing.go` with a `http.RoundTripper` wrapper that signs `POST` requests with bodies using `aws/signer/v4` and the configured credentials provider; non-POST or empty-body requests pass through unsigned
- [x] 3.2 Implement bearer-token mode: when set, the round-tripper appends `Authorization: Bearer <token>` and skips SigV4
- [x] 3.3 Honor `WithHeaders` by merging static headers before signing
- [x] 3.4 Set a `User-Agent` suffix `ai-sdk-go/bedrock/<version>` on outbound requests
- [x] 3.5 Unit tests: SigV4 headers present for POST, absent for GET, bearer mode skips signing, custom headers preserved, error from credentials provider surfaces as `*provider.APICallError`

## 4. Request conversion

- [x] 4.1 Add `api_types.go` with Go structs mirroring upstream `AmazonBedrockConverseInput` (system, messages, inferenceConfig, toolConfig, additionalModelRequestFields, additionalModelResponseFieldPaths, serviceTier) and content block types (text, image, document, toolUse, toolResult, reasoningContent, redactedReasoning, cachePoint)
- [x] 4.2 Add `convert_request.go` with `buildRequest(modelID string, opts provider.CallOptions) (request, []provider.Warning, requestMeta, error)` where `requestMeta` carries flags like `usesJSONResponseTool` and the tool-name mapping used during response decoding
- [x] 4.3 Implement system message handling, including provider-options-driven `cachePoint` injection between system blocks
- [x] 4.4 Implement user/assistant message conversion: text, image (data URLs decoded to base64; URL-only images emit unsupported warning), document blocks, tool calls, tool results (collapsed into user role per Converse contract)
- [x] 4.5 Implement reasoning content conversion for assistant messages: `reasoningText` with optional signature, and `redactedReasoning` from provider metadata
- [x] 4.6 Implement inference config mapping: `maxOutputTokens`, `temperature` (with clamping + warning), `topP`, `topK`, `stopSequences`
- [x] 4.7 Implement `convert_tools.go` `prepareTools` returning `toolConfig`, `additionalTools` (Anthropic provider-tool `tool_choice` pass-through), `betas`, and warnings; handle `auto`, `required`, `none`, and specific tool choice; filter `anthropic.web_search_20250305` with a warning
- [x] 4.8 Implement Anthropic-specific pass-throughs into `additionalModelRequestFields`: `thinking` (enabled/adaptive with budget), `output_config.effort`, `anthropic_beta` (union of caller and tool betas), `output_config.format` for native structured output; emit warnings for non-Anthropic models receiving Anthropic-only options
- [x] 4.9 Implement OpenAI (`reasoning_effort`) and Nova (`reasoningConfig`) effort routing for non-Anthropic models, gated by model-ID prefix
- [x] 4.10 Implement JSON-response-tool fallback for non-supported-structured-output models: synthesize `json` tool, force `toolChoice = required`, set flag for response-side text collapse
- [x] 4.11 Drop `temperature`/`topP`/`topK` when thinking is enabled and emit warnings; clamp temperature to `[0,1]` otherwise
- [x] 4.12 Filter tool content from prompt when no tools active, emitting `toolContent` warning (parity with upstream)
- [x] 4.13 Unit tests for each mapping using table-driven cases; cross-check JSON output against upstream test snapshots where available

## 5. Response conversion (non-streaming)

- [x] 5.1 Add `convert_response.go` with `parseResponse(raw json.RawMessage, mapping toolNameMapping, usesJSONResponseTool bool) (*provider.GenerateResult, error)`
- [x] 5.2 Decode `output.message.content`, iterating text, toolUse, reasoningContent, redactedReasoning blocks
- [x] 5.3 Implement `mapFinishReason(stopReason, isJsonResponseFromTool) -> provider.FinishReason` matching upstream mappings (`end_turn`/`stop_sequence` -> stop, `max_tokens` -> length, `content_filtered`/`guardrail_intervened` -> content-filter, `tool_use` -> tool-calls unless JSON-tool collapse, default -> other)
- [x] 5.4 Implement `convertUsage(usage)` returning `provider.Usage` with cache token detail (`InputTokens.NoCache/CacheRead/CacheWrite/Total`)
- [x] 5.5 Build provider metadata under `amazonBedrock` (and legacy alias `bedrock`): `trace`, `performanceConfig`, `serviceTier`, cache usage details, `isJsonResponseFromTool`, `stopSequence`
- [x] 5.6 Build `Response` metadata from headers: `x-amzn-requestid`, `date`, modelID
- [x] 5.7 JSON-response-tool collapse: emit text part with `JSON.stringify(input)` when the synthetic `json` tool fires
- [x] 5.8 Mistral tool call id normalization via `normalize_tool_call_id.go`
- [x] 5.9 Unit tests using upstream `__fixtures__` non-streaming JSON responses (where available) and hand-built fixtures for edge cases

## 6. Event-stream decoder

- [x] 6.1 Add `eventstream.go` implementing a Smithy event-stream frame decoder: read 4-byte total length, 4-byte headers length, prelude CRC, headers (string/byte/int types we need), payload, message CRC; yield `(headers map[string]string, payload []byte)` pairs to a callback
- [x] 6.2 Extract `:message-type`, `:event-type`, `:exception-type`, `:content-type` headers from each frame
- [x] 6.3 Wrap incomplete-frame partial reads with a buffer that retains across `Read` calls
- [x] 6.4 Unit tests with handcrafted byte fixtures for: minimal frame, frame with multiple headers, multi-frame payload split across reads, exception frame, malformed frames (truncated, bad CRC)

## 7. Stream conversion

- [x] 7.1 Add `convert_stream.go` consuming decoded `(eventType, payload)` events and emitting `provider.StreamPart` on a buffered (≥64) channel
- [x] 7.2 Track per-content-block state (text vs tool vs reasoning) keyed by `contentBlockIndex` to issue matching `*Start`/`*Delta`/`*End` parts
- [x] 7.3 Handle `messageStart` -> response metadata part (id from `x-amzn-requestid`)
- [x] 7.4 Handle `contentBlockStart` with `start.toolUse` -> `PartToolInputStart`
- [x] 7.5 Handle `contentBlockDelta` with `delta.text` -> `PartTextDelta`; with `delta.toolUse.input` -> `PartToolInputDelta`; with `delta.reasoningContent.text` -> `PartReasoningDelta`; with `delta.reasoningContent.signature` -> attach to last reasoning block
- [x] 7.6 Handle `contentBlockStop` -> matching `*End` part based on tracked block kind; for tool blocks emit final accumulated JSON
- [x] 7.7 Handle `messageStop` with `stopReason` -> finish reason set
- [x] 7.8 Handle `metadata` with `usage` and `metrics` -> usage part + provider metadata
- [x] 7.9 Handle exception frames (`internalServerException`, `throttlingException`, `validationException`, `modelStreamErrorException`) -> `PartError` carrying `*provider.APICallError` with correct `IsRetryable`; close channel after
- [x] 7.10 Handle mid-stream transport errors after a 2xx with synthesized retryable `*provider.APICallError`
- [x] 7.11 Respect `ctx.Done()`: cancel the HTTP request, close channel without panicking
- [x] 7.12 Unit tests using upstream `__fixtures__` `.chunks.txt` (text, tool-call, reasoning, json-tool, tool-no-args) re-encoded into binary event-stream frames

## 8. HTTP integration in `model.DoGenerate` / `model.DoStream`

- [x] 8.1 Implement `DoGenerate`: build request, POST to `<baseURL>/model/{modelID}/converse` with body JSON, signed via the configured round-tripper, decode JSON body with `parseResponse`, surface non-2xx as `*provider.APICallError` with status-code-based retryability
- [x] 8.2 Implement `DoStream`: build request, POST to `<baseURL>/model/{modelID}/converse-stream`, kick off goroutine that runs the event-stream decoder, emit `PartStreamStart` with warnings followed by decoded events
- [x] 8.3 Wire context cancellation through both `DoGenerate` and `DoStream`
- [x] 8.4 Add `wrap_api_error.go` translating common error shapes (HTTP status code + JSON body decode + Bedrock exception name) into `*provider.APICallError`
- [x] 8.5 Unit tests using `httptest.Server` for: successful generate, non-2xx error, streaming success, streaming exception event, transport disconnect, context cancellation
- [x] 8.6 Integration test (build-tagged) `TestE2EBedrock*` against the real Bedrock endpoint, gated by env vars; skip when unset

## 9. Registry integration

- [x] 9.1 Add a `Provider` type satisfying `registry.Provider` with `LanguageModel(modelID string) provider.LanguageModel` returning a model constructed with module-level options
- [x] 9.2 Add a `NewProvider(opts ...Option) Provider` constructor capturing options at registration time
- [x] 9.3 Unit tests for composite ID resolution and middleware composition

## 10. Conformance harness updates

- [x] 10.1 Extract framing strategy in `test/conformance/runner.go` behind a `Framing` interface (`SSE` and `Bedrock` implementations); existing Anthropic/Grafana paths keep using `SSE`
- [x] 10.2 Implement Bedrock framing: encode each fixture line as a Smithy event-stream binary frame with `:event-type=<outer key>` and the inner JSON object as payload; response `Content-Type: application/vnd.amazon.eventstream`
- [x] 10.3 Allow a `TestCase` to declare its framing (inferred from provider directory, e.g. `test/conformance/bedrock/...` -> Bedrock framing)
- [x] 10.4 Add `RecordingServer` option in runner that captures request bodies into `request.json` for `recorded/` cases when present (for stricter assertion later)
- [x] 10.5 Unit tests for the binary framing encoder; round-trip one fixture line through the Go event-stream decoder to confirm symmetry

## 11. Conformance Bedrock test runner

- [x] 11.1 Add `test/conformance/bedrock/conformance_test.go` (build tag `conformance`) mirroring `anthropic/conformance_test.go`
- [x] 11.2 Provider factory constructs `bedrock.New(cfg.Model, bedrock.WithBaseURL(baseURL), bedrock.WithCredentials(stubCreds), bedrock.WithGenerateID(idGen))` where `stubCreds` returns dummy AWS credentials so SigV4 signing runs but the replay server doesn't validate it
- [x] 11.3 Discover cases under `test/conformance/bedrock/{upstream,recorded}/`
- [x] 11.4 Verify build-tag gating: without `-tags conformance` the file does not compile into normal tests

## 12. Fixture import: upstream Bedrock

- [x] 12.1 Create `test/conformance/bedrock/upstream/` with one directory per upstream fixture: `text`, `reasoning`, `tool-call`, `tool-no-args`, `json-only-text-first`, `json-other-tool`, `json-tool`, `json-with-tool`, `json-with-tools`, `json-tool-text-then-weather-then-json`, `json-tool-with-answer`
- [x] 12.2 Each directory gets a `config.yaml` with `model` (e.g. `anthropic.claude-sonnet-4-5-20250929-v1:0` matching the upstream test) and any required `tools`/`responseFormat`
- [x] 12.3 Copy the `.chunks.txt` fixture file from `@ai-sdk/amazon-bedrock/src/__fixtures__/` to `input.chunks.txt`; multi-step fixtures use `input-1.chunks.txt`, `input-2.chunks.txt`
- [x] 12.4 Generate `expected.jsonl` for each via `tools/generate.mts` (TS side) — 8/10 pass byte-identical, 2 skipped (known divergence in root streamtext text-delta ordering around tool-input-error)
- [x] 12.5 Add `test/conformance/bedrock/upstream/INDEX.yaml` listing every upstream fixture name with imported directory or `null`

## 13. TypeScript generator and recorder

- [x] 13.1 Add Bedrock provider switch to `test/conformance/tools/common.mts`: factory that returns an `@ai-sdk/amazon-bedrock` provider pointing at a local test server, with deterministic ID generator and a stubbed credential provider
- [x] 13.2 Update `tools/generate.mts` to handle Bedrock fixtures: pipe Bedrock JSON chunks through a local TS replay server that emits Smithy event-stream binary frames, run the upstream `amazonBedrock(modelID)` against it, write `expected.jsonl`
- [x] 13.3 Update `tools/record.mts` to support `--provider bedrock`: configure real Bedrock credentials via env (`AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` or `AWS_BEARER_TOKEN_BEDROCK`), record raw event-stream JSON chunks, save to `recorded/<name>/input*.chunks.txt`
- [x] 13.4 Add a small `--scenario` selector so individual cases can be re-recorded without touching the rest (already supported by record.mts and generate.mts globally; the Bedrock additions work with the existing flag)

## 14. Recorded fixture authoring

- [x] 14.1 Author `config.yaml` for `simple-text`, `tool-call`, `parallel-tool-calls`, `thinking-text`, `thinking-tool-use` under `test/conformance/bedrock/recorded/` (multi-step-tool-call dropped: upstream Bedrock SDK Zod schema validates `toolUse.input` as nonoptional, which terminates the orchestration loop before the second model call when the live API omits the field)
- [x] 14.2 Run `record.mts --scenario bedrock/recorded/<name>` against the workloads-dev AWS profile for each; commit the resulting `input*.chunks.txt` and `expected.jsonl`. Recorded fixtures use `us.anthropic.claude-haiku-4-5-20251001-v1:0` (small, fast) and `us.anthropic.claude-sonnet-4-5-20250929-v1:0` (thinking-capable)
- [x] 14.3 Verify `make test-conformance` passes locally for the newly recorded set

## 15. Documentation and release plumbing

- [x] 15.1 Add `providers/bedrock/README.md` covering: install (`go get`), construction, region/credentials, Anthropic-on-Bedrock provider options, examples for streaming and tool calling, link to upstream reference
- [x] 15.2 Update root `README.md` to list the Bedrock provider in the "Providers" section with a short blurb and link
- [x] 15.3 Add Bedrock to renovate.json if necessary so AWS SDK v2 updates are picked up — not necessary, the existing gomod matchers pick up the new module automatically
- [x] 15.4 Verify `make check` (fmt + vet + test) passes for the new module; verify `make test-conformance` is green (Go side confirmed; TS `typecheck-conformance` needs pnpm which isn't installed in this environment but works in CI)

## 16. Wire up issue and PR

- [x] 16.1 Open a draft PR titled `feat(bedrock): add AWS Bedrock provider with conformance testing` referencing issue #30 — opened as https://github.com/grafana/ai-sdk/pull/194
- [x] 16.2 PR description summarizes: new module, AWS SigV4 + bearer auth, Converse request/response/stream conversion, Anthropic-on-Bedrock pass-through, conformance fixtures imported from upstream + recorded coverage
- [x] 16.3 Close issue #30 on merge — wired via `Closes #30` in commit message and PR body
