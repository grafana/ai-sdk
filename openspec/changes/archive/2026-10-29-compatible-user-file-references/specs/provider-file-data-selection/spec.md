## ADDED Requirements

### Requirement: OpenAI-compatible uploaded user-file references

OpenAI-compatible user-file references SHALL resolve using the trimmed first segment of the configured provider name, without normalizing reference keys or using metadata aliases. The selected string, including empty, SHALL be emitted as `file.file_id` regardless of media type or filename. Missing entries SHALL fail before HTTP. Existing metadata overlays and caller inputs SHALL be preserved.

#### Scenario: Exact configured provider entry
- **WHEN** a provider named `custom-provider.chat` receives a reference containing `custom-provider` and `customProvider` entries
- **THEN** both generation and streaming requests SHALL select only `custom-provider`, emit `type: file` with `file.file_id`, and omit generated inline data and filename fields
- **AND** media types including PDF, image, audio, text and unknown media SHALL not change that reference projection

#### Scenario: Empty ID is present
- **WHEN** the selected provider entry contains an empty string
- **THEN** the outbound request SHALL contain `file_id: ""` rather than omit it or reject it

#### Scenario: Missing or invalid reference fails before transport
- **WHEN** a user reference lacks the exact selected provider key, contains invalid reference values, or conflicts with another file-data arm
- **THEN** generation and streaming SHALL return an error without invoking HTTP
- **AND** differently cased, camel-cased, whitespace-altered or full provider-name keys SHALL not be accepted as fallback aliases

#### Scenario: Mixed content and metadata remain intact
- **WHEN** text, uploaded references, inline files and URL files appear together with message and part metadata
- **THEN** outbound ordering and existing metadata overlay precedence SHALL be preserved, and request metadata SHALL match the emitted body
- **AND** conversion SHALL leave the caller's prompt, reference bytes and provider options unchanged

#### Scenario: Reference support evidence stays scoped
- **WHEN** synthetic transport tests verify uploaded user-file conversion
- **THEN** the documented evidence SHALL establish request projection and pre-I/O errors only, without claiming live endpoint acceptance or multipart tool-result support
