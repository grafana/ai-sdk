## Why

The provider fixture inventory can verify pinned unary JSON, but only Bedrock can replay it through both language-model adapters. Streaming and synthetic tests do not prove that the real unary response and request match across runtimes.

## What Changes

- Enable offline unary replay for Anthropic, OpenAI Responses and OpenAI-compatible Chat Completions.
- Import applicable pinned JSON fixtures without altering their bytes. Compare the single actual HTTP request and the complete unary golden projection from the two adapters.

## Capabilities

### Modified Capabilities

- `conformance-testing`: extend unary replay to the other JSON adapters without changing streaming evidence.

## Impact

Conformance harness and fixture inventory only. No live recorder support, provider fixes, or change to core `GenerateText`.
