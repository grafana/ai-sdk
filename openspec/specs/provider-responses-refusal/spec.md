# Responses refusal preservation

## Purpose

Retain native refusal separately from ordinary text without changing defaults.

## Requirements

### Requirement: Explicit refusal preservation

The provider SHALL expose PreserveRefusal defaulting to false and SHALL NOT send
that option to the endpoint. When enabled, unary and finish metadata SHALL retain
complete refusal text under the resolved namespace's responses.refusal field.
Ordinary generated text SHALL remain separate from refusal text.

#### Scenario: Structured-output refusal

- **WHEN** the native response returns refusal instead of requested schema output
- **THEN** typed metadata retains that refusal without inventing ordinary text

### Requirement: Consistent refusal streaming

Enabled streams SHALL emit custom refusal deltas. Done and final-response
snapshots SHALL supply missing suffixes without duplicating emitted text.
Conflicting finalized text SHALL emit an error.

#### Scenario: Repeated complete snapshots

- **WHEN** deltas are followed by done and completed snapshots
- **THEN** custom deltas concatenate to the refusal exactly once
- **AND** finish metadata contains the complete refusal

#### Scenario: Completion-only refusal

- **WHEN** only the completed response supplies refusal text
- **THEN** a custom refusal delta precedes finish

### Requirement: Opt-in native response user metadata
The Responses provider SHALL preserve validated native metadata maps on unary
results and stream finish metadata when PreserveResponseMetadata is enabled.
The option SHALL not appear in native requests or change default provider output.

#### Scenario: Actual metadata map differs from the request
- **WHEN** the native response contains an explicit metadata map
- **THEN** ParseResponsesMetadata exposes that map without replacing it from request options

#### Scenario: Invalid native metadata
- **WHEN** metadata contains non-string values or violates native size limits
- **THEN** opt-in decoding fails rather than coercing invalid values
