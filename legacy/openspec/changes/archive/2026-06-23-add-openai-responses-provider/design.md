## Context

The repo is a Go port of Vercel's AI SDK. Providers are independent Go modules
under `providers/<name>/` implementing `provider.LanguageModel` (the Go mirror of
`LanguageModelV4`). Two reference providers exist:

- `providers/anthropic` — uses a **vendor SDK** (`anthropic-sdk-go`) for transport
  and event types; converts between SDK types and `provider.*` types; streams via
  a buffered channel + goroutine driving a stateful `streamAdapter.handleEvent`.
- `providers/grafana` — uses **raw HTTP** + the repo's `provider/wire` SSE reader.

There is no OpenAI provider. Upstream's OpenAI Responses implementation is the
canonical reference (files under `packages/openai/src/responses/`). It is built on
plain HTTP (`postJsonToApi` + event-source handler), with a large set of
supporting modules: options schema, input conversion, tool preparation, usage
conversion, finish-reason mapping, capability detection, provider metadata, and
the chunk schema. The official `github.com/openai/openai-go` Go SDK exposes a
`responses` package (v1.12.0+) with request params, the non-streaming `Response`
type, and the full set of streaming event types and a streaming union — i.e. it
mirrors the Responses wire model the SDK port targets.

The three-layer event model is fixed by the repo: `provider.StreamPart` (raw) ->
`TextStreamPart` -> `UIMessageChunk` (SSE wire). This provider produces only the
first layer; orchestration and wire framing are owned by root `aisdk`.

Conformance is the project's primary correctness mechanism: recorded provider SSE
fixtures are replayed through both the Go and upstream TS SDKs and the resulting
`UIMessageChunk` sequences plus outbound request snapshots must be byte-identical.

## Goals / Non-Goals

**Goals:**
- Full behavioral parity with upstream Vercel's Responses implementation:
  request conversion, non-streaming + streaming response conversion, all
  built-in/server-executed tools, reasoning (encrypted + summaries), sources/
  citations, structured output, every documented provider option, and capability-
  gated parameter handling.
- A new `providers/openai` Go module exposing
  `NewResponses(apiKey, modelID, opts ...Option) provider.LanguageModel`.
- A full conformance suite (upstream + recorded) proving byte-identical
  `UIMessageChunk` output and request snapshots vs upstream, plus a full unit-test
  suite mirroring the anthropic provider's test organization.
- Idiomatic Go: typed string enums for discriminators, sentinel `Err`-prefixed
  errors, warnings-not-errors for unsupported features, graceful error
  propagation (no panics across the API boundary), buffered-channel streaming.

**Non-Goals:**
- Chat Completions (`/v1/chat/completions`) — that is issue #22 and a separate
  change. We only leave room for a future `NewChat`/facade.
- A provider facade / `registry.Provider` implementation now. Per-model
  constructor only.
- Azure-specific provider wiring beyond what the option-name fallback
  (`azure` -> `openai`) requires for parity; full Azure constructor deferred.
- Image/transcription/embeddings endpoints. Responses (text+tools+reasoning)
  only.
- Changing the `provider` interface, root orchestration, or existing providers.

## Decisions

### D1: Transport via `github.com/openai/openai-go/responses` SDK (not raw HTTP)
The user selected the official OpenAI Go SDK. The `responses` package provides
request params (`ResponseNewParams`), the `Response` type, and typed streaming
events with an `AsAny()` discriminated union — directly analogous to how the
anthropic provider consumes `anthropic-sdk-go`. This avoids hand-maintaining
request/response/event structs and tracking OpenAI wire churn.

- **Alternative considered**: raw HTTP + `wire.SSEReader` (grafana style, matches
  upstream's `postJsonToApi`). Rejected per user decision; would require porting
  the entire `openai-responses-api.ts` schema by hand.
- **Consequence / risk**: the SDK's struct/union shapes may not 1:1 match the wire
  field names upstream emits. Conformance is byte-level on the **request body** —
  so where the SDK omits/renames fields or changes JSON ordering, we must verify
  against `expected-requests.jsonl`. If the SDK cannot express a needed field, we
  fall back to the SDK's raw/extra-fields escape hatch (`WithExtraFields` / raw
  body option) rather than abandoning the SDK. This is the single biggest
  integration risk and is front-loaded in tasks (a transport spike).

