## Why

Bedrock requests retain orphaned cache points when conversion omits the source content, producing invalid Converse messages (issue #352).

## What Changes

Prepare effective tools and filter inactive tool history before grouping. Keep each cache point only when its source emits a native block. Preserve supported content and the existing toolContent warning.

## Capabilities

### Modified Capabilities
- `bedrock-provider`: source-owned cache points and early tool-history filtering.

## Impact

Bedrock request conversion, focused unit and synthetic HTTP tests, provider guide and parity documentation. No public API change.
