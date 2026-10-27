# Spec Delta

## MODIFIED Requirements

### Requirement: Assistant custom history mapping
The service SHALL map each schema-valid assistant `custom` part to provider-domain custom content with its exact `kind` and scoped opaque provider options under gateway-native-provider-options. Custom parts SHALL be accepted only in assistant messages. Kinds SHALL NOT be filtered, renamed or validated against a provider inventory.

#### Scenario: Native custom kinds reach the model
- **WHEN** a unary or streaming request carries assistant `openai.compaction`, `anthropic.fallback` and an unknown `example.kind` custom part with object-valued options
- **THEN** the selected model SHALL receive all three parts in order with unchanged kinds and options, and no `custom-content` refusal SHALL occur

#### Scenario: Custom part outside assistant role
- **WHEN** a request places a custom part in a user or tool message, or uses an undotted kind
- **THEN** it SHALL fail as schema-invalid before resolution or model invocation

#### Scenario: Reserved namespace on a custom part
- **WHEN** a custom part or a tool-result custom entry carries a `gateway` or `grafana` namespace
- **THEN** the model SHALL receive it unchanged with the part, because only call-level `gateway` controls are host-owned
