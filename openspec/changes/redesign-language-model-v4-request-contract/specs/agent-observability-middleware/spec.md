## MODIFIED Requirements

### Requirement: Recording maps file parts to Agent Observability media

Recording SHALL map supported `file` and `reasoning-file` content from prompts, generated results, and provider streams to `agento11y.PartKindMedia` parts without changing the model request, provider result, or provider/UI wire types. The recorded media metadata SHALL preserve whether the source was a `file` or `reasoning-file` part.

Only image and video media SHALL be recorded. Byte and base64 payloads SHALL be converted to base64 data URLs. Valid data URLs and HTTP(S) URLs SHALL be retained verbatim; recording SHALL NOT fetch remote URLs. The mapper SHALL determine a concrete MIME type from the declared media type, data URL, filename, URL path, or sniffed inline bytes, in that order.

For prompt request files, filename-based inference SHALL read `FilePartFilename`, preserving nil versus explicit empty and never falling back to response/source `Filename`. For generated file or source content, filename-based inference SHALL continue to read `Filename`. Reasoning-file and stream-specific response fields remain unchanged. Mixed request filename state SHALL be rejected at the request boundary rather than normalized by observability mapping.

The mapper SHALL skip data with multiple sources, provider references, inline text file data, malformed base64 or data URLs, URL credentials, non-HTTP(S) remote schemes, unsupported or ambiguous media, and conflicting concrete declared and data-URL MIME types. Percent-escaped data-URL payloads SHALL be decoded for validation without changing the retained URL. Base64 containing CR or LF SHALL be treated as malformed.

Hook preflight evaluation SHALL exclude `file` and `reasoning-file` media so recording support does not widen the hook disclosure boundary. Metadata-only agento11y export SHALL omit media URLs.

#### Scenario: Prompt and generated file parts become media

- **GIVEN** a prompt or generated result containing an image/video `file` or `reasoning-file` part with supported data
- **WHEN** the generation is mapped for recording
- **THEN** the resulting Agent Observability message SHALL contain a media part with the inferred concrete MIME type
- **AND** byte or base64 data SHALL be represented as a base64 data URL
- **AND** the media metadata SHALL identify the source as `file` or `reasoning_file`

#### Scenario: Prompt filename inference uses request ownership

- **GIVEN** a prompt file whose media type requires filename inference and whose `FilePartFilename` points to `"image.png"`
- **WHEN** the request is mapped for recording
- **THEN** inference SHALL use `"image.png"`
- **AND** it SHALL NOT read `Filename`

#### Scenario: Generated filename inference uses response ownership

- **GIVEN** generated file or source content whose media type requires filename inference and whose `Filename` is `"image.png"`
- **WHEN** the result is mapped for recording
- **THEN** inference SHALL continue to use `"image.png"`
- **AND** it SHALL NOT populate or require `FilePartFilename`

#### Scenario: Unsafe or unsupported file data is skipped

- **GIVEN** a file part containing a reference, inline text data, malformed base64, conflicting MIME types, URL credentials, a non-HTTP(S) remote URL, or unsupported/ambiguous media
- **WHEN** the generation is mapped for recording
- **THEN** no media part SHALL be added for that file part

#### Scenario: Hook preflight excludes media

- **GIVEN** a prompt containing text and file media
- **WHEN** `HooksMiddleware` builds its preflight `HookEvaluateRequest`
- **THEN** the request SHALL contain the supported non-media prompt content
- **AND** SHALL NOT contain the file media or its URL/data payload

#### Scenario: Percent-escaped base64 is validated without rewriting the URL

- **GIVEN** a valid data URL whose base64 payload contains percent-escaped base64 characters
- **WHEN** the file part is mapped for recording
- **THEN** the decoded payload SHALL pass strict base64 validation
- **AND** the original data URL SHALL be retained verbatim in the media part
