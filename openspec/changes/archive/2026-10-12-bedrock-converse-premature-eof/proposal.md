## Why

Issue #349 reports partial Converse output being accepted as complete when the body ends before messageStop. Frame-boundary EOF must be distinguishable from a valid terminal event.

## What Changes

Reject clean EOF before messageStop with one retryable provider error and no provider finish. Preserve partial output and never synthesize unfinished tool calls. Keep genuine stop events valid with optional metadata.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: Require messageStop for successful clean EOF.

## Impact

Bedrock stream conversion, synthetic transport tests, frontend error scenarios and registered deviation metadata. No package pins or provider recordings change. Broader event-sequence validation and retries after partial output are non-goals.
