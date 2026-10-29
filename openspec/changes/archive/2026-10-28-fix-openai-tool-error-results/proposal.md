## Why

OpenAI Responses currently serializes tool failures like successes, losing the error discriminator. Issue #446 requires the registered @ai-sdk/openai 4.0.96 behavior at b7f6255acaae7724adc0fd37aac3dae3f3466578.

## What Changes

- Serialize error-text and error-json as JSON objects containing an error property.
- Preserve function output-schema handling, custom output identity, parallel child ordering, cache precedence and caller immutability.
- Verify exact unary and streaming outbound requests with synthetic transport tests.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `openai-responses-provider`: distinguish failed scalar outputs from successful values for function, custom and parallel results.

## Impact

Private OpenAI conversion helper and regression tests; no public API or dependency changes. Existing multipart and execution-denied behavior remains unchanged. Live multipart/cache recording remains tracked by #440.
