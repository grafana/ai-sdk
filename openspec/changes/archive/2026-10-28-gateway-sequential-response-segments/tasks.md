# Tasks

Every Go command below runs from `ai-gateway/` with `GOWORK="$PWD/../go.gateway.work"`, which is what `mise run test-ai-gateway` uses. Never modify any `input*.chunks.txt`, `expected-provider-parts.jsonl`, `expected.jsonl` or `expected-requests.jsonl`. Check with `git diff --exit-code -- 'test/conformance/*/upstream/' 'test/conformance/*/recorded/' test/conformance/testdata/` before each commit.

## 2. Spec-driven failing tests

Each task adds or rewrites tests to the new spec. Cases for new behavior must fail on the current code. Guard cases that already pass, such as metadata while a block is open, are kept and labeled as guards. Record which subtests fail in the commit message.

- [x] 2.1 Add `ai-gateway/providerwire/v4/stream_segments_test.go` with `TestStreamingSegments_Lifecycle`. Every case checks the body with `requireStreamBodyMatchesSchema`, the frame order and the exact `"type":"error"` count. The cases are:
  - **Anthropic-shaped:** metadata, custom, text `0`, tool call and a `finish` whose usage, raw usage and `providerMetadata.anthropic.safeguardResults` differ from the second `finish`; then a raw part; then metadata, custom, text `0` and a `finish`; then close.
  - **OpenAI-shaped:** metadata, reasoning, tool input and call; metadata, tool call; metadata, text and one `finish`; then close.
  - **Empty segment:** two consecutive metadata parts, then content and a `finish`.
  - **Anthropic-shaped without raw chunks:** the Anthropic-shaped case with `includeRawChunks` false. The raw `message_start` between the segments is dropped, and the next metadata is still accepted and written.

  Verify that `go test ./providerwire/v4 -run TestStreamingSegments_Lifecycle` fails on the current code.
- [x] 2.2 Add `TestStreamingSegments_Placement`. Metadata arriving while a text block, a reasoning block or a tool input is open, or while a preview series lacks its final result, must give one `internal_error`, no metadata frame and no `finish`. Include an overlapping message start, where the next message's metadata arrives before the open text block's `text-end`.

  Verify that the open-block cases already pass and that every case passes after group 3.
- [x] 2.3 Add `TestStreamingSegments_Identifiers` with these cases:
  - text ID and reasoning ID reused within one segment: fails;
  - the same IDs reused in a later segment: written unchanged;
  - a tool-input or tool-call ID reopened in a later segment: fails;
  - an approval ID reused in a later segment: fails;
  - a provider-executed call in segment 1 with its final result after a `finish` and new metadata: accepted;
  - a history-seeded deferred result in segment 2: accepted.

  Verify that the cross-segment reuse cases fail on the current code.
- [x] 2.4 Add `TestStreamingSegments_AfterFinish` with these cases:
  - text, reasoning, tool, source, custom or approval directly after a `finish`: the body ends with the `finish` followed by exactly one `internal_error`, and the offending payload is absent;
  - a second `finish`: fails the same way;
  - a provider error after a `finish`, then close: the classified error is written and the stream ends at clean EOF with no `internal_error`;
  - an error, then metadata and a segment: continues;
  - a raw part after a `finish`, with `includeRawChunks` true and false: written or dropped, and the next segment is accepted;
  - a recovered-panic error after a `finish`: one `internal_error`.

  Verify that it fails on the current code.
- [x] 2.5 Add `TestStreamingSegments_Completion` with these cases:
  - close after a `finish`: clean;
  - close before any `finish`: one `internal_error` and no synthetic `text-end` or `finish`;
  - close after a `finish` followed by metadata: one `internal_error` after the metadata.

  Add `TestStreamingSegments_StalledAfterFinish` for a provider that sends a `finish` and then never closes until its context is canceled:
  - with idle expiry and with request-deadline expiry, the body ends with the written `finish` followed by exactly one timeout error, the provider observes cancellation before the error is encoded, and the handler returns within the idle duration or the remaining deadline plus 500ms;
  - with request cancellation, the provider observes cancellation, the `finish` stays written and at most one cancellation error follows it, only while the writer is usable;
  - **stall between segments:** an Anthropic-shaped first segment ending in a `finish`, then a stall with further messages pending, gives the `finish` followed by one timeout error and no clean EOF;
  - a `finish` followed by metadata and then idle gives one timeout error.

  Verify that the timeout-after-`finish` cases fail on the current code, which ends at clean EOF right after the first `finish`.
