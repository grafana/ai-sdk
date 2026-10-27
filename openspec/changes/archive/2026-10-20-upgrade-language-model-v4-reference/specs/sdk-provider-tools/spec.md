# Spec Delta

## ADDED Requirements

### Requirement: Text-encoded partial tool input survives the complete transport path

Tool-input-start SHALL preserve optional JSON or text input encoding through native providers, core events, UI chunks and Gateway. Omitted encoding SHALL retain JSON parsing. Text deltas SHALL concatenate as raw text during partial UI assembly and persistence/resume, while completed tool-call input SHALL remain JSON-encoded. Final available input SHALL clear the partial encoding marker.

#### Scenario: Text input contains quotes and newlines
- **WHEN** a custom tool streams raw text fragments containing quotes and newlines with text encoding
- **THEN** both Gateway clients SHALL retain the encoding and raw deltas
- **AND** Go and pinned frontend UI assembly SHALL expose the same partial string before and after persistence/resume and the same final parsed input

#### Scenario: Invalid declared encoding
- **WHEN** a consumed tool-input-start encoding is neither JSON nor text
- **THEN** Gateway output and the independent client's consumed-field decoding SHALL reject it without changing unrelated variant-field acceptance
