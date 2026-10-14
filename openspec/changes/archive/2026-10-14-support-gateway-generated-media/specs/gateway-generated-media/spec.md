## Purpose

Deliver generated LanguageModel files through Grafana Gateway while preserving the registered provider contract and bounded response ownership.

## ADDED Requirements

### Requirement: Generated file output preserves the registered contract
Unary and streaming Gateway responses SHALL preserve generated file order, media type, data or URL arm and opaque provider metadata. Inline bytes SHALL become standard base64, existing base64 strings SHALL remain unchanged, and URL strings and optional originalUrl SHALL retain their values and presence. Normalized output SHALL omit provider-local generated filenames and unregistered fields.

#### Scenario: Inline bytes and selected empty data
- **WHEN** a provider emits generated files containing bytes, base64, or an explicitly selected empty data payload
- **THEN** both Gateway clients SHALL receive equivalent data-arm values and media types in unary and streaming responses
- **AND** bytes SHALL be encoded once and selected empty data SHALL remain present

#### Scenario: URL and original URL values
- **WHEN** a generated file selects a URL with an absent, empty, or opaque originalUrl value
- **THEN** both clients SHALL preserve the selected URL and originalUrl presence/value without downloading or rewriting them at the provider transport boundary

#### Scenario: Ordinary metadata is independent of capture
- **WHEN** file metadata contains nested opaque values or secret-looking application text and server capture is disabled
- **THEN** registered response fields SHALL remain intact independently of consumer capture configuration
- **AND** absent metadata and an explicitly empty metadata object SHALL remain distinct

#### Scenario: Provider-local filename is omitted
- **WHEN** a valid generated file includes a provider-local filename, including an oversized or invalid UTF-8 filename
- **THEN** the normalized output SHALL omit filename and acceptance SHALL depend only on emitted registered fields

### Requirement: Invalid or oversized media fails atomically
Generated-file validation SHALL reject missing, conflicting or unsupported data arms and malformed consumed fields. The service SHALL enforce aggregate unary-response and complete-SSE-frame bounds before successful write, accounting for base64 and JSON escaping expansion. Rejection SHALL use existing bounded error behavior and SHALL NOT emit a partial file or successful finish.

#### Scenario: Oversized unary file
- **WHEN** media bytes or encoded metadata exceed the unary response budget
- **THEN** no successful response header or partial media body SHALL be written

#### Scenario: Oversized stream frame
- **WHEN** a selected generated file cannot fit its encoded SSE frame budget
- **THEN** the service SHALL emit only the existing bounded terminal error behavior, cancel the producer and close owned resources
- **AND** it SHALL NOT replay the selected generation through another candidate

#### Scenario: Escaping and base64 limits
- **WHEN** raw strings fit but JSON escaping or byte-to-base64 expansion exceeds the encoded budget
- **THEN** the file SHALL fail before its body or frame is written

#### Scenario: Unsupported output arm
- **WHEN** a generated file supplies reference, text, missing data or conflicting sources
- **THEN** the service SHALL fail through its adapter error path without projecting an invented data/URL value

### Requirement: File delivery retains streaming and fallback ownership
A generated file SHALL be valid without a text-start event and SHALL retain its relative content order. Its selected provider SHALL remain committed under reusable fallback semantics. Cancellation, downstream backpressure and terminal errors SHALL preserve bounded stream cleanup without introducing another reader or post-output replay.

#### Scenario: File then provider failure
- **WHEN** a fallback candidate emits a file and later fails
- **THEN** the client SHALL observe the file and terminal failure in order, and no later candidate SHALL run

#### Scenario: Eligible failure before selection
- **WHEN** a candidate fails eligibly before the reusable fallback commitment point and the next candidate emits a file
- **THEN** the existing fallback SHALL select and deliver that file without a text-only request restriction

### Requirement: Generated file parity has independent evidence
Generated-file acceptance SHALL include registered TypeScript and Go client HTTP consumption, strict raw response/schema assertions, frontend file assembly and independently configured server/consumer capture. Synthetic transport evidence SHALL be labeled accurately and SHALL NOT be represented as a real provider recording or private Vercel service proof.

#### Scenario: Both clients consume the same server outputs
- **WHEN** the actual Gateway handler emits data-arm and URL-arm files in unary and stream mode
- **THEN** registered TypeScript and independent Go clients SHALL preserve equivalent registered values
- **AND** raw bodies and frames SHALL validate against strict production schemas

#### Scenario: Frontend assembly and capture
- **WHEN** generated files pass through the Apache Grafana provider and core UI streaming with independently configured consumer capture
- **THEN** the existing frontend suite extended for the Grafana path SHALL assemble schema-parsed file chunks; separate HTTP/capture tests SHALL prove server capture choices do not alter caller results
