## MODIFIED Requirements

### Requirement: Unary function history and selected results
The mapper SHALL support assistant tool calls and tool-role results with text, json, error-text, error-json or text/file content output. It SHALL preserve required empty text, empty content arrays, selected JSON null, and file-data selection and filename presence as defined by gateway-file-inputs. Registered file-content provider options SHALL be supported at the file-entry scope. It SHALL reject inactive fields and leave approvals, execution-denied, provider-executed behavior, custom content, and non-empty output-level or non-file nested result options unsupported.

#### Scenario: Empty selected result
- **WHEN** continuation includes a text result with empty value or a JSON result with null
- **THEN** the selected result arm and value SHALL reach the provider without omission or substitution

#### Scenario: Deferred result family
- **WHEN** a schema-valid result contains an execution-denied or custom-content arm
- **THEN** it SHALL fail as a fixed unsupported capability before resolution or invocation

#### Scenario: File-bearing tool continuation
- **WHEN** supported tool-role history includes ordered text/file result content with empty selected file text/data, an explicit empty filename, and ordinary file-entry provider options
- **THEN** mapping SHALL retain those values and scopes without enabling assistant provider-executed history or output-level result options
