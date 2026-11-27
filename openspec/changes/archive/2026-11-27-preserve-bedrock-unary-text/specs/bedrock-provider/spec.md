## ADDED Requirements

### Requirement: Non-streaming Converse text projection

For each Converse response block in wire order, `DoGenerate` SHALL emit one text part for non-null direct `text`, including an empty string. If direct text is absent or null, it SHALL emit one part per non-null `citationsContent.content[].text` in fragment order, retaining empty strings. This projection SHALL NOT emit citation metadata or sources.

#### Scenario: Text is present alongside citation fragments

- **WHEN** a response block contains direct text and citation text fragments
- **THEN** only the direct text is emitted from those fields, even if it is empty

#### Scenario: Citation fragments supply the answer

- **WHEN** direct text is absent or null and the citation content contains text fragments
- **THEN** each non-null text fragment produces its own part in order, and fragments with missing or null text produce no part

#### Scenario: Present empty text separates reasoning

- **WHEN** an empty text block occurs between two signed reasoning blocks
- **THEN** the text part stays between the reasoning parts and their signatures are preserved

#### Scenario: JSON instruction text is split across blocks

- **WHEN** the JSON instruction route receives an object split across direct text and citation text fragments
- **THEN** the text parts share a single object extractor for the response and retain their order
