## 1. Model Capabilities Function

- [x] 1.1 Add `modelCapabilities` struct and `GetModelCapabilities` function to `anthropic/models.go` with substring matching for all model families (4.6, 4.5, opus 4.1, sonnet 4.x, opus 4.x, 3-haiku, unknown fallback)
- [x] 1.2 Add unit tests for `GetModelCapabilities` covering every model family branch, date-suffixed variants, and unknown model fallback

## 2. Model-Aware Default Max Tokens

- [x] 2.1 Update `buildParams` in `anthropic/convert_request.go` to call `GetModelCapabilities` and use `maxOutputTokens` as the default `MaxTokens` instead of hardcoded 4096
- [x] 2.2 Add/update tests in `anthropic/convert_request_test.go` verifying default max tokens matches model capabilities when `MaxOutputTokens` is nil, and user-provided value is used when set

## 3. Thinking Budget Adjustment

- [x] 3.1 Update `buildParams` to add thinking budget to `MaxTokens` when thinking type is `enabled` with an explicit `budgetTokens` (after `applyProviderOptions` resolves thinking config)
- [x] 3.2 Add tests verifying thinking budget is added for `enabled` type, and not added for `adaptive` type

## 4. Max Tokens Clamping

- [x] 4.1 Add clamping logic in `buildParams`: for known models, if final `MaxTokens` exceeds `maxOutputTokens`, clamp down; emit a warning only when user explicitly set `MaxOutputTokens`
- [x] 4.2 Add tests for clamping: with/without user-provided max tokens, with thinking budget, for unknown models (no clamping), and within-limits (no clamping)

## 5. Verification

- [x] 5.1 Run `make test` to verify all existing tests pass with the new defaults
- [x] 5.2 Run `make lint` to verify no lint issues
