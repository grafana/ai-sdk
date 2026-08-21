## ADDED Requirements

### Requirement: DataContent has an exact public selection API

`DataContent` SHALL remain the shared request/response value with its existing exported payload fields and private selection state. The `provider` package SHALL add the following exported discriminator type, constructors, and inspector without adding exported structural state to response values:

```go
type DataContentType string

const (
    DataContentTypeData      DataContentType = "data"
    DataContentTypeURL       DataContentType = "url"
    DataContentTypeReference DataContentType = "reference"
    DataContentTypeText      DataContentType = "text"
)

type DataContent struct {
    Bytes     []byte          `json:"bytes,omitempty"`
    Base64    string          `json:"base64,omitempty"`
    URL       string          `json:"url,omitempty"`
    Reference json.RawMessage `json:"reference,omitempty"`
    Text      string          `json:"text,omitempty"`
    // private selection state only when zero-value inference is impossible
}

func BytesDataContent(data []byte) DataContent
func Base64DataContent(data string) DataContent
func URLDataContent(url string) DataContent
func ReferenceDataContent(reference json.RawMessage) DataContent
func TextDataContent(text string) DataContent

func (d DataContent) DataType() (DataContentType, bool)
```

Bytes and raw JSON inputs SHALL be copied. `DataType` SHALL use private selection only when an empty payload requires it; otherwise it SHALL infer exactly one arm from non-nil bytes or non-empty base64, non-empty URL, non-empty reference, or non-empty text. `DataContent{}` and conflicting values SHALL remain invalid.

The data arm SHALL permit empty bytes and empty base64. `Base64DataContent("")` SHALL use the established non-nil empty-byte representation so selection and structural round trips remain stable. Simultaneous non-nil bytes and non-empty base64 SHALL be invalid. Empty URL and empty text constructors SHALL record private selection. The reference arm SHALL require a non-null JSON object with string values and SHALL permit `{}`. Every selected or inferred arm SHALL reject non-zero or non-nil payloads belonging to another arm.

`MarshalJSON` and `UnmarshalJSON` SHALL remain compatibility behavior that emits and accepts the established tagged union and preserves current structural response round trips. Decoding SHALL leave private selection empty whenever non-empty legacy payload fields or non-nil bytes infer the arm; it SHALL record private selection only for otherwise-uninferable empty URL or text. Protocol mappers SHALL call `DataType`, inspect payload fields directly, and SHALL NOT delegate protocol authority to these methods. Existing response fields, codecs, untagged response literals, and `reflect.DeepEqual` response tests SHALL remain unchanged.

#### Scenario: Empty inline text is constructible
- **WHEN** `TextDataContent("")` is called
- **THEN** `DataType` SHALL return `DataContentTypeText`
- **AND** validation SHALL succeed without JSON round-tripping

#### Scenario: Empty byte and base64 data are constructible
- **WHEN** `BytesDataContent(nil)` or `Base64DataContent("")` is called
- **THEN** `DataType` SHALL return `DataContentTypeData`
- **AND** compatibility encoding SHALL emit the required empty `data` member

#### Scenario: External mapper inspects the selected arm
- **WHEN** a package outside `provider` receives a `DataContent`
- **THEN** it SHALL determine the selected or inferred arm through `DataType`
- **AND** it SHALL NOT need to marshal the value or inspect private state

#### Scenario: Existing response literal round-trips structurally
- **WHEN** response content uses an untagged non-empty legacy value such as `DataContent{URL: "https://example.com/file"}`
- **THEN** `DataType` SHALL infer `DataContentTypeURL`
- **AND** compatibility encoding and decoding SHALL leave private selection empty so the decoded value remains structurally equal

#### Scenario: Empty text round-trips with selection
- **WHEN** `TextDataContent("")` is compatibility-encoded and decoded
- **THEN** the decoded value SHALL retain private text selection and `DataContentTypeText`

#### Scenario: Data representations conflict
- **WHEN** data has both non-nil bytes and non-empty base64
- **THEN** validation SHALL fail rather than choosing one representation

#### Scenario: Inactive payload conflicts
- **WHEN** selection or inference implies multiple arms
- **THEN** validation SHALL fail rather than choosing one payload silently

#### Scenario: Reference validation is exact
- **WHEN** a reference is null, not an object, or contains a non-string value
- **THEN** validation SHALL fail
- **AND** an empty object SHALL remain valid

### Requirement: Optional nested request scalars preserve presence

Request-side optional strings SHALL use `*string` where Phase 1 proved that an explicit empty string differs from absence. The fields SHALL include `ContentPart.FilePartFilename`, `ContentPart.Reason`, `ToolResultContentValue.Filename`, and `ToolResultOutput.Reason`. Request-side `ContentPart.ProviderExecuted` SHALL use `*bool` so an explicit false tool-call value differs from absence. `ContentPart.Filename string` SHALL remain unchanged for generated response files and `ContentPartTypeSource`. Required strings and unrelated response-domain fields SHALL remain value types.

