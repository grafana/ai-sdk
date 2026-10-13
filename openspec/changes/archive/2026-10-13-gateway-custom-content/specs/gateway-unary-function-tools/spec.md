# Spec Delta

## MODIFIED Requirements

### Requirement: Unary result option scopes and deferred families

Registered file-content provider options SHALL be supported at the file-entry scope, and custom tool-result content with its options SHALL be supported under gateway-custom-content. The mapper SHALL reject inactive fields and leave approvals, execution-denied, and non-empty output-level or text-entry nested result options unsupported. Tool-call/result part provider options SHALL follow gateway-provider-tools continuation rules.

#### Scenario: Unary result option scopes and deferred families
- **WHEN** tool-role file content carries file-entry options and output-level options
- **THEN** file-entry options SHALL be supported while non-empty output-level options SHALL remain unsupported before invocation

#### Scenario: Custom result entry is supported
- **WHEN** tool-role content carries a custom entry with object-valued options alongside a text entry without options
- **THEN** both entries SHALL map in order with the custom entry's options unchanged

### Requirement: Unary function history and selected results

The mapper SHALL support assistant tool calls and tool-role results with text, json, error-text, error-json or text/file/custom content output. It SHALL also support provider-executed assistant calls and registered assistant-side basic tool results under gateway-provider-tools. It SHALL preserve required empty text, empty content arrays, selected JSON null, and file-data selection and filename presence as defined by gateway-file-inputs.

#### Scenario: Empty selected result
- **WHEN** continuation includes a text result with empty value or a JSON result with null
- **THEN** the selected result arm and value SHALL reach the provider without omission or substitution

#### Scenario: Deferred result family
- **WHEN** a schema-valid result contains an execution-denied arm
- **THEN** it SHALL fail as a fixed unsupported capability before resolution or invocation

#### Scenario: File-bearing tool continuation
- **WHEN** supported tool-role history includes ordered text/file result content with empty selected file text/data, an explicit empty filename, and ordinary file-entry provider options
- **THEN** mapping SHALL retain those values and scopes while permitting assistant provider-executed history under gateway-provider-tools without enabling output-level result options

#### Scenario: Provider result in assistant history
- **WHEN** continuation contains a provider-executed call and matching basic result inline in assistant content
- **THEN** the mapper SHALL preserve their order and ownership without converting the result into a client-owned tool message
