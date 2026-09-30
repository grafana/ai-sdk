## MODIFIED Requirements

### Requirement: Request conversion to Converse format

The provider SHALL translate `provider.CallOptions` into the AWS Bedrock Converse request shape (`system`, `messages`, `inferenceConfig`, `toolConfig`, `additionalModelRequestFields`, `additionalModelResponseFieldPaths`) before each call. When a user text or inline image part opts into selective guard content, its guarded form SHALL replace only that part's ordinary Converse content block.

#### Scenario: System messages

- **WHEN** the call's prompt begins with one or more `SystemMessage` parts
- **THEN** they are emitted under the `system` array as `{text: <content>}` blocks in order, before any user/assistant messages

#### Scenario: User text message

- **WHEN** the prompt contains a `UserMessage` with a text part without an enabled per-part `guardContent` option
- **THEN** the request includes `{role: "user", content: [{text: "<content>"}]}`

#### Scenario: Supported user document media type

- **WHEN** a user file part uses one of the supported document media types
- **THEN** the request emits a Converse document block using the mapping `application/pdf` to `pdf`, `text/csv` to `csv`, `application/msword` to `doc`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document` to `docx`, `application/vnd.ms-excel` to `xls`, `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` to `xlsx`, `text/html` to `html`, `text/plain` to `txt`, or `text/markdown` to `md`
- **AND** the document bytes are preserved and the filename-derived name is sanitized to the Bedrock-compatible form

#### Scenario: Top-level inline media type resolution

- **WHEN** an inline user file part supplies only the top-level media type `image` or `application`
- **THEN** request conversion detects the full media type from the inline bytes before selecting the image or document request shape
- **AND** recognized PNG and PDF signatures resolve to `image/png` and `application/pdf` respectively

#### Scenario: Inline text document data

- **WHEN** a user file part carries inline text data and its media type is not a full type/subtype value
- **THEN** request conversion treats the media type as `text/plain`, UTF-8 encodes the text as base64 document bytes, and sanitizes the filename-derived name
- **AND** a full unsupported media type still returns an error

#### Scenario: Document name sanitation and stable fallback

- **WHEN** a user or tool-result document has a filename such as `John's  report.txt`, a name over 200 characters, or `.txt`
- **THEN** Converse uses `Johns report`, the first 200 characters of the sanitized base name, or a generated `document-N` name respectively
- **AND** sanitation strips the extension, collapses whitespace, removes characters outside ASCII letters, digits, spaces, `()[]-`, trims and caps at 200 characters before the final trim; identical names remain stable for prompt caching

#### Scenario: Unsupported user document media type

- **WHEN** a user file part contains base64 data with an unsupported non-image media type such as `application/octet-stream`
- **THEN** request conversion returns an error identifying the unsupported media type before issuing an HTTP request
- **AND** the provider MUST NOT coerce the file to a `txt` document

#### Scenario: Unsupported user file data source

- **WHEN** a user file part carries unsupported URL data (for example, a non-S3 URL) or a provider reference
- **THEN** request conversion returns an unsupported-functionality error before issuing an HTTP request
- **AND** the provider MUST NOT silently drop the file or degrade the error to a warning

#### Scenario: Supported image media types

- **WHEN** a user file part carries inline image data
- **THEN** only `image/jpeg`, `image/png`, `image/gif`, and `image/webp` are accepted as Bedrock image formats
- **AND** non-upstream aliases such as `image/jpg` and unsupported formats such as `image/avif` return an error

#### Scenario: Final assistant prefill whitespace

- **WHEN** the prompt ends with an assistant block whose final message's final content part is text containing surrounding whitespace
- **THEN** that final text is trimmed before it is emitted to Converse, while every other retained text part and signed reasoning text are emitted byte-for-byte unchanged

#### Scenario: Assistant tool call

- **WHEN** the prompt contains an `AssistantMessage` with a `ToolCallPart` of name `lookup` and input `{"q": "x"}`
- **THEN** the assistant message content includes `{toolUse: {toolUseId, name: "lookup", input: {"q": "x"}}}`

#### Scenario: Tool result message

- **WHEN** the prompt contains a `ToolMessage` with a tool result for tool call id `abc`
- **THEN** the request emits a user-role message with `{toolResult: {toolUseId: "abc", content: [...]}}` content (Converse collapses tool results into the user role)

#### Scenario: Inference config mapping

- **WHEN** the consumer sets `MaxOutputTokens=512`, `Temperature=0.5`, `TopP=0.9`, `TopK=20`, `StopSequences=["END"]`
- **THEN** the request body's `inferenceConfig` is `{maxTokens: 512, temperature: 0.5, topP: 0.9, topK: 20, stopSequences: ["END"]}`

#### Scenario: Tools and tool choice

- **WHEN** the consumer supplies function tools and `ToolChoice = required`
- **THEN** the request body includes `toolConfig.tools` with each tool's name, description, and JSON schema, and `toolConfig.toolChoice = {any: {}}`

#### Scenario: Specific tool choice

- **WHEN** the consumer sets `ToolChoice` to a specific tool named `weather`
- **THEN** the request body includes `toolConfig.toolChoice = {tool: {name: "weather"}}`

#### Scenario: Auto tool choice

- **WHEN** the consumer sets `ToolChoice = auto`
- **THEN** the request body includes `toolConfig.toolChoice = {auto: {}}`

#### Scenario: Unsupported parameters produce warnings

