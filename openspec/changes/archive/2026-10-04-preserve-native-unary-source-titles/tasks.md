## 1. Confirm the registered reference

- [x] 1.1 Read `test/conformance/upstream.yaml` and `PARITY.md`; confirm the registered OpenAI, Anthropic and provider versions match the ones the issue assessed, and that provider adapters need focused tests rather than new recordings.
- [x] 1.2 Fetch the registered `@ai-sdk/openai@4.0.78` and `@ai-sdk/anthropic@4.0.65` language-model sources; record the title value each annotation and citation kind contributes, and confirm upstream source parts have no text field.

## 2. Establish failing regressions

- [x] 2.1 Add `providers/openai/sources_test.go` with `TestConvertAnnotations_SourceTitles`, covering all four annotation kinds: title, media type, filename, provider metadata, distinct ids and an empty `Text`.
- [x] 2.2 Add `TestSourceTitles_SurviveSimulatedStreaming`, which drives `NewResponses` over a fake transport through `middleware.SimulateStreaming()` and asserts the four titles arrive in order.
- [x] 2.3 Point the existing unary source assertions in `providers/openai/convert_response_test.go` and `providers/anthropic/convert_response_test.go` at `Title`, and assert `Text` is empty.
- [x] 2.4 Run the new tests against the unfixed producers: `git stash push providers/openai/sources.go providers/anthropic/convert_response.go` then `go test ./... -run 'TestConvertAnnotations_SourceTitles|TestSourceTitles_SurviveSimulatedStreaming'` reported `--- FAIL` for both tests and all four subtests.

## 3. Correct the producers

- [x] 3.1 In `providers/openai/sources.go`, set `Title` for the URL citation and set `Title` alongside `Filename` for the file, container-file and file-path citations; update the package comment to name the canonical field.
- [x] 3.2 In `providers/anthropic/convert_response.go`, set `Title` from `src.Title` for text-block citations and from `result.Title` for web-search results.
- [x] 3.3 Leave both streaming adapters, the Grafana provider, the Gateway transport and the middleware projection unchanged.

## 4. Validate

- [x] 4.1 `go test ./...` in `providers/openai` and `providers/anthropic`: ok.
- [x] 4.2 `mise run test-conformance`: PASS. `mise run test`: the only failures are the two `internal/releasecheck` tests, which scan untracked local `dogfood/` modules excluded through `.git/info/exclude` and fail the same way on a clean checkout.
- [x] 4.3 `mise run fmt-check`, `mise run vet` and `mise run lint`: clean.
- [x] 4.4 `openspec validate --all --strict`: this change validates; the two failing items are pre-existing main specs it does not touch.
