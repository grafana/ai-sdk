## 1. Remove beta header from reasoning-mapping path

- [x] 1.1 In `providers/anthropic/reasoning.go`, remove the line `p.Betas = appendBetaUnique(p.Betas, "effort-2025-11-24")` inside `applyReasoningConfigWithProviderHints` (currently line 158).

## 2. Remove beta header from provider-options path

- [x] 2.1 In `providers/anthropic/convert_request.go`, remove the line `p.Betas = appendBetaUnique(p.Betas, "effort-2025-11-24")` inside `applyProviderOptions` (currently line 1766).

## 3. Update tests

- [x] 3.1 In `providers/anthropic/convert_request_test.go`, remove the assertion `assert.Contains(t, p.Betas, "effort-2025-11-24")` (currently line 460) and replace with `assert.NotContains(t, p.Betas, "effort-2025-11-24")` so that future regressions are caught.
- [x] 3.2 In `providers/anthropic/reasoning_test.go`, remove the assertion `assert.Contains(t, p.Betas, "effort-2025-11-24")` (currently line 223) and replace with `assert.NotContains(t, p.Betas, "effort-2025-11-24")`.
- [x] 3.3 In `providers/anthropic/reasoning_test.go`, audit the two `for b := range ...` loops (around lines 238 and 260) that filter on `"effort-2025-11-24"`; if those test cases were asserting the beta is present, flip the assertion to absent. If they were already asserting absence (e.g., for the `none` scenario), keep the existing intent but simplify if possible. Simplified by replacing both loops with `assert.NotContains`.

## 4. Verify

- [x] 4.1 Run `make test` (or `cd providers/anthropic && go test ./...`) and confirm all anthropic tests pass.
- [x] 4.2 Run `make check` (fmt + vet + test) and confirm everything is clean.
- [x] 4.3 Grep the repo for `effort-2025-11-24` and confirm no production code references remain (only test assertions verifying absence, and OpenSpec archive history, are acceptable).
