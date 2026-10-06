## Why

The Anthropic API supports an `effort` parameter (`low`, `medium`, `high`, `max`) via `output_config.effort` that controls how much reasoning depth the model applies. This is particularly useful with adaptive thinking mode, where the model decides whether to think -- effort lets callers influence that decision. Without it, the model defaults to its own judgment, preventing callers from balancing cost/quality across different operations (e.g., low effort for simple tasks, high effort for complex analysis). Vercel's upstream AI SDK already supports this as a provider option; our Go port does not.

## What Changes

- Add an `Effort` field to `AnthropicOptions` accepting `low`, `medium`, `high`, or `max`
- When set, include `output_config.effort` in the Anthropic API request body
- When set, append the `effort-2025-11-24` beta header to requests
- Effort is independent of thinking mode -- it can be set with or without adaptive/enabled thinking, matching the upstream behavior

## Capabilities

### New Capabilities

- `effort-level`: Provider option for controlling Anthropic reasoning effort level via `output_config.effort`

### Modified Capabilities

None. No existing specs in `openspec/specs/`.

## Impact

- `anthropic/options.go`: New `Effort` field on `AnthropicOptions`
- `anthropic/convert_request.go`: Build `output_config.effort` and add beta header
- `anthropic/convert_request_test.go`: Test coverage for effort passthrough
- No changes to the core `provider/` package -- effort is Anthropic-specific, tunneled through `ProviderOptions`
- No breaking changes
