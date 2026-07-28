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

The stream transform SHALL handle partial tags across chunk boundaries (buffering until a complete tag open/close is confirmed or ruled out).

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
