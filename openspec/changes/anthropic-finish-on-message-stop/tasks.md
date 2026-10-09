# Tasks

## 1. Provider-parts conformance harness

- [x] 1.1 In `test/conformance/tools/generate.mts`, wrap the upstream model with a recorder that captures each call's parts (raw chunks forced) and writes `expected-provider-parts.jsonl` for every Anthropic case alongside the existing outputs; run `mise deps` first and verify regeneration is deterministic (twice, no diff) and that every existing `expected.jsonl`, `expected-requests.jsonl` and `expected-usage.json` is unchanged
- [x] 1.2 In `test/conformance/runner.go`, add a Go recording model decorator and compare its normalized per-call parts against the golden for providers that declare the capability (Anthropic only); verify with a unit test that a count/order mismatch fails and that a missing golden fails for an enabled provider
- [x] 1.3 Generate goldens for all Anthropic `upstream/` and `recorded/` cases without modifying any `input*.chunks.txt` (verify with `git diff --stat`); run `cd test/conformance && go test -tags conformance -run TestConformance ./anthropic/` and verify it fails on finish count/raw ordering for `programmatic-tool-calling` and `simple-text`, then triage every other failure per design decision 2 (fix if caused by this change; otherwise classify and either normalize narrowly or add to the allowlist with an `upstream-sync` issue, and note it in `PARITY.md`)
- [x] 1.4 Add `test/conformance/testdata/anthropic-stream-parts/` with labeled synthetic cases (three deltas then stop, delta-less message then stop, consecutive messages, EOF after delta, error between delta and stop with and without stop, first-frame error, overlapping `message_start`), a generator using the pinned upstream package, and a Go test; verify cases fail against the current adapter and expectations regenerate deterministically
- [x] 1.5 Add a provider-independent `test/conformance/ui/error-then-finish/` fixture (input parts: start, error, finish) with an exact-baseline generator; run the Go replay and record whether it already matches upstream `streamText` (kept failing or passing as found)
- [x] 1.6 Document the always-on provider-parts golden, the allowlist and the synthetic-case category in `test/conformance/README.md` and verify `mise run validate-parity-baseline` and `mise run parity-coverage` accept the new files

## 2. Adapter finish lifecycle

- [x] 2.1 Update the two tests pinned in #377 (`TestStreamAdapter_SafeguardResults/last_non-null` expects one finish carrying the verdict; `TestDoStream_RawTransportEvents/normal` expects raw `message_delta`, raw `message_stop`, finish) and add focused cases for multi-delta single finish and malformed verdict at the delta; confirm they fail with `cd providers/anthropic && go test ./...`
- [x] 2.2 In `providers/anthropic/convert_stream.go`, add stream-level finish-reason state (initial `other`, updated by non-null `message_start.stop_reason` and each `message_delta`, keeping the JSON-response-tool mapping), make `message_delta` update state and validate `safeguard_results` without emitting, and emit `PartFinish` from `message_stop`; verify 2.1 passes and the 1.3/1.4 finish-count and ordering failures disappear
- [x] 2.3 Append `message_stop` to remaining test inputs that end after a delta (`convert_stream_test.go`, `stream_error_test.go`, `stream_transport_test.go`, `transport_context_test.go`, `providers/azure/anthropic_test.go`); verify `go test ./...` passes in `providers/anthropic` and `providers/azure`

## 3. Error frames follow upstream

- [x] 3.1 In `stream_transport.go` and `model.go`, make API `error` frames non-terminal (emit the error part, keep reading) while transport and decode failures stay terminal and a first-frame error still fails `DoStream`; verify the synthetic error cases from 1.4 now match upstream and `stream_error_test.go` and `stream_transport_test.go` are updated accordingly
- [x] 3.2 Checkpoint: if the 1.5 core UI fixture mismatches upstream, stop and report before editing `streamtext.go`; if it matches, no core change. When a core fix is approved, land it with the fixture passing and `go test ./...` at the root
- [x] 3.3 Add a `StreamText` test that a stream truncated after `message_delta` takes the partial/no-output path while a complete stream is unchanged; verify `go test ./...` at the root

