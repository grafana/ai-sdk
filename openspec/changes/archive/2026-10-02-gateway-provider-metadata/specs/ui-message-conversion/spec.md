## ADDED Requirements

### Requirement: Supported continuation metadata presence survives UI conversion
For the affected currently supported text/reasoning, tool-call/basic-result, reasoning-file and source paths, root UI chunk serialization, assembled UI parts and UI message JSON round-trip SHALL preserve represented providerMetadata, callProviderMetadata and resultProviderMetadata presence independently: nil SHALL omit and a non-nil empty object SHALL serialize as an empty object. Namespace objects and nested JSON SHALL remain opaque. Latest non-nullish object assembly SHALL replace rather than recursively merge. ConvertToModelMessages SHALL retain represented continuation metadata as scoped providerOptions on applicable content; source metadata SHALL remain response-only under existing conversion behavior. Changes SHALL be limited to demonstrated metadata serialization/assembly/conversion gaps and SHALL NOT add metadata to pinned UI chunk variants that do not represent it.

#### Scenario: Empty text metadata clears frontend state
- **WHEN** schema-parsed text chunks first carry metadata and a later registered chunk supplies an explicit empty metadata object
- **THEN** the pinned frontend and Go UI reader SHALL assemble empty replacement metadata instead of retaining the prior object
- **AND** affected persisted UI JSON and applicable model-message conversion SHALL preserve that explicit empty state

#### Scenario: Call and result metadata remain separate
- **WHEN** supported tool input/call/output UI chunks carry distinct call and result metadata, including an explicit empty replacement
- **THEN** frontend/Go assembly, UI JSON and applicable model-message conversion SHALL retain the distinct scopes without dropping emptiness or merging objects

#### Scenario: Cross-language proof precedes the core fix
- **WHEN** a demonstrated metadata presence/assembly gap changes frontend-visible wire behavior
- **THEN** a deterministic Go scenario and matching Vitest test SHALL consume SSE through parseJsonEventStream and uiMessageChunkSchema, assert chunk fields and assembled messages, and compare applicable converted model history