#### Scenario: Prompt file filename distinguishes empty from absent
- **WHEN** a prompt file uses a non-nil `FilePartFilename` pointing to `""`
- **THEN** the provider value SHALL differ from the same part with nil `FilePartFilename`

#### Scenario: Tool-result file filename distinguishes empty from absent
- **WHEN** a tool-result file uses a non-nil `Filename` pointing to `""`
- **THEN** the provider value SHALL differ from the same value with nil `Filename`

#### Scenario: Response and source filenames remain value fields
- **WHEN** generated file content or source content carries a filename
- **THEN** it SHALL continue to use `ContentPart.Filename string` with its existing response/source behavior

#### Scenario: Optional reason distinguishes empty from absent
- **WHEN** an approval response or execution-denied output uses a non-nil reason pointer to `""`
- **THEN** the provider value SHALL differ from the same value with a nil reason

#### Scenario: Tool-call provider execution distinguishes false from absent
- **WHEN** a tool-call content part uses a non-nil `ProviderExecuted` pointer to false
- **THEN** the provider value SHALL differ from a tool call with a nil `ProviderExecuted`

#### Scenario: Required empty string remains a value
- **WHEN** a required text, media type, tool name, or correlation identifier is empty
- **THEN** its field SHALL remain a string value whose required wire presence is decided by an explicit protocol mapper

## MODIFIED Requirements

### Requirement: Message is a flat discriminated struct

The `provider` package SHALL define `Message` as a single transport-neutral struct, not a sealed interface:

```go
type Message struct {
    Role            Role            `json:"role"`
    Content         []ContentPart   `json:"content"`
    ProviderOptions ProviderOptions `json:"providerOptions,omitempty"`
}
```

The `Role` field SHALL discriminate the message variant (`"system"`, `"user"`, `"assistant"`, `"tool"`). The previous `Message` sealed interface and the four concrete variants (`SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`) SHALL remain removed. Generic `encoding/json` behavior SHALL NOT define a strict HTTP protocol representation.

#### Scenario: Message is a struct
- **WHEN** the `provider.Message` type is inspected
- **THEN** it SHALL be a Go struct exported as `provider.Message` with public fields `Role`, `Content`, `ProviderOptions`

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** `provider.SystemMessage`, `provider.UserMessage`, `provider.AssistantMessage`, and `provider.ToolMessage` SHALL NOT exist as identifiers

#### Scenario: Message preserves normalized content
- **WHEN** a `Message` carries any supported role, ordered content, provider options, and nil or non-nil collections
- **THEN** ordinary in-process copying SHALL preserve those domain values

#### Scenario: Protocol mapping is explicit
- **WHEN** a protocol encodes a `Message`
- **THEN** it SHALL map the selected role and valid content explicitly
- **AND** it SHALL NOT infer strict wire validity from `encoding/json` output

#### Scenario: Round-trip via encoding/json
- **WHEN** a request `Message` carrying every role is compatibility-marshaled and unmarshaled
- **THEN** the decoded request value SHALL equal the original with no supported field loss
- **AND** this round trip SHALL NOT establish strict protocol validity

### Requirement: ContentPart is a flat discriminated struct

The `provider` package SHALL define `ContentPart` as a single flat transport-neutral struct discriminated by a typed `Type` field:

```go
type ContentPart struct {
    Type             ContentPartType  `json:"type"`
    Text             string           `json:"text,omitempty"`
    Data             *DataContent      `json:"data,omitempty"`
    FilePartFilename *string           `json:"-"` // prompt request files
    Filename         string            `json:"filename,omitempty"` // generated response files and sources
    MediaType        string            `json:"mediaType,omitempty"`
    Kind             string           `json:"kind,omitempty"`
    SourceType       SourceType       `json:"sourceType,omitempty"`
    ID               string           `json:"id,omitempty"`
    URL              string           `json:"url,omitempty"`
    Title            string           `json:"title,omitempty"`
    ToolCallID       string           `json:"toolCallId,omitempty"`
    ToolName         string           `json:"toolName,omitempty"`
    Input            json.RawMessage  `json:"input,omitempty"`
    Output           *ToolResultOutput `json:"output,omitempty"`
    ProviderExecuted *bool            `json:"providerExecuted,omitempty"`
    ApprovalID       string           `json:"approvalId,omitempty"`
    Signature        string           `json:"signature,omitempty"`
    IsAutomatic      bool             `json:"isAutomatic,omitempty"`
    Approved         *bool            `json:"approved,omitempty"`
    Reason           *string          `json:"reason,omitempty"`
    ProviderOptions  ProviderOptions  `json:"providerOptions,omitempty"`
}
```

