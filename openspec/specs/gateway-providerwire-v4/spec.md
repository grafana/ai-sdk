# gateway-providerwire-v4 Specification

## Purpose

Define the independent strict LanguageModelV4 request codec, its canonical request semantics, validation boundary, and coexistence with the legacy provider-wire transport.

## Requirements

### Requirement: Independent strict LanguageModelV4 request package

The repository SHALL provide `github.com/grafana/ai-sdk/gateway/providerwire/v4`, declared as package `providerwirev4`, as an independent strict request codec for the registered LanguageModelV4 contract. Production files in the package MUST depend only on the Go standard library and `github.com/grafana/ai-sdk/provider`; they MUST NOT import the legacy `gateway/providerwire` package or gateway handler, runtime, catalog, failure, client, HTTP, or SSE layers.

The package SHALL export canonical request encoding through `EncodeCallOptions`. Request DTOs and strict request decoding SHALL remain private. The package MUST NOT expose a public DTO model or a public general-purpose request decoder.

#### Scenario: Strict package dependency boundary

- **WHEN** production imports under `gateway/providerwire/v4` are inspected
- **THEN** they SHALL contain no dependency on the legacy provider-wire package or another gateway layer

#### Scenario: Request-only public surface

- **WHEN** the strict package's exported identifiers are inspected
- **THEN** it SHALL export `EncodeCallOptions` and SHALL NOT export request DTO types or a strict request decoder

#### Scenario: No transport execution surface

- **WHEN** the strict request package is inspected
- **THEN** it SHALL contain no HTTP handler, route, model resolver, catalog lookup, result decoder, stream decoder, error envelope, or SSE framing behavior

### Requirement: Private explicit request DTO boundary

Canonical call options, messages, content parts, file data, tools, tool choices, response formats, tool-result outputs, and tool-result content SHALL be represented by unexported wire DTOs and converted field by field to or from provider-domain values. The strict codec MUST NOT obtain canonical nested request JSON by directly marshaling `provider.CallOptions`, `provider.Message`, `provider.ContentPart`, `provider.Tool`, `provider.ToolResultOutput`, `provider.ToolResultContentValue`, or `provider.DataContent`.

Intrinsically opaque request values MAY pass through as copied `json.RawMessage` only after the codec validates the required JSON boundary for that value.

#### Scenario: Provider JSON methods do not own strict bytes

- **WHEN** canonical call options containing nested messages, file data, tools, and tool results are encoded
- **THEN** every standardized field and discriminator SHALL be selected by private DTO conversion rather than provider-domain JSON methods

#### Scenario: Opaque JSON remains lossless

- **WHEN** a valid schema, tool input, provider-tool argument, provider option object, or JSON tool-result value crosses the strict codec
- **THEN** its JSON semantic value SHALL be preserved without reinterpretation

### Requirement: Canonical CallOptions conversion

`EncodeCallOptions` SHALL emit canonical LanguageModelV4 request JSON for every request field represented by `provider.CallOptions`: `prompt`, `tools`, `toolChoice`, `maxOutputTokens`, `temperature`, `topP`, `topK`, `presencePenalty`, `frequencyPenalty`, `stopSequences`, `responseFormat`, `seed`, `reasoning`, `includeRawChunks`, `headers`, and `providerOptions`.

The encoded object SHALL always contain `prompt`, using an empty array when the provider value has no messages. The codec SHALL preserve the distinction between absent and explicitly empty `tools` or `stopSequences` so an empty caller value continues to suppress downstream defaults. The private decoder SHALL require `prompt` to be a non-null array and SHALL produce the equivalent provider value for every supported canonical request.

#### Scenario: Full canonical request round trip

- **WHEN** call options populate every supported setting together with representative messages, tools, provider options, and headers
- **THEN** strict encode followed by strict decode SHALL preserve canonical request semantics, while normalizing multiple system text parts into one part and removing an empty top-level gateway namespace

#### Scenario: Empty request has explicit prompt

- **WHEN** zero-valued call options are encoded
- **THEN** the canonical JSON SHALL contain `"prompt":[]`

#### Scenario: Explicit empty optional collections remain present

- **WHEN** call options contain non-nil empty `tools` or `stopSequences`
- **THEN** strict encoding SHALL emit the corresponding empty array and strict decoding SHALL preserve a non-nil empty slice

#### Scenario: Missing or null prompt is rejected

- **WHEN** strict decoding receives an object with no `prompt` field or with `"prompt":null`
- **THEN** decoding SHALL fail without returning partial call options

