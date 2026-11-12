# Proposal

## Why

When a streamed JSON array starts a negative number but the chunk ends at its minus sign, both partial JSON parsers retain the minus and produce invalid JSON. Tool input snapshots disappear and structured output skips a valid empty-array partial, unlike registered `ai@7.0.159` (issue #649).

## What Changes

- Repair an incomplete first negative array element to an empty array in both parsers; retain the number when digits arrive.
- Cover nested arrays, whitespace, static/dynamic tool input and structured output streaming with regressions and provider-independent upstream evidence.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `structured-output`: partial JSON repair must handle an incomplete initial negative array element.
- `ui-message-stream-reader`: streaming tool inputs retain a valid partial array at the minus boundary.

## Impact

Two private parser statements in `output/fix_json.go` and `ui_message_reader.go`. No public API, dependencies, serialized chunk schema or provider changes. No parser consolidation or unrelated repair behavior changes.