- **WHEN** the consumer sets `FrequencyPenalty`, `PresencePenalty`, or `Seed`
- **THEN** the call returns a `Warning{Type: "unsupported", Feature: "<param>"}` for each unset Converse field and omits it from the request

#### Scenario: Temperature clamping

- **WHEN** the consumer sets `Temperature` outside `[0, 1]`
- **THEN** the request body clamps the value to the nearest bound and emits a `Warning{Type: "unsupported", Feature: "temperature", Details: "...clamped..."}`

## ADDED Requirements

### Requirement: Selective Converse guard-content per-part options

The Bedrock adapter SHALL accept typed user text and inline image part `guardContent` controls under `amazonBedrock` or legacy `bedrock`. Enabled text parts SHALL serialize as `{guardContent:{text:{text:<part text>,qualifiers:<optional array>}}}` and enabled inline image parts as `{guardContent:{image:{format,source}}}`. Only the enabled part SHALL change; option absence or `false` SHALL preserve ordinary conversion. Qualifier values SHALL be restricted to `grounding_source`, `query`, and `guard_content` and SHALL be accepted on text parts only. Both DoGenerate and DoStream SHALL use this same request conversion.

#### Scenario: Mixed protected and unprotected user text

- **WHEN** a user message has `guardContent:true` text with two valid qualifiers, followed by ordinary text and `guardContent:false` text, followed by guarded text with an explicitly empty qualifier array
- **THEN** the Converse request contains corresponding guarded, ordinary, ordinary and guarded text blocks in that order, with both qualifier arrays (including `[]`) retained and with no qualifiers on ordinary blocks

#### Scenario: Guarded inline image among ordinary image and text

- **WHEN** a user message contains an inline image with `guardContent:true`, an inline image with `guardContent:false` and an ordinary text part
- **THEN** the first image alone is wrapped under `guardContent.image` with identical format/base64 bytes, while the other image and text remain ordinary blocks in original order

#### Scenario: Legacy part options

- **WHEN** a user text or inline image part has `guardContent:true` under legacy `bedrock` only, or modern `amazonBedrock:null` with legacy `bedrock.guardContent:true`
- **THEN** that part is guarded

#### Scenario: Modern part options take precedence

- **WHEN** modern non-null `amazonBedrock.guardContent:false` and legacy `bedrock.guardContent:true` coexist on a user text or inline image part
- **THEN** the ordinary block is emitted without merging legacy controls
- **AND** a modern object containing only unrelated keys likewise suppresses legacy fallback

#### Scenario: Unrelated typed Bedrock options on target parts

- **WHEN** a user text or inline image part carries typed `FilePartOptions` with citations in its Bedrock namespace, or an inline image carries typed text-part qualifier options without an enabled image guard flag
- **THEN** conversion ignores controls unrelated to that part and emits its ordinary text or image block without a typed-option mismatch error
- **AND** recognized guard controls, if also present for that part, retain strict validation and modern/legacy precedence

#### Scenario: Typed guard controls on a non-target document

- **WHEN** a user document part carries typed image- or text-part guard options in its Bedrock namespace
- **THEN** it remains an ordinary document, with no `guardContent` and no typed-option mismatch error
- **AND** typed `FilePartOptions.Citations` on a document continue to enable citations

#### Scenario: Validate text controls even when guard flag is false

- **WHEN** a text part's recognized raw or typed guard control is not boolean, is explicitly null, or has non-array, non-string, null or unknown qualifier values (including when `guardContent:false`)
- **THEN** request conversion returns an error before either endpoint issues an HTTP request rather than silently emitting ordinary or guarded content

#### Scenario: Validate image controls without applying text qualifiers

- **WHEN** an inline image's recognized raw or typed guard control is not boolean or is explicitly null
- **THEN** request conversion returns an error before an HTTP request
- **AND** text-only qualifier properties do not turn image, document, video, S3 URL image, or tool-result image content into guarded content

#### Scenario: Existing non-user content and default shape

- **WHEN** selective controls are absent or false, or the prompt contains system, assistant, tool-result, document, video or S3 URL image parts
- **THEN** the existing request content/ordering and top-level `guardrailConfig` passthrough remain unchanged and no `guardContent` member is emitted for those parts, including when an image/text guard option is attached to a non-target part

#### Scenario: Both Converse endpoints

- **WHEN** the same guarded user prompt is sent via DoGenerate and DoStream
- **THEN** `/converse` and `/converse-stream` each receive the equivalent `messages[*].content[*].guardContent` shape before decoding the response

### Requirement: Unary guarded-input intervention fidelity

When a non-streaming Converse response reports `stopReason: guardrail_intervened` and a trace and usage, the provider SHALL expose the content-filter finish reason while preserving the provider trace and reported usage. An authentic upstream response fixture SHALL be used for parity verification; a recorded stream guardrail event SHALL NOT be treated as proof of selective guarded input.

#### Scenario: Registered upstream intervention response

- **WHEN** the exact registered `amazon-bedrock-guard-content-intervened.json` unary fixture is replayed for a guarded query text request with trace-enabled top-level guardrailConfig
- **THEN** the result has raw finish reason `guardrail_intervened`, unified `content-filter`, the reported response text, `providerMetadata.bedrock.trace.guardrail.inputAssessment` including the blocked topic policy, and the fixture's input/output/cache/total and raw usage values
- **AND** the provider request snapshot contains guarded query text and unchanged top-level guardrailConfig