### D2: Module layout — new `providers/openai` module, anthropic file split
Mirror anthropic exactly so the patterns and tests transfer:
`model.go` (struct + `NewResponses` + `DoGenerate`/`DoStream` + `consumeStream`),
`options.go` (`type Option func(*model)` + typed `OpenAIResponsesOptions` with
`ProviderKey() == "openai"` + per-tool/per-part option structs),
`convert_request.go` (`buildParams` equivalent: input items, tools, tool_choice,
params, structured output, provider options, capability gating),
`prepare_tools.go` (tool + tool_choice preparation; isolated like upstream),
`convert_response.go` (`DoGenerate` output-item -> content),
`convert_stream.go` (the `streamAdapter` state machine),
`convert_usage.go`, `finish_reason.go`, `models.go` (capabilities), `sources.go`
(annotations -> `source` parts), `wrap_api_error.go`, `doc.go`.

- `go.mod`: `module github.com/grafana/ai-sdk/providers/openai`, `go 1.26`,
  `replace github.com/grafana/ai-sdk => ../../`, require root + `openai-go` +
  testify.
- **Provider name**: `"openai"` (and the option-name is `"openai"`; an `azure`
  alias path is parsed for parity but not wired to a constructor yet).
- Compile-time check: `var _ provider.LanguageModel = (*model)(nil)`.

### D3: `store`-driven item references are the central conversion invariant
Upstream emits `{ type: "item_reference", id }` instead of inline content
whenever `store && id` (default `store=true`), and drops/inlines reasoning based
on `store` + `encrypted_content`. This single rule threads through assistant text,
tool-calls, tool-results, reasoning, and compaction. The Go `buildParams` carries
a resolved `store bool` and the `hasConversation`/`hasPreviousResponseId` flags
and applies the same branch order as upstream to guarantee request parity.

### D4: Capability detection mirrors upstream, gates params via warnings
Port `getOpenAILanguageModelCapabilities` as a pure prefix-matching function
returning a `modelCapabilities` struct (`isReasoningModel`, `systemMessageMode`,
`supportsFlexProcessing`, `supportsPriorityProcessing`,
`supportsNonReasoningParameters`). Unsupported params (`topK`, `seed`,
`presencePenalty`, `frequencyPenalty`, `stopSequences`; `temperature`/`topP` on
reasoning models unless effort `none` + non-reasoning-param support; `serviceTier`
flex/priority when unsupported; `reasoningEffort`/`reasoningSummary` on
non-reasoning models) emit `provider.Warning{Type: "unsupported"}` and are
stripped — never errors. `reasoningEffort` stays a free string (validated
server-side), per upstream.

### D5: Streaming state machine
`streamAdapter` holds per-stream mutable state mirroring upstream's local vars:
`ongoingToolCalls` keyed by `output_index` (toolName, toolCallId, plus
code-interpreter/apply-patch/tool-search sub-state), `ongoingAnnotations`,
`activeReasoning` keyed by item id (encrypted content + per-summary-index part
state), `activeMessagePhase`, `hasFunctionCall`, `responseId`, `usage`,
`finishReason`, `serviceTier`, `logprobs`, and the
`approvalRequestId -> toolCallId` maps (from prompt and from stream). It consumes
SDK events via the union switch and emits `provider.StreamPart`s with exactly the
upstream ordering (e.g. web-search emits start+end+call on `output_item.added`;
function-call emits end+call on `output_item.done`; code-interpreter seeds a
`tool-input-delta` prefix; apply-patch streams escaped diff deltas). Unknown
events map to a `PartRaw`/no-op (upstream `unknown_chunk`) and never error.

- **Determinism for conformance**: reasoning part ids use `itemId:summaryIndex`;
  message/text ids use the item id; source ids use an injectable
  `generateID` (set deterministically in tests via an option). This matches the
  anthropic approach and upstream's `_internal.generateId`.

### D6: Error mapping
`wrapAPIError(err, requestBody)` uses `errors.As` for the openai-go error type and
builds `provider.NewAPICallError(...)` with status code, response headers, raw
body, retryability, and structured error `Data` (parsed `{error:{...}}` envelope)
— matching anthropic's `wrap_api_error.go`. Stream errors are emitted as
`PartError` via a forced-wrap variant; `stream.Err()` after iteration likewise.
Network errors pass through for `DoGenerate` but are wrapped for stream parts (the
wire requires an `APICallError`).

### D7: Conformance integration follows the established extension pattern
- Create `test/conformance/openai/{upstream,recorded}/`, each case a dir with
  `config.yaml` + `input*.chunks.txt` + generated `expected.jsonl` /
  `expected-requests.jsonl`.
- Add `test/conformance/openai/conformance_test.go` (`//go:build conformance`)
  copying the anthropic file, swapping the factory to build the OpenAI provider
  with a base-URL option pointing at the replay server and a deterministic id
  generator.
