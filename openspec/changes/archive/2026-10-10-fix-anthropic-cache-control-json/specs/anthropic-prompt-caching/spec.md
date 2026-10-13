## ADDED Requirements

### Requirement: Cache-control helper JSON round trips

Cache-control helpers SHALL serialize their annotation as nested `cacheControl` with a `type` and optional `ttl`. JSON round trips of provider options SHALL preserve that annotation on system messages, messages, content parts, and tools. Typed resolution of the round-tripped helper SHALL recover the same cache type and TTL.

#### Scenario: Helper without TTL crosses a JSON boundary
- **WHEN** a cache-control helper with type `ephemeral` and no TTL is serialized in provider options and decoded
- **THEN** its JSON SHALL contain `{"cacheControl":{"type":"ephemeral"}}` with no `ttl`, and the resulting native request SHALL retain the cache annotation

#### Scenario: Helper with TTL crosses a JSON boundary
- **WHEN** a cache-control helper with type `ephemeral` and TTL `1h` is serialized in provider options and decoded
- **THEN** the resulting native request SHALL retain `cache_control: {"type":"ephemeral","ttl":"1h"}`

#### Scenario: Typed resolution after serialization
- **WHEN** a serialized helper is decoded as raw provider options and resolved as a typed cache-control option
- **THEN** its cache type and optional TTL SHALL equal the original helper's values
