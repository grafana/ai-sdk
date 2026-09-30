## Why

Vertex JSON-schema requests force a synthetic tool even on native-capable models. Sonnet 5.5 rejects the resulting tool choice with HTTP 400.

## What Changes

Enable native Vertex JSON output with model capability gating. Keep direct-only beta injection and strict tool support separate. Document the Google Cloud organization-policy prerequisite.

## Capabilities

### Modified Capabilities

- `anthropic-structured-output`: Native Vertex JSON output replaces automatic tool fallback on supported models.

## Impact

Anthropic request conversion, regression tests, provider documentation and parity evidence. No public API or response mapping changes.
