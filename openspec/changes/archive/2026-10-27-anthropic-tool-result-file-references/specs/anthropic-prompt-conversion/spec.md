## MODIFIED Requirements

### Requirement: Tool-result file entries

On the `tool_result` path, the Anthropic provider SHALL convert a URL file entry to an image block for `image/*` and a URL document block otherwise. Inline data SHALL become a base64 image for `image/*` or a base64 PDF document for `application/pdf`, adding the `pdfs-2024-09-25` beta. Every other file entry that is not a provider reference SHALL be omitted with the `other` warning that upstream `convert-to-anthropic-prompt.ts` emits.

#### Scenario: Unsupported inline media type
- **WHEN** a tool result's content contains a file entry with inline data and media type `application/zip`
- **THEN** the entry SHALL be omitted and the result SHALL carry the warning `unsupported tool content part type: file with media type: application/zip`

#### Scenario: Reference or text data
- **WHEN** a tool result's content contains a file entry with inline text data
- **THEN** the entry SHALL be omitted and the result SHALL carry the warning `unsupported tool content part type: file with data type: text`
- **AND** a file entry with reference data SHALL instead follow "Tool-result file references" and SHALL NOT produce the warning `unsupported tool content part type: file with data type: reference`

#### Scenario: Non-image URL
- **WHEN** a tool result's content contains a `text/plain` file entry with URL data
- **THEN** the native tool_result content SHALL contain a document block with a `url` source and no warning

### Requirement: Wire-format and public API are unchanged

The `provider.LanguageModel` interface, the `BuildParams` signature, and the SSE chunk types emitted by the orchestration layer SHALL remain unchanged. Apart from the prompt-conversion behaviors specified above, Anthropic request serialization SHALL remain byte-identical for unaffected prompts.

#### Scenario: Unaffected prompts produce identical wire requests
- **WHEN** a prompt does not exercise grouping, tool-result-in-user-message conversion, final assistant prefill whitespace normalization, assistant compaction serialization, client tool-use normalization, or tool-result file references
- **THEN** the resulting Anthropic API request body SHALL be byte-identical to the pre-change output for the same input

## ADDED Requirements

### Requirement: Tool-result file references

On the `tool_result` path, a file entry with reference data SHALL use the reference's `anthropic` ID, matched exactly, as a `file` source: an image block for `image/*` and a document block for every other media type, at the entry's position and without a warning. It SHALL add the `files-api-2025-04-14` beta once per request. A reference without an `anthropic` ID SHALL fail request conversion, as on the user-content path.

#### Scenario: Referenced image
- **WHEN** a tool result's content contains an `image/png` file entry with reference data `{"anthropic": "file_img"}`
- **THEN** the native tool_result content SHALL contain `{type: "image", source: {type: "file", file_id: "file_img"}}` at that position with no warning
- **AND** the request SHALL carry the `files-api-2025-04-14` beta

#### Scenario: Referenced non-image file
- **WHEN** a tool result's content contains a file entry with reference data `{"anthropic": "file_doc"}` and media type `application/pdf`, `text/plain` or `application/zip`
- **THEN** the native tool_result content SHALL contain `{type: "document", source: {type: "file", file_id: "file_doc"}}` at that position with no warning and no `pdfs-2024-09-25` beta
- **AND** the request SHALL carry the `files-api-2025-04-14` beta

#### Scenario: Empty referenced ID
- **WHEN** a tool result's content contains file entries with reference data `{"anthropic": ""}` and media types `image/png` and `application/pdf`
- **THEN** the native tool_result content SHALL contain `{type: "image", source: {type: "file", file_id: ""}}` for the `image/png` entry and `{type: "document", source: {type: "file", file_id: ""}}` for the `application/pdf` entry
- **AND** the request SHALL carry the `files-api-2025-04-14` beta

#### Scenario: Reference without an anthropic ID
- **WHEN** a tool result's content contains a file entry whose structurally valid reference has no `anthropic` key, such as `{"openai": "file-123"}`, `{"Anthropic": "file-123"}` or `{}`
- **THEN** request conversion SHALL fail with `anthropic: no provider reference found for provider anthropic; available providers: ` followed by the sorted reference keys, and no provider request SHALL be sent

#### Scenario: Files API beta is sent once
- **WHEN** a request contains referenced files in a user message and in several tool results, and the caller also requests `files-api-2025-04-14`
- **THEN** unary and streaming requests sent by the direct provider and with the shared Vertex conversion capabilities SHALL each carry `files-api-2025-04-14` exactly once in the `anthropic-beta` header tokens

#### Scenario: Mixed entries keep order
- **WHEN** a tool result's content contains, in this order, text, inline `application/zip` data, a referenced file, inline text data, a URL document, a URL image, a base64 image and a bytes `application/pdf` entry
- **THEN** the native tool_result content SHALL contain the text, the referenced `file` block, the URL document, the URL image, the base64 image and the base64 PDF document in that order, and only the zip entry and the text-data entry SHALL produce warnings
- **AND** the request SHALL carry both the `files-api-2025-04-14` and `pdfs-2024-09-25` betas
- **AND** the caller's prompt values SHALL be unchanged after conversion