The previous sealed interfaces `UserContentPart`, `AssistantContentPart`, `ToolMessageContentPart` SHALL remain removed. The previous concrete types `TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, and `ToolApprovalResponseContentPart` SHALL remain removed.

The flat representation MAY contain fields belonging to inactive arms in memory. A direct provider request mapper or strict protocol mapper MUST validate the selected role, arm, and filename direction and MUST NOT silently emit, discard, concatenate, or reinterpret inactive-arm fields. The tolerant legacy ProviderWire adapter is exempt only for values accepted by its parent encoder compatibility domain.

Compatibility JSON SHALL map filename fields by selected arm and direction. For a request file arm, a non-nil `FilePartFilename` SHALL encode as `filename`, including `""`; for a source arm, `Filename` SHALL retain its existing `filename` member. File decoding SHALL always populate `FilePartFilename`; source decoding SHALL populate `Filename`. Compatibility encoding SHALL reject both fields populated rather than selecting one.

A generated-response file with nil `FilePartFilename` MAY encode its non-empty `Filename` through the historical `filename` member, but decoding that generic file JSON SHALL normalize the value into request-owned `FilePartFilename` and leave `Filename` empty. Structural generated-response round-trip through generic provider-message JSON is intentionally not guaranteed. Dedicated generated-result, stream, SSE, and response codecs SHALL retain their existing response-owned filename behavior.

#### Scenario: Removed types
- **WHEN** the `provider` package is inspected
- **THEN** none of the listed concrete content-part types and none of the three `*ContentPart` interfaces SHALL exist as identifiers

#### Scenario: ContentPartType constants exist
- **WHEN** `ContentPartType` is inspected
- **THEN** it SHALL be a typed string with constants for at least `text`, `file`, `reasoning`, `reasoning-file`, `source`, `tool-call`, `tool-result`, `custom`, `tool-approval-request`, and `tool-approval-response`

#### Scenario: Every valid request ContentPartType is representable
- **WHEN** every defined request `ContentPartType` is constructed with its required values and optional presence states
- **THEN** the provider value SHALL preserve the selected arm and all populated domain fields

#### Scenario: Round-trip every ContentPartType
- **WHEN** every defined request or source `ContentPartType` is compatibility-marshaled and unmarshaled with valid arm state
- **THEN** the decoded value SHALL equal the original except for the documented generated-file response-to-request filename normalization

#### Scenario: Request mapping rejects invalid inactive fields
- **WHEN** a direct provider or future strict V4 request mapper receives a content part with fields invalid for its selected arm, role, or filename direction
- **THEN** mapping SHALL fail rather than normalize the part silently

#### Scenario: File compatibility JSON preserves filename presence
- **WHEN** file parts with `FilePartFilename == nil`, a pointer to `""`, and a pointer to `"report.pdf"` are compatibility-encoded and decoded
- **THEN** the `filename` member and decoded pointer SHALL preserve absent, explicit-empty, and non-empty states distinctly

#### Scenario: Generated file compatibility JSON normalizes to a request file
- **WHEN** a generated file with `Filename == "report.pdf"` and nil `FilePartFilename` is compatibility-encoded and decoded
- **THEN** encoding SHALL retain `filename: "report.pdf"`
- **AND** decoding SHALL set `FilePartFilename` to `"report.pdf"` and clear `Filename`
- **AND** tests SHALL NOT claim structural generated-response equality through this request-directional codec

#### Scenario: Source compatibility JSON retains Filename
- **WHEN** a source part with `Filename == "report.pdf"` is compatibility-encoded and decoded
- **THEN** its `filename` member SHALL decode back into `Filename`
- **AND** `FilePartFilename` SHALL remain nil

#### Scenario: Dedicated response filename behavior remains unchanged
- **WHEN** generated-result or stream response codecs encode and decode a generated file filename
- **THEN** they SHALL retain response-owned `Filename`
- **AND** they SHALL NOT normalize it into `FilePartFilename`

#### Scenario: Mixed compatibility filename state fails
- **WHEN** compatibility encoding receives a content part with both filename fields populated
- **THEN** it SHALL return an error rather than choose one

### Requirement: ContentPart constructor helpers

The `provider` package SHALL preserve these per-variant constructor helpers:

- `TextPart(text string) ContentPart`
- `FilePart(mediaType string, data DataContent) ContentPart`
- `FilePartWithFilename(mediaType string, data DataContent, filename string) ContentPart`
- `ReasoningPart(text string) ContentPart`
- `ReasoningFilePart(mediaType string, data DataContent) ContentPart`
- `ToolCallPart(toolCallID, toolName string, input json.RawMessage) ContentPart`
- `ToolResultPart(toolCallID, toolName string, output *ToolResultOutput) ContentPart`
- `CustomPart(kind string) ContentPart`
- `ToolApprovalRequestPart(approvalID, toolCallID string, isAutomatic bool) ContentPart`
- `ToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart`
- `ProviderExecutedToolApprovalResponsePart(approvalID string, approved bool, reason string) ContentPart`

Each helper SHALL set the matching `ContentPartType` and relevant fields. Helpers that accept an optional reason as a string SHALL treat `""` as absent to preserve historical behavior; callers requiring explicit-empty presence SHALL set a non-nil `Reason` pointer directly.

#### Scenario: TextPart shape
- **WHEN** `TextPart("hello")` is called
- **THEN** it SHALL return `ContentPart{Type: ContentPartTypeText, Text: "hello"}`

#### Scenario: FilePart shape
- **WHEN** `FilePart("image/png", data)` is called with a valid selected `DataContent`
- **THEN** it SHALL return a file part with the media type, a pointer to the given data, and nil `FilePartFilename`

#### Scenario: FilePartWithFilename preserves empty presence
- **WHEN** `FilePartWithFilename("text/plain", data, "")` is called
- **THEN** it SHALL return a file part whose `FilePartFilename` is non-nil and points to `""`

#### Scenario: File and source filename fields cannot mix in requests
- **WHEN** a prompt request file has non-empty response/source `Filename`, a source has non-nil `FilePartFilename`, or both fields are populated
- **THEN** direct provider and explicit protocol request mapping SHALL fail rather than choosing one field

#### Scenario: ToolCallPart shape
- **WHEN** `ToolCallPart("call_1", "fetch", input)` is called
- **THEN** it SHALL return a tool-call part with the identifiers and input populated and `ProviderExecuted == nil`

#### Scenario: ToolResultPart shape
- **WHEN** `ToolResultPart("call_1", "fetch", output)` is called
- **THEN** it SHALL return a tool-result part with the identifiers and output populated

#### Scenario: ToolApprovalRequestPart shape
- **WHEN** `ToolApprovalRequestPart("apr_1", "call_1", false)` is called
- **THEN** it SHALL return a tool-approval-request part carrying the approval ID and tool-call ID without approval-decision fields

#### Scenario: Approval helper preserves historical empty behavior
- **WHEN** `ToolApprovalResponsePart` is called with an empty reason string
- **THEN** its optional reason pointer SHALL be nil

#### Scenario: Provider-executed approval helper sets true
- **WHEN** `ProviderExecutedToolApprovalResponsePart` is called
- **THEN** its `ProviderExecuted` pointer SHALL be non-nil and true

### Requirement: CallOptions.Prompt is wire-serializable

`CallOptions.Prompt` SHALL remain tagged `json:"prompt,omitempty"` for compatibility consumers and SHALL remain an ordered `[]Message` transport-neutral provider input. The generic tag and provider custom marshalers SHALL NOT be the authority for strict ProviderWire V4. Every HTTP protocol SHALL own an explicit mapper that preserves role, order, required-empty values, optional presence, collection presence, and opaque provider options.

#### Scenario: Prompt field shape
- **WHEN** the `CallOptions` struct is inspected
- **THEN** `Prompt` SHALL be an ordered slice of the flat `Message` domain type with its compatibility JSON tag

#### Scenario: Prompt JSON tag
- **WHEN** the `CallOptions` struct is inspected
- **THEN** the `Prompt` field SHALL carry `json:"prompt,omitempty"` and SHALL NOT carry `json:"-"`

#### Scenario: Prompt round-trip
- **WHEN** compatibility JSON marshals and unmarshals a non-empty valid request `Prompt` carrying every role
- **THEN** every supported request message and content part SHALL be preserved

#### Scenario: Prompt domain values remain copyable
- **WHEN** `CallOptions.Prompt` carries every supported role and content arm
- **THEN** in-process provider calls SHALL receive the same ordered domain values

#### Scenario: System content is not concatenated by strict mapping
- **WHEN** a future strict V4 mapper receives a valid system message
- **THEN** it SHALL map the one registered system text value exactly
- **AND** it SHALL reject an invalid system shape rather than concatenate or discard parts

#### Scenario: Collection presence survives in memory
- **WHEN** `Prompt` or nested content is non-nil and empty
- **THEN** the provider value SHALL retain that state for an explicit protocol mapper

#### Scenario: Generic JSON is not protocol authority
- **WHEN** strict request compatibility is evaluated
- **THEN** conformance SHALL use the protocol schema and explicit mapper output
- **AND** a provider-type `encoding/json` round trip SHALL NOT establish compatibility