- [x] 2.6 Add `TestStreamingSegments_ExecutionOverview`, following the `TestInvocation_CommittedErrorEnrichment` harness. Every `finish` must carry the same `gateway.execution` overview, and a native `gateway` namespace must be relocated on each `finish`. Verify it fails on the current code, where the second `finish` is absent.
- [x] 2.7 Add `TestStreamingSegments_UpstreamFixtureReplay`.
  - For each of the seven fixtures, read `../../../test/conformance/<provider>/upstream/<name>/expected-provider-parts.jsonl` read-only. Decode each part with `provider.StreamPart.UnmarshalJSON`, and serve the parts through the handler with `"includeRawChunks":true`. The replay model closes its channel right after sending the last committed part, so the stream ends on the provider-close path (clean EOF after the last `finish`) and never waits for the idle timer.
  - Compare the written events with the expected parts one by one. Apply only the documented ProviderWire projections (timestamp encoding, the execution overview and raw credential projection), each named in the test. Name every presence adjustment the comparison needs per case, such as absent versus `false` `isError` or an omitted `providerExecuted: false`, or state in the case that it needs none.
  - Add the labeled synthetic cases `consecutive-messages` and `overlapping-message-start` from `test/conformance/testdata/anthropic-stream-parts/expected-parts.jsonl`. The second must end with its provider error followed by one premature-close `internal_error`.

  Verify that the Anthropic fixtures fail at their first `finish` and the OpenAI fixtures at their second metadata on the current code.
- [x] 2.8 Rewrite the existing tests that encode the old rules, so that they assert the new spec directly:
  - `stream_test.go` `TestStreamingRuntimePartVolumeAndTerminalAuthority/finish is final without waiting for close`: post-finish `text-start` now gives the `finish` followed by one `internal_error`. `post-finish-private` stays absent, and the handler still returns promptly.
  - `stream_test.go` `TestProviderMetadata_Lifecycle/bounded drain after finish`: the `finish` metadata is kept, one `internal_error` follows, `late-private` is absent and latency stays bounded.
  - `stream_test.go` `TestStreamingRuntimeTextStateAndUnsupportedParts`:
    - "duplicate metadata" and "late metadata" move to positive cases ending in a `finish`;
    - an explicit "metadata then close" premature-close case is added;
    - "reused id" stays a within-segment failure.
  - `stream_test.go` `TestStreamCustomContent_Output/response metadata after custom fails`: renamed to "response metadata after custom opens a segment"; asserts that the metadata and the `finish` are written.
  - `sources_runtime_test.go` `TestSourcesStreamingLifecycle`:
    - "late metadata" becomes a positive case with a `finish`;
    - "post finish" now expects failure, and the source stays absent.
  - `stream_tools_test.go` `TestStreamingTools_InvalidTransitions` "late metadata": moves to a positive test with a `finish`. "metadata after input start" stays invalid.
  - `runtime_test.go` `TestProviderMetadata_RuntimeFailureAndFinishAuthority` (L1063-1072): the `finish` and its `"future":{"final":true}` metadata stay written, exactly one `internal_error` follows the `finish`, and `late` stays absent.

  Verify that the rewritten cases fail on the current code. Then run `grep -rn 'post-finish\|late metadata\|after finish\|makeStream(finish\|finishPart(), ' providerwire/v4/*_test.go`, inspect every hit, and check that no other test asserts suppression of a part after a `finish` or a clean end after a stall.

## 3. Segment state implementation (design D1–D7)

