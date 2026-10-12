# Provider File Data Selection Specification

## Purpose

Define selected file-data variants, structural validation, and native-provider input conversion for LanguageModelV4.

## Requirements

### Requirement: Public selected file-data arms
The provider package SHALL expose constructors for bytes, base64, URL, provider-reference, and text file data and public inspection distinguishing the registered data, URL, reference, and text arms. Constructors SHALL preserve explicitly selected empty payloads. Bytes and base64 SHALL be representations of the same registered data arm. Unambiguous existing field literals SHALL remain usable; empty string selection SHALL not depend on constructing JSON.

#### Scenario: Empty selections are observable
- **WHEN** a caller constructs empty byte data, empty base64 data, empty text, an empty URL string, or an empty reference object
- **THEN** inspection and tagged JSON SHALL retain the selected arm and required payload member rather than produce an unselected value
- **AND** structural selection SHALL not imply that every native provider accepts that payload

#### Scenario: Selected data survives round-trip
- **WHEN** valid constructed file data is marshaled and unmarshaled
- **THEN** the selected arm and semantic payload SHALL remain identical, with bytes serialized as standard base64

### Requirement: Ambiguous file data fails closed

Shared validation SHALL reject unselected data, conflicting arms or bytes/base64 forms and malformed references. References SHALL be string-value objects without reserved `type`; empty objects SHALL remain structurally valid. Explicit selection plus its payload SHALL count as one arm. Marshaling/native conversion SHALL NOT choose a populated field over conflicting selection. Supported unambiguous legacy forms SHALL remain decodable.

#### Scenario: Selected empty text conflicts with a URL
- **WHEN** a caller selects empty text and then adds a URL payload
- **THEN** validation and marshaling SHALL fail, and direct provider calls SHALL issue zero external provider requests

#### Scenario: Tagged decoding cannot erase conflicts
- **WHEN** SDK JSON decoding receives `{"type":"text","text":"","url":""}`, another inactive arm with a null value, or a tagged payload combined with a legacy payload alias
- **THEN** decoding SHALL fail rather than discard the inactive member and return valid selected data

#### Scenario: Unambiguous legacy decoding remains supported
- **WHEN** SDK decoding receives an existing supported legacy file-data form with one payload source
- **THEN** it SHALL map to the corresponding canonical selected arm without accepting mixed tagged and legacy payloads

#### Scenario: Invalid provider reference
- **WHEN** a reference is null, malformed JSON, a non-object, contains non-string values, or contains the reserved `type` member
- **THEN** validation SHALL fail before external provider I/O

#### Scenario: Empty reference lacks the selected provider
- **WHEN** a structurally valid reference lacks an identifier required by the selected native provider
- **THEN** the converter SHALL apply the pinned provider's missing-reference behavior rather than reinterpret it as another file arm

### Requirement: Tagged file decoding retains conflict detection

SDK tagged decoding SHALL reject other-arm members, including empty/null values and legacy payload aliases, before discarding them. This focused check SHALL NOT require Gateway schemas.

#### Scenario: Tagged file decoding retains conflict detection
- **WHEN** a tagged text payload also contains an empty URL member
- **THEN** SDK decoding SHALL reject the conflict before discarding it, without importing Gateway schemas

### Requirement: Native file conversion respects selected input semantics

Anthropic/shared Vertex, Bedrock, OpenAI Responses and OpenAI-compatible SHALL inspect selection, not non-emptiness; retain pinned role, media, URL, reference, option, warning and error semantics; and not drop/change required empty text/data to another arm. Invalid direct-call structures SHALL fail before external I/O. Unsupported content SHALL match pinned upstream or a documented existing deviation.

#### Scenario: Empty inline text document
- **WHEN** an Anthropic or Bedrock request uses an explicitly selected empty text file supported by that converter
- **THEN** native request conversion SHALL retain the text-document representation and empty content

#### Scenario: Nullish filename defaults
- **WHEN** OpenAI Responses receives otherwise equivalent PDF inputs with absent and explicit empty filenames
- **THEN** only the absent filename SHALL receive its default, and raw native request JSON SHALL preserve the explicit empty value
- **AND** equivalent presence checks SHALL cover tool-result files and OpenAI-compatible PDF inputs

#### Scenario: Bedrock document names
- **WHEN** Bedrock receives absent, empty, or non-empty filenames
- **THEN** document naming and sanitization SHALL match the pinned Bedrock converter rather than a provider-independent default rule

#### Scenario: Media restrictions remain provider-specific
- **WHEN** a request uses full, bare top-level, or wildcard media types, a restricted URL scheme, or an unsupported text/reference arm
- **THEN** the selected provider SHALL apply its registered conversion or unsupported-content behavior without a new Gateway-wide MIME allowlist or implicit URL fetch

### Requirement: Native file filename defaults

Filename defaults SHALL distinguish absence from explicit empty wherever pinned providers use nullish defaulting; provider-specific empty-name normalization SHALL remain intact.

#### Scenario: Native file filename defaults
- **WHEN** a provider using nullish filename defaults receives an explicitly empty name
- **THEN** it SHALL preserve explicit empty rather than apply an absent-name default, while retaining provider-specific empty-name normalization
