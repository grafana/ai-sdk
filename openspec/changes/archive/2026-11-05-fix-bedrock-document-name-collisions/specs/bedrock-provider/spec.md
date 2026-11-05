## ADDED Requirements

### Requirement: Request-wide unique document names

The Bedrock Converse provider SHALL assign unique document names within each request, sharing naming state across user and tool-result documents in all supported message roles. It SHALL preserve unused bases and append the first unused ` (N)` suffix starting at 2 on collision, trimming and truncating the base to keep the result within 200 characters. Only generated bases SHALL advance the fallback counter. Each request SHALL start with fresh naming state.

#### Scenario: Sanitized names and literal suffixes collide

- **WHEN** document filenames are `invoice.pdf`, `invoice.pdf`, and `invoice (2).pdf`
- **THEN** their names SHALL be `invoice`, `invoice (2)`, and `invoice (2) (2)`
- **AND** different filenames sanitizing to the same base SHALL use the same collision rules

#### Scenario: User and tool-result documents share names

- **WHEN** user and tool-result documents use the same sanitized base across message boundaries
- **THEN** each emitted name SHALL be unique in conversion order, including tool results carried in assistant or user messages

#### Scenario: Generated names collide with explicit names

- **WHEN** a named `document-1.pdf` precedes an unnamed document
- **THEN** the names SHALL be `document-1` and `document-1 (2)`
- **AND** the next unnamed document SHALL use `document-2`

#### Scenario: Suffixes preserve the document name limit

- **WHEN** two sanitized document names contain 200 `a` characters
- **THEN** the first name SHALL contain 200 `a` characters and the second SHALL contain 196 `a` characters followed by ` (2)`

#### Scenario: Independent requests repeat the prompt

- **WHEN** identical prompts are converted in separate requests, including concurrently
- **THEN** each request SHALL produce the same names with no naming state shared between requests