## 4. Consumers, docs and final checks

- [x] 4.1 Add a Gateway handler test (existing native-fake pattern in `ai-gateway/cmd/grafana-ai-gateway/internal/service/`) where an early null-verdict delta precedes a verdict delta, asserting the forwarded finish carries `safeguardResults` and that a stream truncated after the delta is reported as a premature close; verify with `go test` in `ai-gateway`
- [x] 4.2 In `test/conformance/PARITY.md`, record provider-part coverage for Anthropic finish timing, the per-message verdict/metadata reset deviation from upstream, and that synthetic cases are mock-stream evidence only; verify `mise run validate-parity-baseline` passes
- [x] 4.3 Run `mise run parity-check`, `mise run check` and `mise run lint`; verify clean and that existing UI goldens are unchanged unless the diff is explained by this change

## 5. All providers

- [x] 5.1 Enable the provider-parts golden for Bedrock, OpenAI and OpenAI-compatible (`PROVIDER_PARTS_PROVIDERS`, `providerPartsProviders`), regenerate goldens with unchanged inputs, and verify every existing `expected*.jsonl` is unchanged and regeneration is deterministic
- [x] 5.2 Investigate every failure the new goldens surface and classify each as an adapter deviation, a representation difference, or a harness artifact; verify with a per-provider list of distinct difference signatures covering all mismatching parts, not only the first
- [x] 5.3 Fix the adapter deviations: Bedrock raw envelope and extra `toolCallId`; OpenAI input escaping, field order and finish response ID; OpenAI-compatible duplicate tool-input fields; verify with focused unit tests and `go test ./...` in each provider module
- [x] 5.4 Normalize the harness artifacts narrowly (generated MCP tool-call IDs) and document every normalization; verify with the runner unit tests
- [x] 5.5 Update `README.md` and `PARITY.md` for all providers and verify `mise run validate-parity-baseline` and `mise run parity-coverage`
- [x] 5.6 Verify `mise run check` and `mise run parity-check` are green with the allowlist empty

## 6. Full parity

- [x] 6.1 Measure each normalization by removing it and listing the differences it hid; verify against all four providers
- [x] 6.2 Serialize `stream-start` warnings and millisecond UTC timestamps in `StreamPart.MarshalJSON`; verify with `provider` unit tests
- [x] 6.3 Filter Anthropic raw usage iterations to the declared fields; verify with unit tests and the four affected conformance cases
- [x] 6.4 Classify Anthropic error frames with upstream's table for first-chunk and mid-stream errors and update the pinned error tests; verify with `go test ./...` in `providers/anthropic` and the synthetic error cases
- [x] 6.5 Narrow the harness normalization to the inherent differences, regenerate goldens deterministically, and update README, PARITY.md and specs; verify `mise run check` and `mise run parity-check`

## 7. Stream-level metadata

- [x] 7.1 Make Anthropic provider metadata and the safeguard verdict stream-level and update the pinned unit tests; verify `go test ./...` in `providers/anthropic`
- [x] 7.2 Add synthetic cases for a verdict carried into the next message and a delta-less message repeating metadata, with expectations generated by upstream; verify the conformance suite passes
- [x] 7.3 Replace the reset requirement in the `anthropic-safeguards` delta and drop the deviation from `PARITY.md`; verify `openspec validate --strict` and `mise run validate-parity-baseline`

## 8. Input transformations

- [x] 8.1 Map and validate `input_transformations` for unary and streaming Anthropic provider metadata (message start, message delta, stream-level carry); verify with unit tests that fail without the change
- [x] 8.2 Add synthetic streaming cases and a synthetic unary harness whose expectations come from the pinned upstream; verify they fail without the change and pass with it
- [x] 8.3 Add the `anthropic-input-transformations` spec and document the coverage in `PARITY.md` and the conformance README; verify `openspec validate --strict` and `mise run validate-parity-baseline`

## Workflow follow-up

- Open a draft PR titled `fix(anthropic): emit finish once on message_stop`, linking #377 and noting overlap with #201 and #393.
- Archive the change after review.
