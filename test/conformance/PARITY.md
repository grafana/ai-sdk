# Upstream Parity Coverage

This document describes what the conformance baseline verifies against the
registered upstream target in `upstream.yaml`. It is a coverage map, not a claim
of total parity.

## Status Values

- `automated`: covered by committed tests or validation scripts.
- `manual`: requires source or test comparison during implementation or review.
- `documented-deviation`: intentionally different from upstream behavior or CI policy.
- `gap`: known missing coverage or metadata.

## Confidence Model

Conformance tests are both a parity checker and the repository's behavioral
confidence suite. Prefer them for any bug fix or feature whose behavior can be
expressed as recorded provider input plus expected upstream output. Use
hand-written Go tests for local invariants, error paths, and small helpers that
do not cross a provider or UI wire boundary.

When a reported bug can be reproduced by replaying provider chunks or asserting
provider requests, add or update the conformance fixture first, observe the Go
failure, and then fix the implementation. When adding a new parity-sensitive
feature, record or import upstream behavior alongside the implementation so the
fixture becomes the executable contract for future upgrades.

## Layered Coverage Map

### Core ai-sdk Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `StreamText` orchestration lifecycle | mixed | `expected.jsonl` covers start, step, finish, incomplete provider streams, and selected multi-step flows; root tests cover provider-ordered step content, locally generated tool-part reconciliation, per-step first-content timeouts, and semantic inter-content timeouts. | Intentional Go deviation: upstream `ai@7.0.40` emits locally generated approval requests adjacent to their tool calls during stream transformation, while Go batches local approval handling after provider streaming and appends those requests after recorded provider content. Both `StepResult.Content` and response messages preserve their SDK's respective order. Add fixtures for newly supported step options or stop conditions. |
| `TextStreamPart` to `UIMessageChunk` conversion | automated | `expected.jsonl` compares Go output with upstream `toUIMessageStream()` output, including the provider-independent generated-file golden for file and reasoning-file chunks. | New chunk types require fixture coverage before being treated as complete. Intentional Go deviation: when original messages are explicitly present and no custom UI stream ID generator is supplied, Go uses its default `GenerateID` to preserve the historical `OriginalMessages != nil` persistence sentinel behavior; upstream only injects a generated ID when `generateMessageId` is supplied, except for last-assistant continuation ID reuse. |
| UI message wire format | automated | `expected.jsonl` verifies chunk type names, JSON fields, ordering, and deterministic IDs. | Frontend-specific hook behavior still requires source review. |
| UI message stream readers | mixed | Go unit tests cover `StreamUIMessage` progressive snapshots, `AssembleUIMessage` final assembly, partial tool input JSON repair, snapshot cloning, lazy IDs, metadata merging, finish-state callbacks, repeated tool-call IDs across steps, reasoning files, source document filenames, loose known-chunk decoding, unknown discriminator rejection, and default `ChunkError` handling compared against `ai@7.0.40` source. | Reader options intentionally do not yet expose upstream `message`, `onError`, or `terminateOnError`; `StreamUIMessage` follows upstream's default `terminateOnError=false` for error chunks. Current Go UI message types do not yet model upstream `custom` chunks/parts, nor upstream-only tool UI fields such as `title`, `toolMetadata`, `rawInput`, and `preliminary`; those remain documented representation gaps rather than hidden reader guarantees. |
| SSE framing and parsing | automated | Conformance replay and framing tests verify fixture framing and stream parsing behavior. | Browser transport behavior is outside this suite. |
| Tool orchestration | mixed | Tool call, no-arg tool, approvals, parallel tools, selected multi-step fixtures, `toolChoice`, `activeTools`, tool provider options, and tool error simulation are configurable in conformance. Root tests cover upstream-compatible injective tool-approval signatures and guarded legacy verification. | Add fixtures for newly reported tool-loop behavior. |
| Agent / ToolLoopAgent core orchestration | mixed | Root unit tests cover Agent identity, settings/per-call merge, `StepCountIs(20)` Agent default, direct `StreamText` one-step default preservation, callback merging, runtime context propagation, provider call-header marker insertion, approval/external/provider-executed tool inheritance, structured output, and stream error propagation. | No dedicated conformance fixture yet because `ToolLoopAgent` delegates to the existing `StreamText`/`GenerateText` path. Upstream `prepareCall`, `allowSystemInMessages`, `experimental_download`, `include`, `_internal` ID generators, tools-context/call-options-template behavior, telemetry, stream transforms, sandbox sessions, repair/refine hooks, `toolOrder`, TypeScript generic/schema inference, and `callOptionsSchema` are documented gaps or Go adaptations, not silent options. Provider network User-Agent/header behavior is a gap beyond the root `provider.CallOptions.Headers` boundary. |
| Structured output | automated | Fixtures with `expected-object.json` validate object, array, choice, and raw JSON output paths. The OpenAI `structured-json-output-length` fixture asserts the parsed `OutputValue` for a non-`stop` finish reason. Root unit tests cover ordered partial snapshots and array element streams, including delayed consumption beyond the channel buffer. | Parse and validation failures remain unit-tested because fixtures currently model successful output expectations only. `PartialOutputStream` and `ElementStream` are Go result APIs rather than provider or UI wire boundaries, so their delivery semantics remain unit-test covered. Upstream's stable `repairText` callback on the legacy object APIs is not exposed by the Go `Output` abstraction. |
| Error and warning behavior | mixed | Truncated-stream fixtures cover upstream-visible no-output errors; other paths require Go unit tests and upstream source comparison. | Add conformance fixtures when additional upstream-visible errors can be represented in replay. |
| Usage, finish reason, and metadata propagation | mixed | Existing fixtures validate covered chunk fields and request snapshots; opt-in `expected-usage.json` snapshots compare per-step provider usage. | Add targeted fixtures for newly exposed metadata. |
| Cancellation and abort behavior | automated | Provider-independent `ui/stream-abort` fixtures compare actual `StreamText` cancellation UI chunks for no-output, partial-output, and pending-output streams against abort behavior re-attested from `ai@7.0.40` source and tests; focused Go tests cover `GenerateText` cancellation during and immediately after tool execution. | Provider transport cancellation remains covered by provider-specific unit tests rather than timed replay fixtures. |
| Realtime transport | gap | Upstream source review is required today. | The Go port does not expose upstream realtime model or browser transport APIs, including raw string and binary WebSocket event serialization or OpenAI realtime speech translation. |