- [x] 3.1 In `stream.go`, change `streamState`:
  - replace `metadataSeen` and `textStarted` with a typed segment phase (`segmentOpen`, `segmentFinished`);
  - rename `usedIDs` to `segmentTextIDs` and `usedReasoningIDs` to `segmentReasoningIDs`;
  - add a `quiescent()` method built from the checks now in the `finish` branch;
  - add a segment-start method that clears both per-segment sets.

  Remove the `textStarted` writes in `stream.go`, `reasoning.go` and `stream_tools.go`. Verify with `go build ./...` and `go vet ./providerwire/v4`.
- [x] 3.2 In `processStreamPart`:
  - gate the Finished phase: allow only metadata, raw and provider errors, and send everything else to adapter failure;
  - make metadata require `quiescent()` and start a segment;
  - make `finish` use `quiescent()`, set the Finished phase and return `streamPartContinue`;
  - delete `streamPartFinished`.

  Keep `tools`, `history` and `approvalIDs` stream-global. Verify that the tests in 2.1–2.4, 2.6 and 2.8 pass.
- [x] 3.3 In `consumeStreamParts` and the first-part path of `runStream`, decide the outcome when the wait ends:
  - if the wait ended by provider close and the phase is Finished, return without a frame (clean EOF);
  - otherwise call `cancel()` and keep `writeStreamTerminalForWait` unchanged. Idle timeout, request deadline and cancellation therefore write their normal terminal error in both phases, after any written `finish`.

  Cleanup stays with the deferred owner in `serveStream`. Verify that 2.5 and 2.7 pass, and that `go test ./providerwire/v4/...` passes in full.
- [x] 3.4 Find models in `ai-gateway` that keep their channel open after a `finish`; they now end with a timeout error after the idle duration.
  - **Primary check:** run `go test -v ./...` in `ai-gateway` on the base revision and after group 3, and compare every `--- PASS (Ns)` time. Any test whose time grew by the idle duration, or that now fails with a timeout error after its `finish`, is a candidate.
  - **Secondary audit:** `git grep -n -A8 'PartFinish\|finishPart()' -- 'ai-gateway/**/*.go'` and inspect each model, including the `finishPart()` helper users and non-test models: the testserver (`test/providerwire-v4/testserver/providerwire_v4.go`, `native_values.go`, `citations.go`) and the models behind the plugin e2e path (`mise run test-ai-gateway-e2e`).

  Make each candidate close after its `finish`, unless the test is about non-cooperative providers. Verify that `go test -v ./...` in `ai-gateway` passes with no such timing growth.
- [x] 3.5 Update the `providerwire/v4` godoc (`doc.go` and the `Limits` comment if needed) to say that a stream ends at provider close and that a `finish` ends a response segment. Verify with `go vet ./providerwire/v4` and `mise run lint`.

## 4. Race, resource and service-boundary tests (design D8)

- [x] 4.1 In `stream_segments_test.go`, add the resource cases, following `TestStreamDrainBounds` and `TestStreamingTools_ConcurrentReadyCancellation`:
  - **Continuously ready provider:** it emits valid segments forever until the request deadline. At most one terminal outcome, provider cancellation, and the drain exits within `StreamDrainDuration`.
  - **Non-cooperative provider:** it sends a `finish` and ignores cancellation. The handler returns, and the drain goroutine exits within its duration.
  - **Racing cancellation:** cancellation races the second segment's metadata. One outcome, no second cleanup.
  - **Writer failure** on the second metadata frame: no further write.

  Verify with `go test -race -count=20 -run 'TestStreamingSegments' ./providerwire/v4/`.
