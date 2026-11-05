## Why

Bedrock rejects duplicate document names within a request. The Go provider sanitizes filenames but does not disambiguate collisions, unlike the registered `@ai-sdk/amazon-bedrock` 5.0.130 baseline. This implements issue #657.

## What Changes

- Allocate document names using request-local generated-name and used-name state shared across user and tool-result documents.
- Match upstream suffix selection and the 200-character limit while preserving names when no collision occurs.
- Add request conversion regressions covering every document path and request isolation.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `bedrock-provider`: document names are unique across the whole converted request.

## Impact

Only Bedrock Converse request conversion, its tests, and its capability spec change. No public API, dependencies, provider response fixtures, frontend wire behavior, or release metadata change.