### Provider Contract Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `provider.LanguageModel` vs upstream `LanguageModelV4` | mixed | `mise run parity-provider-shape` reports discriminator drift; semantic interface changes still require source comparison. | Exact method/field-shape equivalence remains manual. |
| Speech translation model contract | gap | Source review against provider v4 and core `streamTranslate` is required today. | The Go port intentionally scopes its provider boundary to LanguageModel and does not expose the experimental speech-translation model, core streaming orchestration, OpenAI realtime translation factory, or conformance coverage. |
| `provider.CallOptions` mapping | mixed | Request snapshots validate behavior-affecting fields that current fixtures exercise. | New option fields require either fixture coverage or documented gaps. |
| Message and content part taxonomy | mixed | Provider request snapshots cover current message/content variants. The Anthropic `ui-tool-model-output` fixture verifies that persisted UI tool results apply `Tool.ToModelOutput` and reach the provider as multipart text/file content. | New content parts must include provider request assertions. Known Go conversion gaps versus `ai@7.0.40` include upstream `convertDataPart` support and some tool conversion semantics (`input-streaming` filtering default and output-denied default text); preserve existing Go behavior unless a focused parity change updates fixtures and docs. |
| Tool definitions and tool choice | mixed | Tool fixtures assert declared tool schemas and selected tool request behavior. | Forced choices and active tool subsets are known harness gaps. |
| Provider metadata and options passthrough | mixed | Provider options used by current fixtures are covered by request snapshots. | Provider-specific option expansion requires new fixtures. |
| Warning model and unsupported feature handling | manual | Source review and Go tests are required today. | Add request or stream fixtures when warning behavior affects upstream-visible output. |
| Stream part taxonomy | mixed | Provider fixture replay covers stream parts present in committed chunks. | New stream parts require upstream fixture import or recording. |