- [x] 4.2 In `cmd/grafana-ai-gateway/internal/service`, add a fallback acceptance case to `fallback_acceptance_test.go`: the selected primary streams one valid segment and then fails validation in the second. One `internal_error`, and the secondary is never invoked. Verify with `go test -race ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 4.3 In `model_observability_test.go`, add a two-segment stream through the real observation chain and handler:
  - one log record and one metric set carry the strongest counters across both `finish` parts and the last finish reason;
  - the Agent Observability generation carries the first segment's usage and finish reason, as gateway-text-observability specifies;
  - a normal close records success, not cancellation;
  - a `finish` followed by a stalled provider, in two variants that both expect the `finish` followed by exactly one timeout error on the wire:
    - **deadline:** the request deadline expires during the stall; the logger records outcome `timeout`, and metrics record `status=canceled` with `errorType=context_deadline_exceeded`;
    - **idle:** the idle timeout fires during the stall and cancels the model context; the logger records outcome `cancelled`, and metrics record `status=canceled` with `errorType=context_canceled`, as an idle timeout before any `finish` does today (`middleware/logger/stream.go:163-167`, `middleware/prometheus/errors.go:46-52`).

  Verify with `go test ./cmd/grafana-ai-gateway/internal/service/...`.
- [x] 4.4 Verify with `mise run test-race`.

## 5. Cross-language evidence (design D13)

- [x] 5.1 In `test/providerwire-v4/testserver/recorded_usage.go`, add the seven fixtures to the `registerRecordedUsage` list. Do not edit any fixture `config.yaml`.
  - **Paths:** `root` is the Anthropic `recorded` directory (`PROVIDERWIRE_RECORDED_USAGE_DIR`, set to `test/conformance/anthropic/recorded` by `runtime-integration.test.ts:55`). Follow the existing `openai-reasoning-text` entry (`recorded_usage.go:52`): use `filepath.Join(root, "..", "upstream", "<name>")` for the five Anthropic fixtures and `filepath.Join(root, "..", "..", "openai", "upstream", "<name>")` for the two OpenAI fixtures.
  - **Route names:** `fixture.name` is both the catalog model ID and the `/recorded-usage/<name>/requests` route (`recorded_usage.go:132-133`), so every name must be unique in the list. Prefix each new name with its provider, for example `anthropic-tool-search-bm25`, `openai-reasoning-encrypted-content` and `openai-shell`; a bare `shell` is too generic. Use the same names as the `id` values in task 5.2.
  - **Line length:** the longest line in the seven `input.chunks.txt` files is 2990 bytes (`openai/upstream/reasoning-encrypted-content`), well under the scanner's 1 MiB cap at `recorded_usage.go:82`, so the cap stays unchanged.
  - The five Anthropic upstream configs have no `prompt`, and the config check at `recorded_usage.go:63-64` rejects an empty one, which would fail the whole suite at startup. Apply the conformance runner's default instead: an empty `prompt` becomes `"test"`, as in `test/conformance/runner.go:1179-1182` and `test/conformance/tools/generate.mts:375`, `:381` and `:513`. Keep rejecting an empty `model`.
  - The testserver and `SSEFraming` already use `event: <type>\ndata: …`, so the upstream `.chunks.txt` inputs need no reframing.

  Verify with `go test ./test/providerwire-v4/testserver/...`.
- [x] 5.2 In `test/providerwire-v4/runtime-integration.test.ts`, add the seven fixtures to the recorded replay loop, with each `id` equal to its task 5.1 route name and `path` set to `<provider>/upstream/<name>`. Both the TypeScript `streamText` and `captureGoClient` outputs must `deepEqual` the unchanged `expected.jsonl`, and the native requests must equal `expected-requests.jsonl`. Extend the loop, not the fixture configs, so both clients send what the conformance runner and generator send:
  - **Prompt:** replace `cfg.prompt!` (L774) and the Go client's `text: cfg.prompt` (L779) with `cfg.prompt ?? "test"`, matching `generate.mts` and `runner.go:1179-1182`. The goldens contain `"text":"test"`.
  - **Provider tools for the Go client:** the Go tool list (`definitions`, L778) is built only from `cfg.tools`. Add each `cfg.providerTools` entry as a `provider.Tool` with `type: "provider"`, its `name`, `id` and `args` (`provider/language_model.go:77-111`), exactly as `BuildToolSet` in `test/conformance/runner.go` and `buildTools` in `test/conformance/tools/common.mts` project them. The TypeScript path already uses `buildTools(cfg.tools, cfg.providerTools)`.
  - **Provider options:** keep forwarding per-tool `providerOptions` (for example `anthropic.deferLoading` in the deferred tool-search fixtures) on function tools and top-level `cfg.providerOptions` for both clients, exactly as the runner projects them.

  Verify that `mise run test-ai-gateway-source-integration` fails before group 3 and passes after it.
- [x] 5.3 Add a deterministic synthetic testserver scenario, `sequential-segments`, with an Anthropic-shaped and an OpenAI-shaped variant, and a test for it in `ai-gateway/test/providerwire-v4/runtime-integration.test.ts`. That harness uses `node:test` (`describe`/`it` from `node:test`, `node:assert/strict`), run by `tsx --test` under the `test:client` script of `ai-gateway/test/providerwire-v4/package.json`, not Vitest. The scenario stays in this Gateway harness rather than `test/integration/` (design D13.5). The test must:
  - check every raw SSE frame with `validateStreamEvent` from `./schema`;
  - parse both clients' UI streams with `parseJsonEventStream` from `@ai-sdk/provider-utils` and `uiMessageChunkSchema` from `ai`, both already dependencies of the harness;
  - assert that both clients assemble both segments' text, the repeated text ID and the last `finish` usage identically.

  Verify with `mise run test-providerwire-v4`, which runs `test:client`.

## 6. Docs and the observability follow-up

- [x] 6.1 Update the docs:
  - in `docs/concepts/gateway-architecture.md`, change the stream flowchart edge `frame -->|valid finish| cleanup` (L185) to show that a `finish` continues reading, that provider close after a `finish` that is the last lifecycle part ends cleanly, and that idle, deadline or cancellation keep their terminal error after any written `finish`; adjust the failure paragraph (L198-205) to match;
  - in `docs/concepts/gateway-glossary.md`, add a "Response segment; sequential response; message" row to "Bytes and stream lifecycle";
  - in `docs/providers/grafana-gateway.md`, change "the stream's finish" (L337) to "each stream finish", and add a sentence telling consumers that one stream may carry several `response-metadata` and `finish` events and that EOF follows provider close;
  - in `ai-gateway/docs/text-observability.md`, add the Agent Observability first-segment limitation next to L74.

  Verify with `mise run lint-docs`, and check the architecture diagrams and glossary against the resulting code.
- [x] 6.2 File the follow-up issue for `middleware/agentobservability` (design D10). Its scope:
  - finalize at upstream close or a terminal outcome instead of the first `PartFinish` (`recording.go:239-256`);
  - aggregate usage across all usage-bearing parts;
  - keep the last finish reason and "end before downstream EOF";
  - update `agent-observability-middleware` and remove the gateway-text-observability limitation in the same PR.

  Its release intent is its own module scope. Link it from this change's PR, and replace "separately tracked" in the spec delta with the issue number before archive. Verify that the issue exists and is linked.

## 7. Acceptance and integration checks

- [x] 7.1 Run these and confirm they all pass:
  - `mise run test-ai-gateway`
  - `mise run test-providerwire-v4`
  - `mise run test-ai-gateway-source-integration`
  - `mise run test-ai-gateway-command`
  - `mise run test-ai-gateway-e2e`
  - `mise run verify-ai-gateway-boundary`
  - `mise run verify-sdk-gateway-isolation`
  - `mise run test-race`
  - `mise run test`
  - `mise run lint`
  - `mise run lint-docs`
  - `mise run validate-parity-baseline`
  - `mise run parity-check`
- [x] 7.2 Run `git diff --stat origin/main -- 'test/conformance/*/upstream/' 'test/conformance/*/recorded/' test/conformance/testdata/ test/conformance/upstream.yaml test/conformance/PARITY.md`. It must show no changes. Then run `openspec validate gateway-sequential-response-segments --strict`, which must pass.

## Workflow follow-up

- Open a draft PR titled `feat(ai-gateway)!: forward sequential response segments in one stream`. Include:
  - the review walkthrough from `AGENTS.md`;
  - the statement that the evidence is mock-stream or replay evidence, not live-provider acceptance.
- Link the Agent Observability follow-up #582 from the PR description.
- Before pushing, check the staged diff and the outgoing commits for secrets.
- Archive the change in the same PR before merge. If the Agent Observability follow-up merged first, drop the limitation clauses from the gateway-text-observability delta before archiving.
