## 1. Reproduce

- [x] 1.1 Probe a `[good, missing]` step on `main`: `step.ToolResults` is `[missing good]`, the next request lists `good, missing`, and `step.Content` is built from recorded response content.
- [x] 1.2 List the four append sites and confirm the denied path from the issue reproduces as well.

## 2. Failing tests

- [x] 2.1 Add `TestStreamText_ToolResultsFollowCallOrderInMixedSteps` with three cases: a rejected call after an executed one, a denied approval after an executed one, and an executed call between a rejected and a denied one.
- [x] 2.2 Run it on `main`: the three cases fail with `[missing good]`, `[dangerous good]` and `[missing dangerous good]`.

## 3. Fix and record

- [x] 3.1 Add `sortToolResultsByCall` and call it after tool execution, before `buildContent`.
- [x] 3.2 Add the `step-tool-results-call-order` `documented-deviation` entry to `test/conformance/upstream.yaml`.
- [x] 3.3 Extend the call-order requirement and add the mixed-step scenario.

## 4. Validate

- [x] 4.1 `go test -race ./...`: the root module passes; the only failures are the two `internal/releasecheck` tests, which scan untracked local `dogfood/` modules and fail the same way on a clean checkout.
- [x] 4.2 `mise run test-conformance`: PASS. `mise run test-integration`: 64 of 64.
- [x] 4.3 `mise run validate-parity-baseline`: 109 of 109.
- [x] 4.4 `mise run fmt-check`, `mise run vet`, `mise run lint`: clean.