### Provider Implementation Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Anthropic request conversion | automated | `test/conformance/anthropic/**/expected-requests.jsonl` compares behavior-affecting requests. | Add fixtures for new Anthropic options, content parts, and beta headers. |
| Anthropic response stream parsing | automated | Anthropic fixture replay compares Go stream output with upstream expectations. | Newly observed event types need fixtures before release. |
| Anthropic provider-defined tools | mixed | Web search, code execution, and selected provider tool fixtures cover current paths. | Provider tool configuration fields remain fixture-driven. |
| OpenAI Responses request and stream conversion | automated | OpenAI request/output snapshots cover text, reasoning, structured output, hosted tools, native provider-tool continuation items, multi-step calls, stored references, client-executed `openai.computer` actions, and three-step `openai.programmatic_tool_calling` with caller/result replay. Provider unit tests cover legacy computer calls, stored-item replay, screenshot URL/file-ID output, missing output, action variants, future-family model defaults, and tool-search output item IDs. | Intentional Go deviation from `@ai-sdk/openai@4.0.22`: apply-patch tool results resolve configured provider-tool aliases before taxonomy dispatch; the baseline checks the literal `apply_patch` name. This preserves native call/output pairing for aliased provider tools and is covered by Go unit tests, while conformance snapshots use the canonical name. HTTP-level API errors remain unit-tested when replay cannot represent them. |
| Bedrock request conversion | automated | `test/conformance/bedrock/**/expected-requests.jsonl` compares behavior-affecting requests. | Header assertions intentionally exclude volatile SigV4 fields. |
| Bedrock response stream parsing | mixed | Bedrock fixtures cover text, tools, JSON, and reasoning paths. | Add fixtures as Bedrock support expands. |
| Grafana provider-wire transport | automated | Grafana conformance runs the real Grafana client through the public `gateway/providerwire` server and downstream Anthropic replay; `providers/grafana/providerwire_server_test.go` covers unary, ordered streaming, and mid-stream errors. The `test/interop` harness drives a real upstream `@ai-sdk/gateway` client against the public server (streaming text, tool-call round trip, provider-executed tool result, file input plus inline-data and URL-valued file/reasoning-file output, mid-stream + pre-stream errors). | This covers transparent transport behavior, not Grafana service behavior. The reusable Go server lifecycle has no direct upstream TypeScript server equivalent. |
| Provider-wire upstream encode compatibility | automated | The provider-wire encoders emit upstream `LanguageModelV4` JSON for every previously divergent shape (system-as-string, prompt tool-result single-`value`, streamed tool-result `result`/`isError`, generated-file `data`/`url` union for both file types, flat source, `{"type":"error","error":{...}}` stream part, unary tool-call `input` string, `{"error":{...}}` HTTP envelope); streamed tool-result metadata remains opaque, and decoders stay tolerant of both upstream and legacy Go encodings. Inline bytes/base64 and URL variants are covered by `provider/upstream_encode_compat_test.go`, `provider/upstream_decode_compat_test.go`, `gateway/providerwire/sse_test.go`, and `test/interop`. Supersedes decisions D6 (system as array) and D4 (error as `apiCallError`) from `2026-04-30-lossless-provider-wire`. | — |
| Provider error mapping | mixed | Go tests and upstream/source comparison cover HTTP errors; Anthropic initial SSE errors and OpenAI-compatible structured SSE errors have replay fixtures for upstream-visible stream behavior, while unit tests assert lossless provider error data. | Intentional Go deviation: Anthropic `api_error` and `overloaded_error` SSE failures are retry-eligible by provider cause, including after output, while the registered upstream only marks an initial `overloaded_error` retryable. Promoted Anthropic errors retain the full envelope in `Data` for gateway normalization while preserving the inner provider error in `ResponseBody`. Core only retries `DoStream` call failures; it never replays an established stream from a retryable `PartError`, because callers must separately prove that no output or effects escaped. HTTP-200 SSE `rate_limit_error` remains non-retryable pending evidence of its service timing and delay metadata. |
| Unsupported option warnings | manual | Go tests and source review are required today. | Track accepted gaps in this file or `upstream.yaml`. |

