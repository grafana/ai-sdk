## Why

The Go SDK hardcodes `MaxTokens: 4096` for all Anthropic models regardless of their actual capabilities. Modern Claude models support 32k–128k output tokens, so users either get suboptimal defaults or must manually configure every call. The upstream TypeScript SDK has model-aware defaults via `getModelCapabilities`, which the assistant team needs for auto-detecting extended output support and thinking budget integration.

## What Changes

- Add an unexported `getModelCapabilities` function to the `anthropic` package that returns per-model metadata (`maxOutputTokens`, `supportsAdaptiveThinking`, `isKnownModel`) based on model ID string matching
- Replace the hardcoded `MaxTokens: 4096` default with the model's actual `maxOutputTokens` when `MaxOutputTokens` is not explicitly set
- When thinking is enabled but no budget is provided, default to 1024 tokens and emit a compatibility warning (upstream parity)
- When thinking is enabled, add the thinking budget to `max_tokens`
- When `max_tokens` exceeds the model's maximum for known models, clamp to the model max and emit a warning

## Capabilities

### New Capabilities
- `model-capabilities`: Per-model metadata lookup (maxOutputTokens, supportsAdaptiveThinking, isKnownModel) and its integration into request building for defaults, default budget, clamping, and warnings

### Modified Capabilities

## Impact

- **anthropic/models.go**: New `getModelCapabilities` function and `modelCapabilities` struct
- **anthropic/convert_request.go**: `buildParams` updated to use model capabilities for default `MaxTokens`, thinking budget adjustment, and clamping with warnings
- **Wire format**: No change to SSE/stream format; `max_tokens` in the Anthropic API request will change from a fixed 4096 to model-appropriate values
- **Breaking behavior**: Users relying on the implicit 4096 default will now get higher defaults (e.g., 128k for Claude 4.6 models). This is a behavior improvement, not a breaking API change, since the Go function signature is unchanged.
