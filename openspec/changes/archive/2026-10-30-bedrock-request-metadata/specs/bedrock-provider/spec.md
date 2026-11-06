## MODIFIED Requirements

### Requirement: Top-level Converse provider option pass-through

The provider SHALL pass additional raw `amazonBedrock` properties to the Converse request top level. Absent/null `amazonBedrock` SHALL fall back to legacy `bedrock`; non-null modern options SHALL take complete precedence without merging. `reasoningConfig`, `additionalModelRequestFields`, `serviceTier`, `structuredOutputMode`, and `requestMetadata` SHALL be excluded from direct pass-through so specialized conversion remains authoritative.

#### Scenario: Guardrail configuration pass-through

- **WHEN** raw `amazonBedrock` provider options include `guardrailConfig` with identifier, version, trace, and stream processing mode fields
- **THEN** the request body includes the unchanged `guardrailConfig` object at the top level

#### Scenario: Legacy namespace pass-through

- **WHEN** `amazonBedrock` is absent and raw `bedrock` provider options include an additional top-level Converse property
- **THEN** the request body includes that property unchanged

#### Scenario: Modern namespace precedence

- **WHEN** both namespaces contain additional top-level Converse properties and `amazonBedrock` is non-null
- **THEN** only properties from `amazonBedrock` are used and legacy properties are not merged

#### Scenario: Null modern namespace falls back to legacy

- **WHEN** `amazonBedrock` is null and `bedrock` contains provider options
- **THEN** the request uses the legacy provider options

#### Scenario: Active tool configuration remains authoritative

- **WHEN** pass-through options contain `toolConfig` and the call has active tools
- **THEN** the generated `toolConfig` for the active tools overrides the pass-through value

## ADDED Requirements

### Requirement: Converse request metadata

The provider SHALL expose optional string-to-string request metadata as `BedrockOptions.RequestMetadata` and accept equivalent raw options. Both Converse APIs SHALL send it at the request top level, with nil omitted and empty maps retained. Selected raw metadata SHALL be validated before HTTP without exposing its contents in errors. Existing namespace precedence and duplicate raw member last-value semantics SHALL be preserved.

#### Scenario: Typed and raw string metadata

- **WHEN** a caller supplies string request metadata using typed or raw options in either supported namespace
- **THEN** both generation and streaming requests include the same top-level requestMetadata object

#### Scenario: Omitted and empty metadata

- **WHEN** request metadata is absent or a typed map is nil
- **THEN** requestMetadata is absent from the request
- **AND** an explicitly empty map is instead serialized as an empty object

#### Scenario: Modern namespace selects the entire option object

- **WHEN** modern options are present with an empty metadata map or no metadata field and legacy options contain metadata
- **THEN** the selected modern options determine metadata presence without merging legacy values
- **AND** absent or null modern options instead permit legacy metadata

#### Scenario: Malformed metadata fails locally

- **WHEN** the selected raw requestMetadata is null, is not an object, or contains a non-string value including null
- **THEN** generation and streaming fail before HTTP
- **AND** the error names requestMetadata without exposing metadata keys or values

#### Scenario: Ordinary JSON member matching

- **WHEN** raw metadata uses a case-insensitive struct-field spelling or duplicate members
- **THEN** decoding follows ordinary encoding/json struct-field matching, and the last duplicate field or record entry wins
- **AND** valid typed metadata uses canonical requestMetadata spelling when emitted

#### Scenario: SDK does not add service limits

- **WHEN** a string record exceeds sixteen entries or contains keys or values longer than 256 characters
- **THEN** the SDK SHALL preserve the record without adding entry-count or string-length validation beyond the registered upstream schema
