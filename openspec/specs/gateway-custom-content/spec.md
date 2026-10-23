# gateway-custom-content Specification

## Purpose
Define bounded transport of registered LanguageModelV4 custom content through ProviderWire and both Gateway clients, preserving provider-owned kinds, options and metadata without kind inventories or telemetry capture.

## Requirements

### Requirement: Assistant custom history mapping
The service SHALL map each schema-valid assistant `custom` part to provider-domain custom content with its exact `kind` and scoped opaque provider options under gateway-native-provider-options. Custom parts SHALL be accepted only in assistant messages. Kinds SHALL NOT be filtered, renamed or validated against a provider inventory.

#### Scenario: Native custom kinds reach the model
- **WHEN** a unary or streaming request carries assistant `openai.compaction`, `anthropic.fallback` and an unknown `example.kind` custom part with object-valued options
- **THEN** the selected model SHALL receive all three parts in order with unchanged kinds and options, and no `custom-content` refusal SHALL occur

#### Scenario: Custom part outside assistant role
- **WHEN** a request places a custom part in a user or tool message, or uses an undotted kind
- **THEN** it SHALL fail as schema-invalid before resolution or model invocation

#### Scenario: Reserved namespace on a custom part
- **WHEN** a custom part carries a reserved Gateway namespace
- **THEN** the existing reserved-provider-options refusal SHALL apply before resolution

### Requirement: Custom tool-result content mapping
The service SHALL map tool-result `content` output entries of type `custom` to provider-domain custom tool-result content carrying their scoped opaque provider options. Mixed text, file and custom entries SHALL keep their order. A custom entry without options SHALL remain a valid empty custom entry.

#### Scenario: Anthropic tool reference reaches the adapter
- **WHEN** a tool result's content contains a custom entry with `anthropic: {type: "tool-reference", toolName: "search"}` between text and file entries
- **THEN** the model SHALL receive the three entries in order with the custom entry's options unchanged

#### Scenario: Empty custom entry
- **WHEN** a custom tool-result entry has no provider options
- **THEN** it SHALL map to a custom entry with no options rather than a refusal

### Requirement: Unary custom content output
Unary success SHALL encode provider custom content as `{type: "custom", kind, providerMetadata?}` in original content order under explicit private DTOs. The kind SHALL be non-empty valid UTF-8 containing a dot. Metadata SHALL follow gateway-provider-metadata presence and validity. Invalid custom output SHALL fail before HTTP 200.

#### Scenario: Custom-only paid response
- **WHEN** a provider returns only `anthropic.fallback` custom content with metadata
- **THEN** both registered clients SHALL consume success with the same kind and metadata, and default high-level retries SHALL NOT invoke the provider again

#### Scenario: Invalid custom output
- **WHEN** a provider returns custom content with an empty, undotted or invalid-UTF-8 kind, or invalid metadata
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Large custom metadata
- **WHEN** valid custom content carries metadata larger than the former unary budget
- **THEN** the handler SHALL return success with the kind and metadata unchanged

### Requirement: Streamed custom content events
The stream SHALL encode each provider custom part as one atomic `{type: "custom", kind, providerMetadata?}` event in provider order. Custom events SHALL be valid between stream start and finish, SHALL count as started content so later response metadata is rejected, and SHALL NOT open, close or alter text, reasoning or tool state.

#### Scenario: Custom between active blocks
- **WHEN** a custom part arrives while a text block and a tool-input block are open
- **THEN** it SHALL be emitted in order and both blocks SHALL remain open and closable by their existing rules

#### Scenario: Response metadata after custom
- **WHEN** response metadata arrives after a custom event
- **THEN** the handler SHALL emit at most one safe terminal error and no synthetic finish

#### Scenario: Invalid custom stream part
- **WHEN** a custom part has an invalid kind or invalid metadata
- **THEN** the handler SHALL emit at most one safe terminal error without serializing the provider-domain part

#### Scenario: Large custom event
- **WHEN** a valid custom part encodes to a frame larger than the former 1 MiB limit
- **THEN** the complete custom event SHALL be written and the stream SHALL continue

### Requirement: Generic custom kinds without inventory
The service and both clients SHALL transport any well-formed custom kind, including kinds introduced by later native adapters, without an allowlist, a per-kind codec or silent filtering.

#### Scenario: Unknown future kind
- **WHEN** a provider emits unary and streamed custom content with kind `example.future_block` and nested opaque metadata
- **THEN** raw server output and both clients SHALL retain the kind and metadata unchanged

#### Scenario: Custom event right after response metadata
- **WHEN** a synthetic custom event shaped like a later native `anthropic.message_start` follows response metadata and precedes text
- **THEN** the stream SHALL emit it unchanged and continue with the text and finish, without claiming native emission at the registered baseline

### Requirement: Custom content fallback commitment
Custom content SHALL follow existing fallback commitment. A unary success containing custom content and a first streamed custom part SHALL each select their candidate, and later adaptation failures SHALL NOT replay on another candidate.

#### Scenario: First streamed part is custom
- **WHEN** a configured-fallback candidate emits a custom part first and then fails
- **THEN** that candidate SHALL remain selected and no later candidate SHALL run

### Requirement: Custom content telemetry separation
Custom provider options and metadata SHALL remain caller-visible independently from operator capture. Metadata-only operator telemetry SHALL exclude custom options and metadata. Known credential protections SHALL continue to apply.

#### Scenario: Metadata-only capture
- **WHEN** a custom part carrying compaction encrypted content passes through the Gateway under metadata-only operator capture
- **THEN** the caller SHALL receive the encrypted content while operator logs and Agent Observability records SHALL exclude it

### Requirement: Custom content two-client evidence
The registered Vercel client and the independent Go client SHALL be compared on unary and streaming custom output. Native request fakes SHALL verify custom history and tool-result content on direct and configured-fallback routes. Evidence SHALL NOT invent provider recordings.

#### Scenario: Two-client equivalence
- **WHEN** the same unary and streaming custom outputs are consumed by both clients
- **THEN** their normalized kinds, metadata, order and finish SHALL agree, and synthetic native evidence SHALL be labeled as such
