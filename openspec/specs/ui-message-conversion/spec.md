# UI Message Conversion Specification

## Purpose

Define conversion of persisted UI messages into provider V4 model messages, including lossless provider-reference file handling.

## Requirements

### Requirement: UI file parts represent provider references

The root package SHALL define `FilePart.ProviderReference` as an optional `map[string]string` serialized as `providerReference`, where keys are provider names and values are provider-specific file identifiers. JSON encoding and decoding SHALL preserve both populated references and an explicitly present empty reference object.

#### Scenario: Provider reference JSON round-trip

- **WHEN** a UI file part carries `ProviderReference: map[string]string{"openai": "file-abc123"}`
- **THEN** it SHALL serialize with `"providerReference":{"openai":"file-abc123"}`
- **AND** decoding SHALL restore the same provider reference map

#### Scenario: Empty provider reference remains present

- **WHEN** a UI file part carries a non-nil empty `ProviderReference`
- **THEN** it SHALL serialize with `"providerReference":{}`
- **AND** decoding SHALL restore a non-nil empty map rather than treating the field as absent

### Requirement: UI file conversion preserves provider references

`ConvertToModelMessages` SHALL convert user and assistant UI file parts with a present `ProviderReference` into provider file content whose `DataContent.Reference` carries the canonical provider reference object. A present provider reference SHALL take precedence over the UI file part URL, including when the reference object is empty. Media type, filename, and provider metadata SHALL remain associated with the converted file part.

#### Scenario: User file reference takes precedence over URL

- **WHEN** a user UI file part carries both a URL and an OpenAI provider reference
- **THEN** the converted provider file data SHALL contain the provider reference
- **AND** it SHALL NOT contain URL or inline base64 data

#### Scenario: Assistant file reference is preserved

- **WHEN** an assistant UI file part carries a provider reference
- **THEN** the converted assistant message SHALL contain a provider file part with the same reference object

#### Scenario: Empty reference does not fall back to URL

- **WHEN** a UI file part carries a non-nil empty provider reference and a URL
- **THEN** conversion SHALL preserve the empty reference object
- **AND** it SHALL NOT substitute the URL as provider file data

#### Scenario: Assistant file without reference preserves URL data

- **WHEN** an assistant UI file part has no provider reference
- **THEN** conversion SHALL preserve its URL as provider URL data
- **AND** a data URL SHALL remain URL data rather than being normalized to base64 data

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
