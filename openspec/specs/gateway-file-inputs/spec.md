# gateway-file-inputs Specification

## Purpose

Define strict, bounded Gateway mapping and privacy for ordinary file inputs.

## Requirements

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

### Requirement: Scoped file options preserve semantic JSON
Supported message-level and ordinary file-part provider options, including nested tool-result file options, SHALL preserve namespace objects and opaque nested JSON at their original scope. Reserved host namespaces `gateway`, `grafana`, and `grafana-ai-sdk` SHALL fail safely rather than reach native providers unless an owning host feature consumes them. File-entry and function-tool options SHALL obey consumption-backed protections under gateway-native-provider-options, without selected-backend namespace or field filtering. Existing call-level options, body headers, and text-part options SHALL retain their mapping and host protections. This capability SHALL NOT enable output-level tool-result options or non-file nested result options.

#### Scenario: Scoped opaque values survive
- **WHEN** supported message/file scopes carry ordinary provider options containing nested null, false, zero, empty strings, arrays, or objects
- **THEN** the model SHALL receive semantically identical values at the same scopes, including explicitly empty namespace objects

#### Scenario: File options remain available to native interpretation
- **WHEN** file-entry or function-tool options contain both the selected backend's namespace and an unrelated ordinary namespace or an unlisted ordinary field
- **THEN** all ordinary namespaces and fields SHALL reach the model at their original scopes without Gateway inventory filtering
- **AND** native consumption and concrete bypass protections SHALL determine behavior

#### Scenario: Host namespace is not forwarded
- **WHEN** a supported message or file scope includes a reserved host namespace not consumed by an owning feature
- **THEN** mapping SHALL fail with a fixed safe response before resolution or invocation and SHALL NOT promote it to headers or call options

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

### Requirement: File observation remains private
File-bearing requests SHALL use the existing single logical observation chain with canonical public identity, usage, finish, timing, and safe error state. Gateway metadata-only records, logs, metrics, and errors SHALL omit file payloads, inline text, URLs, references, filenames, provider options, credentials, and private backend identity. Reusable observation SHALL respect selected arms and existing payload-capture controls without fetching URLs or reinterpreting unsupported media as empty binary.

#### Scenario: Hostile markers remain private
- **WHEN** unary or streaming file requests embed distinct private markers in every arm, filename, and option scope and complete, fail, or cancel
- **THEN** logical observation SHALL finalize according to the existing lifecycle and public errors/exported records/logs/metrics SHALL contain none of the markers

### Requirement: Registered client and native conversion evidence
Acceptance SHALL include focused semantic requests emitted by the exact registered TypeScript Gateway client, production Go replay, equivalent independent Go-client requests, authenticated real-handler unary/streaming direct/configured-fallback scenarios and native provider request assertions. Model-boundary tests SHALL cover all supported file arms and presence; native request witnesses SHALL use backend-supported cases and prove candidate-specific scoped option consumption without requiring every backend to accept every arm. Existing comprehensive goldens SHALL remain unmodified except through reviewed client regeneration; deferred siblings SHALL NOT be mistaken for missing ordinary-file coverage. Provider fixture provenance and Apache-to-Gateway dependency isolation SHALL remain enforced. Synthetic native requests SHALL NOT claim live acceptance or full output-derived continuation.

#### Scenario: Cross-client file acceptance
- **WHEN** registered TypeScript and Go clients send equivalent supported file requests through authenticated direct and configured-fallback routes
- **THEN** semantic request bodies and mapped selected arms/filename presence SHALL agree and both clients SHALL consume the existing supported response families

#### Scenario: Evidence is independently reproducible
- **WHEN** file-input validation runs
- **THEN** providerwire checks, native assertions, parity checks and applicable module-boundary checks SHALL pass without relabeling synthetic provider events as recordings or relying on committed workspace replacements
