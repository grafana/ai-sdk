# Proposal

## Why

The documented `CacheControl` helpers lose their annotations when provider options cross a JSON boundary: their exported Go fields serialize without the nested `cacheControl` key the Anthropic provider reads. Issue #350 reports that gateway callers consequently lose prompt caching without an error.

## What Changes

- Serialize `AnthropicCacheControl` using the upstream nested `cacheControl` shape, preserving the cache type and optional TTL.
- Decode that shape when resolving the typed option after a JSON round trip.
- Add regression coverage for the serialized option and native requests built from round-tripped system, message, content-part, and tool annotations.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `anthropic-prompt-caching`: specify serialization and typed resolution of the documented cache-control helpers.

## Impact

Changes are limited to the Apache-2.0 Anthropic provider module and its prompt-caching specification. Existing helper signatures and in-process behavior remain compatible; no dependencies or Gateway code change.

## Non-goals

No new exported cache resolver, cache validation policy, or change to the upstream baseline.
