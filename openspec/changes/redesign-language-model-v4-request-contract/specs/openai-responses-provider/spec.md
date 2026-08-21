## ADDED Requirements

### Requirement: Custom-tool continuation file filenames preserve optional presence

When OpenAI Responses converts `ToolResultOutput` content into a custom-tool continuation request, non-image inline file data SHALL map `ToolResultContentValue.Filename *string` with the exact pinned nullish-default behavior:

- nil filename SHALL use the fallback filename `"data"`;
- a non-nil pointer to `""` SHALL emit an explicitly empty filename and SHALL NOT use the fallback;
- a non-nil pointer to a non-empty string SHALL emit that string exactly.

The conversion SHALL test the final serialized Responses request rather than only an intermediate SDK value.

#### Scenario: Absent custom-tool file filename defaults
- **WHEN** custom-tool output contains a non-image inline file with nil `Filename`
- **THEN** the final request `input_file.filename` SHALL be `"data"`

#### Scenario: Explicit empty custom-tool file filename is preserved
- **WHEN** custom-tool output contains a non-image inline file with `Filename` pointing to `""`
- **THEN** the final request `input_file.filename` SHALL be `""`
- **AND** it SHALL NOT be replaced by `"data"`

#### Scenario: Non-empty custom-tool file filename is preserved
- **WHEN** custom-tool output contains a non-image inline file with `Filename` pointing to `"report.pdf"`
- **THEN** the final request `input_file.filename` SHALL be `"report.pdf"`