#### Scenario: Supported model-setting literals

- **WHEN** tool choice, response format, or reasoning is present
- **THEN** the codec SHALL accept only the registered discriminator or reasoning literals and SHALL require the active variant's required fields

### Requirement: Canonical message roles and content arms

The strict codec SHALL encode and decode system content as a string. User content SHALL admit only `text` and `file`; assistant content SHALL admit only `text`, `file`, `custom`, `reasoning`, `reasoning-file`, `tool-call`, and `tool-result`; tool content SHALL admit only `tool-result` and `tool-approval-response`. Every message and content part SHALL require a recognized discriminator and the fields required by its active arm.

System provider options MAY appear on the message, but system content SHALL contain only plain text without content-level provider options. Encoding multiple valid system text parts SHALL concatenate their text in order. Tool approval responses SHALL require a non-empty approval ID and a present boolean `approved` value.

#### Scenario: Canonical system message

- **WHEN** a system message containing plain text is encoded
- **THEN** its `content` SHALL be a JSON string and strict decoding SHALL restore one text content part

#### Scenario: Legacy system array is rejected

- **WHEN** strict decoding receives a system message whose `content` is an array
- **THEN** decoding SHALL fail even though the legacy provider-wire decoder accepts that form

#### Scenario: Role/content mismatch is rejected

- **WHEN** a known content discriminator appears under a role that does not admit it
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Custom content identifier is provider-qualified

- **WHEN** custom assistant content is encoded or decoded
- **THEN** `kind` SHALL contain non-empty components on both sides of a dot

### Requirement: Request privacy allowlist

The strict request codec SHALL reject provider-domain prompt values and request JSON that contain known non-request or private fields. Prompt content MUST reject `sourceType`, `id`, `url`, `title`, `signature`, and `isAutomatic` when any is present. Tool approval responses MUST reject `toolCallId`, `toolName`, and `providerExecuted` when any is present. Top-level `abortSignal` MUST be rejected because cancellation belongs to the local transport and is not request JSON.

Unknown standard request fields SHALL be rejected by the fail-fast request-object policy below. Known private fields retain explicit contextual rejection regardless of their JSON value.

#### Scenario: Private field with any JSON value is rejected

- **WHEN** canonical-looking prompt JSON contains a known private field, including when its value is `null`
- **THEN** strict decoding SHALL fail

#### Scenario: Private provider value cannot be encoded

- **WHEN** a provider content value populates a known request-private field
- **THEN** strict encoding SHALL fail rather than expose the field

#### Scenario: Abort signal is not accepted from JSON

- **WHEN** a request object contains `abortSignal`
- **THEN** strict decoding SHALL fail regardless of its value

### Requirement: Canonical tagged file data

Prompt `file` and tool-result `file` content SHALL support canonical `data`, `url`, `reference`, and `text` tagged file-data variants. Prompt `reasoning-file` content SHALL support only `data` and `url`. File content SHALL require `data` and `mediaType`; reasoning files MUST NOT carry `filename`.

Inline bytes SHALL encode as base64 in the `data` variant. At a full-file or tool-result-file boundary, a present zero-valued `provider.DataContent` SHALL represent canonical empty inline text; this convention MUST NOT apply to reasoning files. A URL SHALL be non-empty. A provider reference SHALL be an open provider-keyed object whose values are strings and SHALL reject the exact key `type`, which is reserved to distinguish provider references from tagged file-data objects. Encoding SHALL reject provider values with no representable canonical variant or with multiple populated representations.

#### Scenario: Every prompt file-data variant round trips

- **WHEN** prompt or tool-result file content uses canonical inline data, URL, provider-reference, or inline-text data
- **THEN** strict encode and decode SHALL preserve its discriminator and semantic value

#### Scenario: Empty tagged data is preserved

- **WHEN** file data is empty inline data or `{"type":"text","text":""}`
- **THEN** strict decode followed by encode SHALL preserve the same tagged empty meaning

#### Scenario: Reasoning file rejects prompt-only variants

- **WHEN** reasoning-file data uses `reference` or `text`
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Invalid provider reference is rejected

- **WHEN** a provider reference is not an object, contains a non-string value, or contains the reserved key `type`
- **THEN** strict encoding or decoding SHALL fail

### Requirement: Canonical tool and tool-result unions