- Extend `runner.go`: add `"openai"` to `requestHeaderAllowlists` (Authorization
  redacted, `OpenAI-*` headers normalized) and add any OpenAI-specific body
  normalization (e.g. tools array already sorted; verify `input` array order is
  preserved).
- Extend TS tooling: add `@ai-sdk/openai` to `tools/package.json`; add an OpenAI
  branch to `createModel()` in `generate.mts`/`record.mts`, `PROVIDER_BASE_URLS`
  + `checkAPIKeys` (`OPENAI_API_KEY`) in `record.mts`, and any builders in
  `common.mts`. Then `make generate-conformance` produces goldens with no keys.
- Seed `upstream/INDEX.yaml` mapping copied Vercel fixtures (from
  `packages/openai/src/responses/__fixtures__` and `__snapshots__`).

### D8: Test taxonomy
- **Unit (`providers/openai/*_test.go`)**: white-box `package openai`, testify,
  table-driven `t.Run` subtests, hand-written mocks, compile-time interface
  checks. Files mirror source: `convert_request_test.go`,
  `prepare_tools_test.go`, `convert_response_test.go`, `convert_stream_test.go`
  (with `unmarshalEvent`/`collectParts` drivers), `convert_usage_test.go`,
  `finish_reason_test.go`, `models_test.go`, `options_test.go`, and an
  `httptest`-backed `wrap_api_error_test.go` exercising the real `DoStream`/
  `DoGenerate` error path with `option.WithBaseURL`.
- **Conformance (recorded + upstream)**: per D7, gated by `-tags conformance`,
  runs in the non-blocking CI conformance job with committed fixtures.

## Risks / Trade-offs

- **SDK wire fidelity vs conformance** → The openai-go SDK may serialize requests
  differently from upstream (field omission, ordering, nullability). Mitigation:
  a front-loaded transport spike validating one request round-trips byte-identical
  to upstream's `expected-requests.jsonl`; use SDK raw/extra-field escape hatches
  where needed; the runner's body normalization (object-key order ignored, arrays
  preserved) absorbs benign ordering differences.
- **Breadth of the surface** → Full parity is large (12+ built-in tools, ~30 SSE
  event types). Mitigation: tasks are sequenced so a text-only happy path
  (request + stream + generate + one conformance case) is green early, then each
  tool/feature is added incrementally behind its own tasks and fixtures.
- **SDK version churn / pre-1.0 of unreleased features** → Some Responses features
  (tool_search, apply_patch, compaction, shell) may lag in the Go SDK.
  Mitigation: pin a known-good `openai-go` version; for any feature the SDK
  cannot express, fall back to raw-field injection and a recorded fixture, or mark
  that sub-feature as a follow-up with a tracked warning rather than blocking the
  whole change.
- **Reasoning/`store` branch complexity** → The `store`/`conversation`/
  `previousResponseId` interactions are subtle and easy to get wrong.
  Mitigation: port the upstream branch order verbatim and cover each path with a
  dedicated `convert_request_test.go` case plus an `upstream/` fixture.
- **Determinism** → Non-deterministic ids would break byte-level conformance.
  Mitigation: injectable `generateID` option set in tests; derive ids from item
  ids/indices exactly as upstream.

## Migration Plan

Additive only — new module, new test cases, new optional CI/Makefile entries. No
existing code paths change, so there is nothing to roll back beyond removing the
new module/directory. Ship behind no flag; the provider is opt-in via its
constructor. CI conformance for OpenAI is non-blocking initially (matches the
existing conformance job), so partial fixture coverage cannot break the main
pipeline.

## Open Questions

- ~~Exact pinned `github.com/openai/openai-go` version~~ **Resolved**: pinned
  `github.com/openai/openai-go/v3 v3.37.0` (the v3 major module, far ahead of
  v1.12.0, tracks the current Responses surface including `tool_search`,
  `apply_patch`, `shell`, `compaction`). The transport spike (task 1.3) confirmed
  the SDK serializes a simple text request byte-compatibly with upstream:
  `{"input":[{"content":"...","role":"system"},{"content":[{"text":"...","type":"input_text"}],"role":"user"}],"model":"...","temperature":...}`.
  No escape-hatch was required for the text path. The SDK uses `param.Opt[T]`
  for optional scalars and union types (`...UnionParam` with `OfX` fields +
  `ResponseInputItemParamOf*` constructors) for polymorphic fields.
