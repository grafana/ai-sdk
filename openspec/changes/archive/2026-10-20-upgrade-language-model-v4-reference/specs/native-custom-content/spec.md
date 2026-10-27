# Spec Delta

## ADDED Requirements

### Requirement: Anthropic message-start usage is custom output

Each accepted native Anthropic message start SHALL emit a custom part of kind `anthropic.message_start` immediately after response metadata and before prepopulated tool calls. Its Anthropic metadata SHALL carry native ID, model and accumulated raw usage. It SHALL NOT depend on raw-chunk opt-in and SHALL survive supported Gateway custom output transport.

#### Scenario: Native message starts without raw-chunk selection
- **WHEN** a valid message-start event is read without raw-chunk opt-in
- **THEN** response metadata SHALL be followed by the custom message-start part with ID, model and usage
- **AND** a duplicate open message start SHALL NOT emit a second custom part