The strict codec SHALL support function and provider tools. A function tool SHALL require a non-empty name and an object-valued input schema; each input example SHALL contain an object-valued input. A provider tool SHALL require a non-empty name, a provider-qualified ID, and a present object-valued `args` field, including when the object is empty. Strict encoding SHALL reject non-empty provider-domain `providerOptions` on a provider tool. Strict decoding SHALL treat non-reserved `providerOptions` as a known inactive function-tool sibling field and ignore it, while still rejecting a detectable nested `gateway` namespace.

Tool calls SHALL require a non-empty call ID, non-empty tool name, and one valid JSON input value. Tool results SHALL require a non-empty call ID, non-empty tool name, and one canonical output arm: `text`, `json`, `error-text`, `error-json`, `content`, or `execution-denied`. Content output SHALL support canonical `text`, `file`, and `custom` values. The JSON and error-JSON output arms SHALL preserve JSON `null` as a valid value.

Output-level `providerOptions` SHALL be supported on every output arm except `content`, where the registered contract admits provider options only on nested content values. Strict encoding SHALL reject a `content` output with non-empty output-level provider options. Strict decoding SHALL ignore a non-reserved output-level provider-options field on the `content` arm as a known inactive sibling-arm field, while still rejecting a nested reserved `gateway` namespace.

#### Scenario: Function tool schema and examples use object boundaries

- **WHEN** a function tool has a non-object schema or an input example whose input is not an object
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Empty provider-tool args remain present

- **WHEN** a provider tool has an empty arguments map
- **THEN** encoding SHALL emit `"args":{}` and decoding SHALL preserve a non-nil empty map

#### Scenario: Provider-qualified tool ID is required

- **WHEN** a provider tool ID lacks a non-empty provider component or a non-empty tool component separated by a dot
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Provider-tool options are inactive on decode

- **WHEN** strict decoding receives a provider tool with non-reserved `providerOptions`
- **THEN** decoding SHALL ignore that known inactive sibling field while preserving the provider tool's active fields

#### Scenario: Provider-tool options cannot be encoded

- **WHEN** a provider-domain provider tool contains non-empty `providerOptions`
- **THEN** strict encoding SHALL fail rather than emit a field absent from the active arm

#### Scenario: Every canonical tool-result output arm round trips

- **WHEN** a tool result uses any supported output discriminator with valid active-arm data
- **THEN** strict encode and decode SHALL preserve the selected output and provider options at locations admitted by that arm

#### Scenario: Content output excludes output-level provider options

- **WHEN** a provider-domain `content` output has non-empty output-level provider options
- **THEN** strict encoding SHALL fail rather than emit a field absent from the canonical arm

#### Scenario: Content-output provider options are an inactive sibling field

- **WHEN** strict decoding receives a `content` output with non-reserved output-level provider options
- **THEN** decoding SHALL ignore that inactive field while preserving provider options on nested content values

#### Scenario: Legacy tool-result shapes are rejected

- **WHEN** strict decoding receives split `text`, `json`, or `content` output fields or legacy `file-data`, `file-url`, or `file-reference` content discriminators
- **THEN** decoding SHALL fail

### Requirement: Strict active-arm validation with fail-fast unknown fields

The private decoder SHALL reject non-object boundaries, unknown discriminators, unknown standard fields, missing required fields, malformed active fields, known legacy fields, and `null` for typed fields that require a concrete string, number, boolean, array, or object. Unknown-field rejection SHALL apply to the top-level call-options object and every nested standard request object before model invocation. The decoder SHALL accept JSON `null` only for opaque values whose canonical type admits null, including tool-call input and JSON tool-result output.

For a recognized discriminator, fields defined by the same union but belonging only to inactive sibling arms SHALL be ignored. Provider-options namespaces, provider references, headers, schemas, tool inputs, tool arguments, and JSON tool-result values SHALL remain explicit keyed or opaque extension boundaries and SHALL NOT apply a fixed inner-field allowlist beyond their own structural contract.

#### Scenario: Unknown discriminator fails closed

- **WHEN** a message, content, file-data, tool, tool-choice, response-format, tool-output, or tool-result-content union carries an unknown discriminator
- **THEN** decoding SHALL fail without a partial provider value

#### Scenario: Unknown standard field fails closed

- **WHEN** the top-level call options or a nested standard request object contains a field outside its complete understood field set
- **THEN** decoding SHALL fail before model invocation rather than silently discard caller intent

#### Scenario: Typed null fails closed

- **WHEN** a required or present optional typed field is `null` where the active arm requires a concrete typed value
- **THEN** decoding SHALL fail

