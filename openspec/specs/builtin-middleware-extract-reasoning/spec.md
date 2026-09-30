## Purpose

Define middleware behavior for extracting tagged reasoning content from generated and streamed text, including chunk boundaries and reasoning-first output.

## Requirements

### Requirement: Extract reasoning from text content
`ExtractReasoning` SHALL accept a tag name and return a `Middleware` that extracts XML-tagged reasoning sections from text output, converting them to reasoning content parts.

Configuration options:
- `TagName` (required): the XML tag name to extract (e.g., `"think"` matches `<think>...</think>`)
- `Separator` (optional, default `"\n"`): separator between multiple reasoning sections
- `StartWithReasoning` (optional, default `false`): whether to assume the output starts inside a reasoning tag

#### Scenario: Basic reasoning extraction in generate
- **WHEN** model output contains `<think>reasoning text</think>actual response`
- **AND** `ExtractReasoning` is configured with `TagName: "think"`
- **THEN** `DoGenerate` result content SHALL contain a reasoning part with text `"reasoning text"` and a text part with text `"actual response"`

#### Scenario: No reasoning tags present in generate
- **WHEN** model output contains no reasoning tags
- **THEN** the text content SHALL pass through unmodified

#### Scenario: Multiple reasoning sections in generate
- **WHEN** model output contains multiple `<think>...</think>` sections
- **THEN** all reasoning text SHALL be extracted and joined with the separator
- **AND** the remaining text sections SHALL be joined with the separator

### Requirement: Extract reasoning from streaming output
`ExtractReasoning` SHALL transform streaming text deltas, emitting reasoning-start/delta/end events for tagged sections and text deltas for non-tagged content.

The stream transform SHALL handle partial tags across chunk boundaries (buffering until a complete tag open/close is confirmed or ruled out). For interleaved text blocks with distinct provider IDs, it SHALL retain each block's own delayed text start until that block emits visible text or ends, SHALL associate every emitted text delta and text end with the correct text start and ID, and SHALL assign a distinct reasoning ID to each reasoning segment across the stream (including segments in different text blocks).

#### Scenario: Streaming reasoning extraction
- **WHEN** streaming text deltas contain `<think>` and `</think>` boundaries
- **THEN** content inside tags SHALL be emitted as `reasoning-start`, `reasoning-delta`, `reasoning-end` events
- **AND** content outside tags SHALL be emitted as `text-delta` events

#### Scenario: Tag split across chunks
- **WHEN** a `<think>` tag is split across two consecutive text delta chunks (e.g., `"<thi"` then `"nk>"`)
- **THEN** the middleware SHALL buffer until the tag is fully received
- **AND** SHALL NOT emit partial tag text as content

#### Scenario: StartWithReasoning enabled
- **WHEN** `StartWithReasoning` is true
- **THEN** the middleware SHALL treat the start of output as inside a reasoning tag (no opening tag needed)
- **AND** SHALL emit reasoning events until the first closing tag is encountered

#### Scenario: Interleaved blocks retain their text lifecycles
- **WHEN** text starts for IDs `a` and `b` precede interleaved deltas and their corresponding ends
- **THEN** each text start SHALL be emitted only before its own first visible text delta or its own text end
- **AND** every emitted text delta and text end SHALL retain its originating text ID with exactly one corresponding text start
- **AND** a reasoning-only block SHALL emit its own delayed text start before its text end without stealing another block's start

#### Scenario: Interleaved nonempty reasoning segments have distinct IDs
- **WHEN** text blocks `a` and `b` contain overlapping nonempty tagged reasoning segments
- **THEN** each such segment SHALL have a matching reasoning-start and reasoning-end with the same ID
- **AND** reasoning segments in different blocks SHALL have distinct IDs, even if their starts and ends interleave
- **AND** visible text and reasoning content SHALL remain associated with their respective blocks when streamed to UI messages

#### Scenario: First empty reasoning segment retains existing behavior
- **WHEN** a block's first reasoning segment is empty
- **THEN** the middleware SHALL emit a reasoning-start and reasoning-end with the same ID for that segment
