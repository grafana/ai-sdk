## Why

The `effort` parameter for Anthropic extended thinking went GA on 2025-11-24. The Anthropic SDK published it under a beta flag (`effort-2025-11-24`) during the preview period, but the flag was retired when the feature shipped. Direct Anthropic silently ignores the stale header, but Vertex AI's strict validator now actively rejects requests carrying it with HTTP 400:

```
Unexpected value(s) `effort-2025-11-24` for the `anthropic-beta` header.
```

Upstream removed the beta append in `vercel/ai@e5c4f40` (anthropic canary.46). The Go port must follow to restore Vertex AI compatibility.

## What Changes

- Stop adding the `effort-2025-11-24` beta header when the `output_config.effort` field is set, both for the provider-options path (`AnthropicOptions.Effort`) and the reasoning-mapping path (`CallOptions.Reasoning` on adaptive-capable models).
- The `output_config.effort` request body field continues to drive the feature end-to-end on direct Anthropic, Bedrock, and Vertex.
- **BREAKING (wire-level)** for Vertex AI users who were previously seeing HTTP 400 rejections — those requests now succeed. No caller-visible API change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `effort-level`: Remove the requirement that mandates appending the `effort-2025-11-24` beta header when effort is set (both the provider-option path and the reasoning-mapping adaptive path).

## Impact

- `providers/anthropic/reasoning.go` — remove the `appendBetaUnique(..., "effort-2025-11-24")` call in `applyReasoningConfigWithProviderHints`.
- `providers/anthropic/convert_request.go` — remove the same beta append in `applyProviderOptions` (provider-options path).
- `providers/anthropic/reasoning_test.go` and `providers/anthropic/convert_request_test.go` — remove assertions that verify the beta is present; add (or update) assertions that it is absent.
- No public Go API surface changes.
- Restores Vertex AI compatibility for any request that sets `effort` or uses reasoning mapping with adaptive thinking.