#### Scenario: Opaque nullable JSON remains valid

- **WHEN** tool-call input or a JSON tool-result output is the JSON value `null`
- **THEN** strict decoding SHALL preserve that JSON value

#### Scenario: Inactive sibling fields do not widen active arms

- **WHEN** a known union object contains a field defined only for an inactive sibling arm with an incompatible value
- **THEN** decoding SHALL ignore that field and preserve the selected active-arm semantics

#### Scenario: Explicit extension boundaries remain open

- **WHEN** provider options or another defined opaque or keyed request value contains arbitrary valid inner fields
- **THEN** strict decoding SHALL preserve those fields subject to that boundary's structural, privacy, and reserved-namespace rules

### Requirement: Provider options and reserved gateway namespace

Every provider-options value accepted by the strict codec SHALL be a JSON object and SHALL round trip as a `provider.RawProviderOption` after decoding. Encoding SHALL preserve the opaque payload for both value and non-nil pointer forms of `provider.RawProviderOption`; a nil pointer value SHALL be rejected. At the top-level call-options boundary, an absent `gateway` entry or a `gateway` entry equal to an empty object SHALL be removed. A top-level `gateway` value that is non-empty, `null`, or not an object SHALL be rejected.

A `gateway` provider-options entry nested in a message, content part, function tool, tool-result output, or tool-result content value SHALL always be rejected, including when it is an empty object.

This request-only codec intentionally does not implement Gateway routing, fallback, credential, caching, timeout, tier, attribution, or data-governance controls. Specific top-level controls MAY be introduced by a future gateway policy layer, but they are outside this phase and MUST NOT pass through to provider-visible options.

#### Scenario: Top-level empty gateway options are removed

- **WHEN** call options contain `providerOptions.gateway` as an empty object alongside zero or more provider namespaces
- **THEN** strict encoding or decoding SHALL succeed, omit `gateway`, and preserve every other namespace

#### Scenario: Top-level gateway controls are rejected

- **WHEN** `providerOptions.gateway` is non-empty, `null`, or not an object
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Nested gateway namespace is rejected

- **WHEN** any nested provider-options map contains a `gateway` key
- **THEN** strict encoding or decoding SHALL fail regardless of the nested value

#### Scenario: Provider options require objects

- **WHEN** a provider-options namespace contains a scalar, array, or null value
- **THEN** strict encoding or decoding SHALL fail

#### Scenario: Raw provider option representations preserve their payload

- **WHEN** strict encoding receives an object-valued `provider.RawProviderOption` in value or non-nil pointer form
- **THEN** it SHALL emit the wrapped JSON object without serializing the Go wrapper fields
- **AND** it SHALL reject a nil `provider.RawProviderOption` pointer

### Requirement: Legacy provider-wire behavior remains unchanged

Introducing the strict V4 request package MUST NOT change production files, exported identifiers, tolerant decoding, resolver contracts, request headers, or error behavior in `gateway/providerwire`. The legacy request decoder SHALL continue accepting both canonical request forms and the historical Go request forms it currently supports.

#### Scenario: Legacy public API remains source-compatible

- **WHEN** existing consumers compile against `gateway/providerwire`
- **THEN** all existing exported request, response, error, SSE, handler, resolver, route, header, MIME, timeout, and limit identifiers SHALL remain available with their existing types and values

#### Scenario: Legacy tolerant request decoding remains available

- **WHEN** the legacy request decoder receives a historical system-content array or another currently supported compatibility form
- **THEN** it SHALL continue decoding that request successfully

#### Scenario: Canonical request remains accepted by legacy handler

- **WHEN** the existing provider-wire handler receives a valid canonical request body
- **THEN** it SHALL continue passing equivalent call options to the resolved model

### Requirement: Request codec verification evidence

The strict request capability SHALL include focused tests for canonical conversion, every supported request union arm, invalid provider-domain encoding, strict decode rejection, unknown-field rejection, inactive sibling-arm tolerance, opaque extension boundaries, typed null handling, request privacy, reserved namespaces, package dependencies, and legacy compatibility. Verification SHALL include race-enabled tests for both provider-wire packages, gateway provider-wire vetting, and the repository's registered parity validation.

#### Scenario: Focused verification succeeds

- **WHEN** the strict request codec change is complete
- **THEN** request-focused tests, dependency checks, legacy compatibility checks, vetting, and registered parity validation SHALL pass without requiring result, stream, HTTP, SSE, catalog, or client behavior from the strict package
