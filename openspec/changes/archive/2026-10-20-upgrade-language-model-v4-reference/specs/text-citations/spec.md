# Spec Delta

## Purpose

Preserve references supporting generated text independently of complete retrieval source lists across native models, orchestration, UI persistence and Gateway transport.

## ADDED Requirements

### Requirement: Text citations retain their attachment and wire shape

Generated text and text-end events SHALL preserve ordered citations with a URL or document source, optional nonnegative offsets and optional cited text. Core text content, UI chunks and persisted text parts SHALL retain the attachment. Missing citations SHALL remain absent; an explicit empty array SHALL remain empty.

#### Scenario: Text completes with document and URL citations
- **WHEN** a native text-end carries URL offsets and a document citation with empty required display fields and empty cited text
- **THEN** schema-parsed frontend assembly and Go assembly SHALL retain both citations on that text part
- **AND** persistence and resume SHALL preserve their source metadata and optional offsets

### Requirement: Retrieval sources are independent of text citations

Anthropic and OpenAI adapters SHALL emit complete available retrieval sources rather than duplicate citation-derived URL/file sources. They SHALL retain citations on text independently. Streaming fallback sources SHALL wait until message completion for Anthropic and stream flush for OpenAI; OpenAI retrieved files SHALL deduplicate by file ID.

#### Scenario: Retrieval data is present
- **WHEN** a search returns retrieved sources and text cites a subset
- **THEN** all retrieved sources SHALL remain source parts and citations SHALL remain attached to text without duplicate URL/file annotation source parts

#### Scenario: Retrieval data is unavailable
- **WHEN** URL or file annotations arrive without corresponding retrieval results
- **THEN** annotation-derived sources SHALL be emitted at the fallback boundary in upstream order
- **AND** container-file and file-path source behavior SHALL remain independent

### Requirement: Gateway citation projection uses shared output bounds

Gateway unary text and text-end events and the independent Go client SHALL preserve citations. Gateway projection SHALL reject unrepresentable sources, invalid UTF-8, invalid offsets and aggregate citation content exceeding existing response or frame bounds without truncation. This SHALL NOT add a new citation-specific resource limit or operator capture field.

#### Scenario: Both clients consume bounded citations
- **WHEN** the authenticated handler returns valid citations in unary and streaming modes
- **THEN** the pinned TypeScript client and independent Go client SHALL receive equivalent citations and existing strict output schemas SHALL accept every event

#### Scenario: Citation output cannot be represented
- **WHEN** citation offsets are negative or unsafe integers, source fields are invalid, or combined citation content exceeds the output budget
- **THEN** the handler SHALL fail its existing output validation rather than omit citations or return partial citation JSON
