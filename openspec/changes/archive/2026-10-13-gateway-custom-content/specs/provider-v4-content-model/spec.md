# Spec Delta

## MODIFIED Requirements

### Requirement: Anthropic provider unsupported content warnings

The Anthropic provider's assistant-content conversion SHALL produce a warning and skip `ContentPart` values whose `Type` it does not support natively. Specifically, `ContentPartTypeReasoningFile` parts MUST emit a `Warning{Type: "unsupported"}` and be omitted from the converted assistant content. `ContentPartTypeCustom` parts SHALL follow anthropic-fallback-content and native-custom-content instead.

#### Scenario: Custom-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeCustom}` whose kind is not `anthropic.fallback`
- **THEN** the provider SHALL skip the part without adding a warning

#### Scenario: Reasoning-file-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeReasoningFile}`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the part
