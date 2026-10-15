## Why

Compaction cache usage is omitted from top-level Anthropic usage. The adapter already selects executor iterations for input and output, but previously retained partial top-level cache counts, undercounting normalized input usage (#351).

## What Changes

- Sum cache reads and writes from the same executor iterations as input and output.
- Preserve fallback and no-executor top-level accounting; exclude advisor usage and avoid double counting.
- Document the deliberate upstream deviation and synthetic HTTP evidence boundary.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `anthropic-prompt-caching`: consistent generate and streaming executor cache accounting.

## Impact

Anthropic usage conversion, HTTP regression tests, parity metadata and cache usage specifications. No public API, version pins or provider recordings change. This archive records the completed implementation during delivery review; it does not claim a proposal was created before the original implementation.
