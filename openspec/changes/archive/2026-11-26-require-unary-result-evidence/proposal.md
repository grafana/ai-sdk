## Why

Bedrock unary replay skips a fixture when its result snapshot is missing, so the conformance run can pass without comparing the result. The existing unary snapshot also checks only the legacy metadata namespace, even though both adapters emit both namespaces.

## What Changes

- Require the registered upstream unary result snapshot during Go provider replay and fail on missing, unreadable or mismatched evidence.
- Retain the existing unary snapshot shape and compare the full provider metadata, including both Bedrock namespaces.

## Capabilities

### Modified Capabilities

- `conformance-testing`: enforce upstream-generated unary result evidence in provider replay.

## Impact

The two existing Bedrock unary fixtures and the shared conformance harness. No changes to provider behavior, fixture inputs, the upstream pin, or CI configuration.