- Whether any built-in tool (e.g. `tool_search`, `shell`, `apply_patch`,
  `compaction`) is not yet expressible via the SDK and must use raw-field
  injection or be deferred — to be determined empirically as each tool is ported
  (the v3 SDK exposes constructors for all of them; confirm field-by-field).
- Whether to add a minimal `examples/openai-responses` runnable example in this
  change or a follow-up (leaning follow-up to keep scope bounded).

## Implementation status and intentional deviations (post-implementation)

Verified by conformance (byte-identical chunks + request snapshots vs upstream)
for: simple text, reasoning summaries, JSON-schema structured output, function
tool calling, and `previous_response_id` continuation. Parity bugs caught and
fixed via conformance: `item_reference` requires an explicit `type` field; text
/reasoning/tool-call stream parts must carry `providerMetadata.openai.itemId`
(and reasoning carries `reasoningEncryptedContent`); function tool `strict` is
omitted unless true; the conformance factory must target `/v1/responses`.

Now implemented (in addition to the first batch):

- **All built-in tool request mapping**: web_search / web_search_preview (bare
  `web_search`/`web_search_preview` types), code_interpreter, file_search, mcp
  (with `require_approval` default `never`), custom, image_generation,
  local_shell, shell, apply_patch, tool_search.
- **Non-streaming + streaming conversion** of the rarer output items:
  `computer_call`, `image_generation_call`, `shell_call`(+output),
  `local_shell_call`, `apply_patch_call`, `tool_search_call`(+output),
  `custom_tool_call`, `compaction`, plus the common items.
- **Streaming code-interpreter delta seeding** (`{"containerId":...,"code":"` +
  JSON-escaped code deltas + closing `"}`), verified byte-identical to upstream.
- Web-search / code-interpreter tool results wrap the action/outputs in
  `{"action":...}` / `{"outputs":[...]}` using the SDK values' original wire
  JSON (`RawJSON()`) so zero-valued struct fields are not re-serialized — a
  parity bug caught by conformance.

Conformance coverage now: simple text, reasoning summaries, JSON-schema
structured output, function tool calling, `previous_response_id`, web search
with sources, and code interpreter — 7 upstream fixtures, all byte-identical.

A suite of nine live-recorded fixtures is committed under `openai/recorded/`
(recorded via `make record-conformance`), each passing Go conformance
byte-identically: `simple-text`, `tool-call`, `parallel-tool-calls`,
`multi-tool-chain`, `reasoning-text`, `reasoning-tool-use`, `web-search`,
`code-interpreter`, and `structured-json-output`. These exercise multi-turn
tool round-trips, reasoning with encrypted-content round-trips, provider-executed
tools, and structured output against the real API.

Recording against a Zero Data Retention (ZDR) org surfaced and fixed four real
upstream-parity bugs that the hand-authored fixtures had not exposed:

1. **`function_call` item id**: a non-stored re-sent function call must carry
   `id` (the `fc_...` item id from `providerMetadata.openai.itemId`); the
   provider was dropping it.
2. **`function_call_output` JSON-string ordering**: the tool result is an opaque
   JSON string; the conformance runner (Go + `common.mts`) now parses it for
   order-insensitive comparison, matching upstream's order-preserving
   serialization.
3. **Empty reasoning summary**: an empty reasoning part must serialize
   `summary: []`, not `[{type:"summary_text",text:""}]`.
4. **web_search action + streaming annotations**: the `web_search` tool-result
   `action` is reconstructed as `{type, query}` (camelCased `openPage`/
   `findInPage` for other subtypes), dropping the raw `queries` field; and
   `output_text.annotation.added` events are now accumulated into the `text-end`
   chunk's `providerMetadata.openai.annotations`, both matching upstream.

Not recordable on a ZDR org (covered by upstream fixtures + unit tests instead):
`previous_response_id` continuation (needs server-side storage) and MCP
tool-approval (needs a live remote MCP server).

Remaining deferred (genuinely):

- **Non-stored built-in tool *input* item round-trip** (multi-turn resends of
  prior `shell_call`/`apply_patch_call`/`tool_search_call`/`local_shell_call`/
  `custom_tool_call` assistant items and their outputs when `store=false`):
  these fall back to `function_call`/`function_call_output`. The default
  `store=true` path round-trips correctly for all tool types via
  `item_reference`.

Tooling note: `generate.mts` regenerates goldens for *all* providers and, under
the pinned `ai@7.0.0-beta.116`, emits a degenerate golden for the pre-existing
Anthropic `thinking-tool-signature-roundtrip` case. When regenerating OpenAI
goldens, restore non-OpenAI fixtures (`git checkout -- test/conformance/anthropic`)
to avoid clobbering them.