### Frontend Interop Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `@ai-sdk/react` `useChat` wire compatibility | automated | Integration tests consume Go UI message SSE through the upstream React hook package. | Add scenarios for tool approval and multi-step UI state as risk increases. |
| Agent UI stream helpers | mixed | Root unit tests cover pre-stream validation, UI-to-model conversion before streaming, original-message preservation, chunk equality with `StreamTextResult.ToUIMessageStream`, and HTTP SSE framing through the existing writer. | Helpers intentionally reuse the existing UI chunk/SSE path, so no new fixture is required unless emitted chunks change. Validation is a minimum Go-model validator; remaining upstream `validateUIMessages` differences, deeper schema/provider-specific validation, and unsupported upstream UI part features are gaps. |
| `useCompletion` compatibility | automated | Integration tests consume Go text streams through the upstream React hook package. | Add error/abort cases when those surfaces change. |
| `useObject` compatibility | automated | Integration tests consume Go streamed JSON through the upstream React hook package; conformance fixtures validate object output where `expected-object.json` exists. | Add schema-error cases when object validation behavior changes. |
| Chunk ordering and state transitions | automated | `expected.jsonl` preserves upstream chunk ordering for covered fixtures. | Add fixtures for new lifecycle states. |

### Conformance Harness Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Baseline package validation | automated | `mise run validate-parity-baseline` checks `upstream.yaml` against all parity TypeScript consumers: conformance tools, integration tests, interop tests, and CLI tooling. | The canonical baseline is the npm package versions recorded in `upstream.yaml`. |
| Upstream expectation generation | automated | `mise run generate-conformance` produces `expected.jsonl`, request snapshots, structured output expectations, and opt-in per-step usage snapshots. | Generation only covers fields supported by `config.yaml`. |
| Mature stable baseline upgrade | automated | `mise run parity-upgrade` selects the newest coherent stable package set from the npm `latest` release lines that satisfies pnpm's configured minimum release age, then regenerates expectations. | Divergences still require human classification and fixes. |
| Fixture config expressiveness | automated | Provider fixtures use YAML for models, prompts, tools, provider tools, approvals, provider options, JSON response format, `toolChoice`, `activeTools`, `streamOptions`, tool provider options, and tool error simulation. Provider-independent core UI fixtures replay `LanguageModelV4` stream parts directly. | Expand only when a fixture needs a new upstream-visible option. |
| CI enforcement | automated | Baseline validation, full conformance replay, interop, and integration are all required status checks on `main`, so any parity divergence blocks the merge. | Regenerated expectations must land in the same pull request as the behavior change, because a stale expectation now blocks merges rather than emitting a warning. |
| Upstream fixture import tracking | automated | Provider `upstream/INDEX.yaml` files track imported and missing upstream fixtures; `mise run parity-coverage` validates local inventory and upstream fixture index drift. | `null` entries are intentional coverage gaps until imported. |

## Review Rules

Parity-sensitive changes must classify every observed difference from upstream
as one of:

- parity-preserving Go adaptation
- intentional deviation
- implementation bug
- coverage gap

Intentional deviations and coverage gaps must be recorded in `upstream.yaml` or
this coverage map before the change is considered complete.

The codec-package move to `gateway/providerwire` is a source-breaking,
parity-preserving Go adaptation in the provider transport layer: it preserves
main's upstream-compatible protocol bytes and UI-message behavior. Intentional pre-commit response
fixes make nil or unencodable unary server results, unencodable API-call error
envelopes, and API-call errors with invalid final HTTP statuses return an
encodable retryable canonical HTTP 500 error instead of an empty implicit HTTP
200 or HTTP-server panic. The SSE reader also processes final-line bytes
returned together with `io.EOF`; canonical server-emitted frames and all valid
protocol bytes remain unchanged. No provider-wire or UI snapshot regeneration
is required.
