## Why

Callers need a compact explanation of which fallback candidate was selected and why preceding candidates failed. The original dormant evidence subsystem duplicated errors and execution state, added diagnostic-specific allocations, and could turn successful generation into an attribution failure.

## What Changes

- Improve reusable Apache SDK fallback capture with request-scoped observation and `Attempt.SourceErr`, without changing fallback policy or lifecycle.
- Replace the dormant AGPL evidence collector, wrappers and allocation machinery with a pure Gateway execution overview projection.
- Represent ordered observed attempts, configured candidate identity, outcomes and protected failure summaries. Remove selected indexes, completion/replay claims, fallback-intent flags, duplicated routing identity, raw diagnostic trees and disposition taxonomies.
- Use standard shallow JSON decoding for native message/type/string-or-number code; retain precise numeric codes without publishing original error bodies, arbitrary causes or request/header data.
- Make metadata enrichment best-effort under the caller's existing complete response/frame limits. If native namespace relocation cannot fit, preserve the original response and leave the native namespace opaque.
- Update the compact namespace schema and focused tests without activating production handlers or restacking #370.

## Capabilities

### New Capabilities

- `fallback-attempt-capture`: Request-scoped observation with candidate-local source errors and independent observer isolation.
- `gateway-execution-overview`: Compact configured-identity projection and best-effort namespace enrichment without runtime activation.

### Modified Capabilities

None. Existing fallback selection and ProviderWire runtime requirements remain unchanged.

## Impact

Shared `fallback` API/tests/guide and AGPL `ai-gateway/internal/execution`, namespace schema and tests. No dependency, LanguageModelV4, minimum Go version or published-module pin changes. This single OpenSpec change owns the entire PR, including the already implemented SDK capture stage. Production delivery and both-client/frontend overview proof remain #370; fuller native transport diagnostics remain #323.
