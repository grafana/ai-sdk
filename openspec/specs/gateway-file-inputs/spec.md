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
- **WHEN** a request also activates reasoning files, custom content, approvals, or unsupported provider-executed history
- **THEN** the applicable deferred capability SHALL fail safely before model invocation without enabling that family as a side effect of file support

### Requirement: Scoped file options preserve semantic JSON
Supported message-level and ordinary file-part provider options, including nested tool-result file options, SHALL preserve namespace objects and opaque nested JSON at their original scope. Reserved host namespaces `gateway`, `grafana`, and `grafana-ai-sdk` SHALL fail safely rather than reach native providers. This capability SHALL NOT enable top-level call options, body headers, output-level tool-result options, or unrelated part options.

#### Scenario: Scoped opaque values survive
- **WHEN** supported message/file scopes carry ordinary provider options containing nested null, false, zero, empty strings, arrays, or objects
- **THEN** the model SHALL receive semantically identical values at the same scopes, including explicitly empty namespace objects

#### Scenario: Host namespace is not forwarded
- **WHEN** a supported message or file scope includes a reserved host namespace
- **THEN** mapping SHALL fail with a fixed safe response before resolution or invocation and SHALL NOT promote it to headers or call options

### Requirement: File validation preserves processing order and bounds
Malformed role/data/reference/media-field shapes, typed null, mixed arms, missing required fields, and schema-invalid file members SHALL fail before the execution boundary, resolution, and model invocation. Schema-valid runtime-deferred branches SHALL remain safe unsupported failures. The existing complete-request byte limit SHALL bound file strings and decoding work, including JSON escaping and base64 overhead. Native-provider restrictions SHALL be enforced before external provider I/O rather than guessed by the protocol mapper. The Gateway SHALL NOT retrieve file URLs or broaden the fallback capability subset. Retaining ordinary empty message-option namespaces SHALL NOT narrow existing text fallback eligibility; the gateway-ordered-text-fallback contract SHALL govern this no-op distinction.

#### Scenario: Invalid request has no effects
- **WHEN** a file request has conflicting arms, a forbidden reference member, a missing mediaType, a file directly in a tool-role content array, or a typed null filename
- **THEN** complete validation SHALL fail with zero execution-boundary, resolver, and model calls

#### Scenario: Encoded request byte boundary
- **WHEN** otherwise supported file requests are below, exactly at, and one byte above the configured complete-body limit
- **THEN** only the first two SHALL reach mapping and the oversized body SHALL fail without unbounded buffering or provider invocation

#### Scenario: Existing route restriction remains effective
- **WHEN** newly admitted files, active scoped options, or disallowed tool history target a text-only fallback route
- **THEN** that restriction SHALL remain effective before any physical candidate invocation

### Requirement: File observation remains private
File-bearing requests SHALL use the existing single logical observation chain with canonical public identity, usage, finish, timing, and safe error state. Gateway metadata-only records, logs, metrics, and errors SHALL omit file payloads, inline text, URLs, references, filenames, provider options, credentials, and private backend identity. Reusable observation SHALL respect selected arms and existing payload-capture controls without fetching URLs or reinterpreting unsupported media as empty binary.

#### Scenario: Hostile markers remain private
- **WHEN** unary or streaming file requests embed distinct private markers in every arm, filename, and option scope and complete, fail, or cancel
- **THEN** logical observation SHALL finalize according to the existing lifecycle and public errors/exported records/logs/metrics SHALL contain none of the markers

### Requirement: Registered client and native conversion evidence
Acceptance SHALL include focused semantic goldens emitted by the exact registered TypeScript Gateway client, production Go replay, equivalent independent Go-client requests, authenticated real-handler unary/streaming scenarios, and native provider request assertions. Existing comprehensive goldens SHALL remain unmodified except through reviewed client regeneration; deferred siblings SHALL not be mistaken for missing ordinary-file coverage. Provider fixture provenance and Apache-to-Gateway dependency isolation SHALL remain enforced.

#### Scenario: Cross-client file acceptance
- **WHEN** registered TypeScript and Go clients send equivalent supported file requests through the authenticated service
- **THEN** semantic request bodies and mapped selected arms/filename presence SHALL agree and both clients SHALL consume the existing supported response families

#### Scenario: Evidence is independently reproducible
- **WHEN** file-input validation runs
- **THEN** providerwire checks, native assertions, parity checks, and readonly standalone module checks SHALL pass without relabeling synthetic provider events as recordings or relying on committed workspace replacements
