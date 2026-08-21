## MODIFIED Requirements

### Requirement: File and custom parts pass through

`ToResponseMessages` SHALL convert each collected `ContentPartTypeFile` response entry into an assistant request `file` `ContentPart` preserving `Data`, `MediaType`, and `ProviderOptions`. The output request part SHALL write filename presence to `FilePartFilename` and SHALL clear `Filename`:

- when the input already has non-nil `FilePartFilename`, the output SHALL copy that pointer value;
- otherwise, when the generated response input has non-empty `Filename`, the output SHALL set `FilePartFilename` to a copy of that string;
- otherwise, `FilePartFilename` SHALL remain nil.

An input with both filename fields populated SHALL be invalid mixed state in focused tests and SHALL NOT be produced by orchestration. `ToResponseMessages` remains error-free, so it SHALL deterministically prefer the request-specific non-nil `FilePartFilename` only as a defensive fallback for manually constructed input; request boundaries still reject mixed state.

The function SHALL convert each `ContentPartTypeCustom` entry to an assistant `custom` `ContentPart` preserving `Kind` and `ProviderOptions`.

#### Scenario: File part is appended to the assistant message

- **WHEN** the input contains a generated `file` part
- **THEN** the assistant request message SHALL contain a `file` part preserving data, media type, and provider options

#### Scenario: Generated file filename moves to the request field

- **WHEN** the input contains a generated `file` part with `Data`, `MediaType`, and `Filename == "report.pdf"`
- **THEN** the assistant request message SHALL contain a `file` part with equivalent data and media type
- **AND** `FilePartFilename` SHALL point to `"report.pdf"`
- **AND** `Filename` SHALL be empty

#### Scenario: Existing request filename presence is copied

- **WHEN** the input file part has `FilePartFilename` pointing to `""` and empty `Filename`
- **THEN** the assistant request part SHALL retain a non-nil pointer to `""`
- **AND** `Filename` SHALL remain empty

#### Scenario: Absent generated filename remains absent

- **WHEN** the input file part has both filename fields absent or empty
- **THEN** the assistant request part SHALL have nil `FilePartFilename` and empty `Filename`

#### Scenario: Mixed manually constructed input has deterministic fallback

- **WHEN** a manually constructed input file has both filename fields populated
- **THEN** focused tests SHALL identify it as invalid producer state
- **AND** the error-free helper SHALL copy `FilePartFilename` and clear `Filename` rather than reading the response field

#### Scenario: Custom part is appended to the assistant message

- **WHEN** the input contains a `custom` part with `Kind == "openai.compaction"` and `ProviderOptions` set
- **THEN** the assistant message SHALL contain a `custom` `ContentPart` with both fields preserved
