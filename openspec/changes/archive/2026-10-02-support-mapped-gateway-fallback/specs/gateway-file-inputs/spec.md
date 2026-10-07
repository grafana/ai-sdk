## MODIFIED Requirements

### Requirement: Ordinary file inputs map without loss
The strict Gateway mapper SHALL support user/assistant ordinary file parts and file entries in supported tool-role content results for unary and streaming requests. It SHALL preserve ordering, selected data/URL/reference/text arms, required empty payloads, media type, and absent/empty/non-empty filename distinctions using explicit private DTO mapping and public provider constructors. The complete registered schema SHALL remain the wire authority, independent of provider-domain JSON methods.

#### Scenario: Four file arms reach the model
- **WHEN** a focused registered-client request contains the four file arms in supported prompt or tool-result positions
- **THEN** the recording model SHALL receive the same selections, payloads, order, media types, and filename presence in both execution modes

#### Scenario: Empty text and data remain selected
- **WHEN** a supported file carries empty data or empty text and an explicitly empty filename
- **THEN** the model SHALL receive the selected empty arm and a present empty filename, not absent data or filename

#### Scenario: Deferred sibling capabilities remain deferred
- **WHEN** a request also activates custom content, approvals, or unsupported provider-executed history
- **THEN** the applicable deferred capability SHALL fail safely before model invocation without enabling that family as a side effect of file support

### Requirement: File validation preserves processing order and bounds
Malformed role/data/reference/media-field shapes, typed null, mixed arms, missing required fields, and schema-invalid file members SHALL fail before the execution boundary, resolution, and model invocation. Schema-valid runtime-deferred branches SHALL remain safe unsupported failures. The existing complete-request byte limit SHALL bound file strings and decoding work, including JSON escaping and base64 overhead. Native-provider restrictions SHALL be enforced before external provider I/O rather than guessed by the protocol mapper. The Gateway SHALL NOT retrieve file URLs. Supported file-bearing requests SHALL remain eligible for configured fallback with the same selected arms, payloads, order, filename presence and scoped ordinary options as direct invocation. Empty or active ordinary options SHALL NOT narrow that eligibility, and no selected-backend inventory or candidate intersection SHALL sanitize requests to make them eligible. Native interpretation, failure eligibility and first-part commitment SHALL remain governed by their existing contracts.

#### Scenario: Invalid request has no effects
- **WHEN** a file request has conflicting arms, a forbidden reference member, a missing mediaType, a file directly in a tool-role content array, or a typed null filename
- **THEN** complete validation SHALL fail with zero execution-boundary, resolver, and model calls

#### Scenario: Encoded request byte boundary
- **WHEN** otherwise supported file requests are below, exactly at, and one byte above the configured complete-body limit
- **THEN** only the first two SHALL reach mapping and the oversized body SHALL fail without unbounded buffering or provider invocation

#### Scenario: Supported file request reaches configured candidates
- **WHEN** a mapped unary or streaming request contains supported ordinary files, active scoped options or supported file-bearing tool history and a primary fails eligibly before commitment
- **THEN** the next configured candidate SHALL receive the same mapped values without text-only refusal, arm substitution or option filtering

#### Scenario: File presence survives eligible failover
- **WHEN** a supported request includes selected empty text/data and filenames that are absent, explicitly empty or nonempty
- **THEN** each attempted model SHALL receive the original selected arms and filename presence, not collapsed absence or substituted bytes

### Requirement: Registered client and native conversion evidence
Acceptance SHALL include focused semantic requests emitted by the exact registered TypeScript Gateway client, production Go replay, equivalent independent Go-client requests, authenticated real-handler unary/streaming direct/configured-fallback scenarios and native provider request assertions. Model-boundary tests SHALL cover all supported file arms and presence; native request witnesses SHALL use backend-supported cases and prove candidate-specific scoped option consumption without requiring every backend to accept every arm. Existing comprehensive goldens SHALL remain unmodified except through reviewed client regeneration; deferred siblings SHALL NOT be mistaken for missing ordinary-file coverage. Provider fixture provenance and Apache-to-Gateway dependency isolation SHALL remain enforced. Synthetic native requests SHALL NOT claim live acceptance or full output-derived continuation.

#### Scenario: Cross-client file acceptance
- **WHEN** registered TypeScript and Go clients send equivalent supported file requests through authenticated direct and configured-fallback routes
- **THEN** semantic request bodies and mapped selected arms/filename presence SHALL agree and both clients SHALL consume the existing supported response families

#### Scenario: Evidence is independently reproducible
- **WHEN** file-input validation runs
- **THEN** providerwire checks, native assertions, parity checks and applicable module-boundary checks SHALL pass without relabeling synthetic provider events as recordings or relying on committed workspace replacements
