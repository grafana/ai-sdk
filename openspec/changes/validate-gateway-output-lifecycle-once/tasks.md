# Tasks

## 1. Runtime-path evidence before the removal (commit 1)

- [x] 1.1 Add `providerwire/v4/invalid_output_test.go` with the stream table (every case from `streamAgreementCases`, a hard-coded `rejectAt`, history encoded as a prompt). Each rejected case's body must equal the body of `parts[:rejectAt]` plus a nil-source part; each accepted case must end without a terminal error (D4). Verify with `go test -run TestInvalidOutput ./providerwire/v4/` from `ai-gateway/` on the current code.
- [x] 1.2 Add the unary table to the same file (every case from `unaryAgreementCases`, a hard-coded verdict): 500 internal document with selected-attempt metadata, or 200. Move `TestLifecycleAgreement_StreamFrames` and `TestLifecycleAgreement_UnaryDocuments` there under names without "Agreement". Verify that the same `go test` command passes.
- [x] 1.3 Re-check the `execution` tables, then add only the missing rows listed in D4: a repeated active reasoning start in `stream_validation_test.go`, a provider-executed custom part in `unary_validation_test.go`, and client-call duplicates and mismatched results in `history_test.go`. Verify with `go test ./internal/execution/`.
- [ ] 1.4 Commit `test(ai-gateway): cover invalid output through the ProviderWire handler` after `go test -race ./...` passes in `ai-gateway/`.

## 2. Remove ProviderWire's second evaluation (commit 2)

- [ ] 2.1 In `stream.go`, remove the validator, `phase` and `segmentPhase` from `streamState`, and the `history` parameter from `serveStream`, `runStream` and `newStreamState`. Branch on `part.Type` for the first `stream-start`, and drop the explicit `PartStreamStart` case from `processStreamPart` (D1). Verify with `go build ./...` in `ai-gateway/`.
- [ ] 2.2 In `response.go` and `handler.go`, drop `ValidateGenerateResult`, `unaryMappingContext.history` and the `call.ToolHistory()` read (D2). Verify with `go build ./...`.
- [ ] 2.3 Remove `Call.ToolHistory` and its "valid history is exposed as a copy" subtest. Update the godoc of `ToolHistory` and `NewStreamValidator` if they name an adapter consumer (D3). Verify with `go test ./internal/execution/`.
- [ ] 2.4 Edit the ProviderWire white-box tests listed in the design Context:
  - drop the lifecycle rows from `TestUnarySuccessValidation` and the preliminary half of `TestUnaryProviderToolResult_LargeAndPreliminary`;
  - drop the lifecycle loop of `TestReasoningFrameEncodingAndLifecycle`, renaming the test to match what remains;
  - drop the three `state.phase` assertions.

  Verify with `go test ./providerwire/v4/`.
- [ ] 2.5 Switch `TestRuntimeProviderToolHistory` to `execution.RebuildToolHistory`. Delete `TestUnresolvedProviderCalls_RejectsAmbiguousHistory`, the `unresolvedProviderCalls` wrapper and `lifecycle_agreement_test.go`. Verify that `rg -n 'unresolvedProviderCalls|segmentFinished|lifecycle_agreement|LifecycleAgreement' ai-gateway` returns nothing, and that `go test ./providerwire/v4/` passes with the commit-1 tests unchanged.
- [ ] 2.6 Verify the acceptance boundary: `rg -n 'StreamValidator|ValidateGenerateResult|RebuildToolHistory|ToolHistory' ai-gateway/providerwire --glob '!*_test.go'` returns nothing.
- [ ] 2.7 Update the docs. `internal/execution/doc.go` must stop describing an adapter that evaluates lifecycle rules with its own `StreamValidator`. Check `providerwire/v4/doc.go`, `docs/concepts/gateway-architecture.md` and `docs/concepts/gateway-glossary.md` for drift, and edit them only if they now describe ProviderWire inaccurately. Verify with `go vet ./...` and by reading the godoc.
- [ ] 2.8 Commit `refactor(ai-gateway): validate provider output lifecycle once` after `gofmt -l` is clean and `go test -race ./...` passes in `ai-gateway/`.

## 3. Track checks

- [ ] 3.1 Run `mise run check`, `mise run lint`, `mise run parity-check`, `mise run test-integration`, `mise run test-ai-gateway` and `mise run test-providerwire-v4`, and record the results for the PR description.
- [ ] 3.2 Run `openspec validate validate-gateway-output-lifecycle-once --strict` and verify that it passes.

## Workflow follow-up

- Rebase onto `main` once #609 merges, then re-run group 3.
- Archive the change in the same PR (`docs(openspec): archive validate-gateway-output-lifecycle-once`), syncing the `gateway-execution-runtime` delta.
- Open a draft PR titled `refactor(ai-gateway): validate provider output lifecycle once`, with a review walkthrough per `AGENTS.md`.
