# Proposal

## Why

Issue #377: the Anthropic adapter emits a `PartFinish` on every `message_delta`, while the registered upstream (`@ai-sdk/anthropic` 4.0.71, `eb77f09e`; the issue was filed against 4.0.65, `5d12eaa6`) emits exactly one `finish` per message, when it handles `message_stop`. Direct `DoStream` callers and the AI Gateway, which treats the first finish as terminal, can see an early finish that lacks the final `safeguardResults`, and raw `message_stop` arrives after `finish`. Issue #201 adds the opposite case: the imported `programmatic-tool-calling` fixture has 15 `message_stop` events but only 2 `message_delta` events, so Go emits 2 finishes where upstream emits 15.

The conformance suite could not catch either: it compares only UI chunks from `streamText`, so provider-part timing and count are invisible. This change therefore adds upstream-generated provider-part evidence for every provider with conformance fixtures, investigates every difference it surfaces, and fixes them until the suite is green.

## What Changes

- Add a provider-parts conformance golden: an always-on `expected-provider-parts.jsonl` for every streaming case of Anthropic, Bedrock, OpenAI and OpenAI-compatible, recorded from the pinned upstream model calls (one entry per call, raw chunks included) during the existing generation run and compared with the Go provider parts of the same run. Differences unrelated to this issue are triaged, not hidden (see design).
- Add synthetic stream-part cases (outside `recorded/` and `upstream/`) for scenarios no real fixture has: multiple deltas per message, EOF after delta, error frames, first-frame error. Inputs are labeled synthetic; expected output comes from the pinned upstream TypeScript.
- Emit `PartFinish` only when `message_stop` closes a message, using finish reason, usage and metadata accumulated since `message_start` and `message_delta`. One finish per stop.
- Raw parts arrive before the parts produced by handling their frame: `raw(message_delta), raw(message_stop), finish`.
- A message with no `message_delta` now produces a finish; the finish reason is stream-level as upstream (updated by non-null `message_start.stop_reason` and `message_delta`).
- A stream that ends after `message_delta` without `message_stop` produces no finish.
- **BREAKING (internal)**: Anthropic `error` frames no longer end the Go stream. Like upstream, the adapter emits the error part and keeps reading; a finish follows only if `message_stop` does. Transport and decode failures remain terminal. An error as the first frame still fails `DoStream`.
- Verify with a provider-independent core UI fixture how `StreamText` handles error-then-finish against upstream `streamText`; fix core only if the golden shows a mismatch (see tasks).
- Malformed `safeguard_results` keeps failing at the offending `message_delta`.
- Fix the differences the golden surfaced in the other providers:
  - Bedrock: raw chunks are wrapped under their event or exception type with the AWS padding field dropped, and `tool-input-start` no longer carries a duplicate `toolCallId`.
  - OpenAI: tool-call input strings are not HTML-escaped and follow upstream field order (apply-patch, local shell); the streaming finish reports the response ID from `response.created`.
  - OpenAI-compatible: `tool-input-*` parts carry `id` only (and the tool name on start), not duplicate `toolCallId`/`toolName`.
- Make provider metadata and the safeguard verdict stream-level, as upstream: a verdict or stop sequence from one message can appear on a later message's finish. This replaces the `anthropic-safeguards` requirement to reset the verdict per message.
- Remove the remaining normalizations where Go can match upstream:
  - `stream-start` always serializes `warnings`, and response timestamps serialize as UTC with milliseconds.
  - Anthropic error frames carry upstream's message, status code and retryability per error type, for first-chunk and mid-stream errors.
  - Anthropic raw usage iterations keep only the fields upstream's schema declares.
- Report `inputTransformations` (dropped or rewritten input blocks) in Anthropic provider metadata for unary and streaming calls, as upstream does. Go previously had no counterpart. Cover it with unit tests and with synthetic unary and streaming cases whose expectations come from the pinned upstream.
- Out of scope: Gateway first-finish terminal rule for sequential messages (#393, updated separately), and the `thinking.blockBinding` request option that causes transformations.

## Capabilities

### New Capabilities
- `anthropic-input-transformations`: reporting of input transformations in unary and streaming provider metadata.
- `anthropic-stream-finish-lifecycle`: when and how often the Anthropic stream adapter emits `PartFinish`, ordering relative to raw parts, behavior for incomplete streams, and handling of `error` frames.

### Modified Capabilities
- `anthropic-safeguards`: the streaming verdict requirement refers to "the last PartFinish" and the timing-boundary requirement says finish timing is unchanged; both must follow the single-finish-on-stop contract.
- `conformance-testing`: adds provider-parts goldens and the synthetic stream-part case category.
- `provider-v4-core-types`: stream part JSON follows the V4 shape for `stream-start` warnings and timestamps.
- `bedrock-provider`: raw chunks mirror upstream's envelope; tool-input parts are identified by id.
- `openai-responses-provider`: tool-call input serialization and the finish response ID match upstream.
- `openai-compatible-stream-metadata`: tool-input parts are identified by id.

## Impact

- `providers/anthropic/convert_stream.go`, `convert_usage.go` (finish lifecycle, stream-level usage, iteration filter), `stream_transport.go`, `model.go` (error frames become non-terminal) and `wrap_api_error.go` (upstream's error classification).
- `provider/stream_part.go` (V4 `stream-start` and timestamp JSON).
- `providers/bedrock/convert_stream.go`, `providers/openai/stream_adapter.go`, `convert_response.go`, `sdk_helpers.go`, `providers/openai-compatible/stream.go` (shape fixes surfaced by the golden).
- `test/conformance`: `tools/generate.mts`, `runner.go`, a new `expected-provider-parts.jsonl` in every streaming case directory of the four providers (inputs untouched), new `testdata/anthropic-stream-parts/`, new `ui/` error-then-finish fixture, `PARITY.md`.
- Tests that feed a delta without `message_stop` in `providers/anthropic` and `providers/azure` need a stop appended.
- `aisdk.StreamText`: unchanged for complete streams; a stream truncated after `message_delta` takes the partial/no-output path; an error followed by a finish matches upstream `streamText` (core UI fixture `ui/error-then-finish`).
- AI Gateway Anthropic streaming: finish frames carry final metadata and follow the raw `message_stop`; frame shape unchanged.
