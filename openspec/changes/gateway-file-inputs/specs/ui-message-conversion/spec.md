## ADDED Requirements

### Requirement: Ordinary UI file inputs preserve filename presence
The root package SHALL represent ordinary UI `FilePart.Filename` as `*string`, where nil is absent and a pointer to an empty string is explicitly present empty. UI JSON round-trip, `ConvertToModelMessages`, provider-domain file content, and Go Gateway request projection SHALL preserve absent, empty, and non-empty filenames for user and assistant file parts. Provider-reference precedence, media type, and provider metadata association SHALL remain unchanged. UI `SourceDocumentPart` and generated/source-only descriptive filenames SHALL retain existing normalization. This requirement SHALL NOT add filenames to generated file SSE chunks or enable generated-media output through the Gateway.

#### Scenario: Filename presence survives the UI input path
- **WHEN** otherwise equivalent user or assistant UI file JSON contains an absent, empty, or non-empty filename
- **THEN** Go decoding and re-encoding SHALL preserve all three states
- **AND** conversion to provider messages and Gateway request projection SHALL preserve the same distinction

#### Scenario: Reference input retains filename presence
- **WHEN** an ordinary UI file has a present provider reference and an explicit empty filename
- **THEN** conversion SHALL retain the reference arm and present empty filename without falling back to URL data or omitting the filename

#### Scenario: Cross-language input conversion proves the distinction
- **WHEN** the deterministic integration scenario validates and converts each filename state with the pinned TypeScript UI API and the Go input path
- **THEN** filename presence SHALL agree in both roles through the resulting model/client request, not merely in an intermediate Go struct
- **AND** any SSE consumed by the scenario SHALL use the registered chunk schema without adding an unregistered filename field to file chunks
