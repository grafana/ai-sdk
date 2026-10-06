## Why

The repo ports Vercel's AI SDK to Go but has no OpenAI provider. In upstream, the
OpenAI Responses API (`POST /v1/responses`) is the default endpoint for OpenAI
models and is where active development happens: a richer event model,
server-executed built-in tools (web search, code interpreter, file search, image
generation, MCP, shell, apply-patch, tool search), native reasoning with
encrypted content plus summaries, citations/sources, structured output, and
stateless conversation continuity via `previous_response_id`/`conversation`.
Without it, Go consumers cannot target OpenAI at all, and the project's "drop-in
parity with upstream" promise has a major gap. This change resolves issue #23.

## What Changes

- Add a new Go module `providers/openai` (own `go.mod`, `replace` to root) that
  implements `provider.LanguageModel` against the OpenAI Responses API using the
  official `github.com/openai/openai-go/responses` SDK for transport, params, and
  SSE event types.
- Add constructor `NewResponses(apiKey, modelID string, opts ...Option) provider.LanguageModel`
  (anthropic-style per-model constructor; leaves room for a future `NewChat` and a
  provider facade per #22). Add `WithRequestOptions(...)` and any
  transport/base-URL options needed for testing.
- **Request conversion** (`provider.CallOptions` -> Responses request): map
  system messages to `instructions`/`system`/`developer` per `systemMessageMode`;
  user/assistant/tool messages to the `input` item array (text, images via
  `input_image`, files via `input_file`, `function_call`, `function_call_output`,
  reasoning items, `item_reference` when `store && id`, tool-approval responses,
  compaction); scalar params; structured-output `text.format`; full tool
  preparation and `tool_choice` resolution including `allowed_tools`.
- **Built-in (provider-executed) tools** with full parity: `web_search`,
  `web_search_preview`, `code_interpreter`, `file_search`, `image_generation`,
  `local_shell`, `shell`, `apply_patch`, `mcp` (incl. approval flow),
  `tool_search`, `computer_use`. Each maps to `provider.ContentPart` /
  `provider.StreamPart` with `ProviderExecuted` and appropriate metadata, plus
  citation `source` parts and `include` auto-population.
- **Non-streaming conversion** (`DoGenerate`): map every Responses output item
  type to content parts, usage, finish reason, provider metadata, and warnings.
- **Streaming conversion** (`DoStream`): a stateful SSE state machine mapping the
  full Responses event protocol (`response.created`, `output_item.added/done`,
  `output_text.delta/done`, `function_call_arguments.*`, reasoning summary
  events, code-interpreter/apply-patch deltas, annotations, `response.completed`/
  `incomplete`/`failed`, `error`, unknown-chunk fallback) to `provider.StreamPart`.
- **Typed provider options** (`ProviderKey() == "openai"`): `previousResponseId`,
  `conversation`, `instructions`, `reasoningEffort`, `reasoningSummary`,
  `truncation`, `store`, `metadata`, `include`, `maxToolCalls`,
  `parallelToolCalls`, `serviceTier`, `textVerbosity`, `user`, `logprobs`,
  `strictJsonSchema`, `systemMessageMode`, `forceReasoning`, `allowedTools`,
  `promptCacheKey`, `promptCacheRetention`, `safetyIdentifier`,
  `passThroughUnsupportedFiles`, `contextManagement`, plus per-tool and per-part
  options needed for round-tripping (`itemId`, `reasoningEncryptedContent`,
  `approvalRequestId`, `imageDetail`, `namespace`, `phase`).
- **Model capability detection** mirroring upstream
  `getOpenAILanguageModelCapabilities` (reasoning-model detection,
  `systemMessageMode`, flex/priority processing, non-reasoning-parameter support)
  to gate parameter stripping and emit `Warning`s instead of errors.
- **Error mapping** translating OpenAI SDK errors to `provider.APICallError`
  (status, headers, body, retryability, structured error data) for both
  `DoGenerate` and emitted stream `PartError`s.
- **Full conformance suite**: a new `test/conformance/openai/` provider directory
  with `upstream/` (copied from Vercel fixtures) and `recorded/` cases, an
  `openai/conformance_test.go` wired to the shared runner, plus the runner
  `requestHeaderAllowlists` entry, any OpenAI-specific request-body
  normalization, and the `generate.mts`/`record.mts`/`common.mts` plumbing so
  expected goldens regenerate without API keys.
- **Full unit-test suite** mirroring the anthropic provider: request, response,
  stream, tool-prep, options, capabilities, and an `httptest`-backed
  error/DoStream path test.

## Capabilities

### New Capabilities
- `openai-responses-provider`: The OpenAI Responses API provider — constructor,
  module layout, request conversion, non-streaming and streaming response
  conversion, built-in/server-executed tools, reasoning, sources/citations,
  structured output, typed provider options, model capability detection, and
  error mapping; full behavioral parity with upstream Vercel's Responses
  implementation.

### Modified Capabilities
- `conformance-testing`: Extend the conformance suite to support the OpenAI
  provider — the runner gains an OpenAI request-header allowlist and OpenAI body
  normalization, auto-discovery of `test/conformance/openai/{upstream,recorded}`
  cases, and the TS tooling (`generate.mts`/`record.mts`/`common.mts`) gains an
  OpenAI branch so OpenAI fixtures generate/record/replay like Anthropic.

## Impact

- **New module**: `providers/openai/` (`go.mod` with `replace ../../`), depending
  on root `github.com/grafana/ai-sdk` + `github.com/openai/openai-go` + testify.
- **New dependency**: `github.com/openai/openai-go` (responses package) in the new
  provider module only; root module unaffected.
- **Test infra**: `test/conformance/runner.go` (allowlist + normalization),
  `test/conformance/tools/{common,generate,record}.mts` and their
  `package.json` deps (`@ai-sdk/openai`), new `test/conformance/openai/` tree.
- **Build/CI**: `Makefile` and `.github/workflows/ci.yml` gain the new module in
  build/vet/lint/test matrices and the conformance job picks up the OpenAI cases
  (committed fixtures, no API keys).
- **Docs**: a `docs/providers/openai.md` page (setup/behavior) and `doc.go`
  godoc for the new package; root README router entry.
- **No changes** to the root `aisdk` orchestration, the `provider` interface, or
  existing providers.
