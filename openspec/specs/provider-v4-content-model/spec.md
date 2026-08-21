# provider-v4-content-model Specification

## Purpose

Define the provider V4 message, content-part, and related stream/generate content model using flat discriminated structs that round-trip losslessly through JSON.
## Requirements
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

### Requirement: Message constructor helpers preserved

The provider package SHALL preserve constructor helpers for ergonomics: `NewSystemMessage(text string) Message`, `NewUserMessage(parts ...ContentPart) Message`, `NewAssistantMessage(parts ...ContentPart) Message`, and `NewToolMessage(parts ...ContentPart) Message`. Each SHALL set the appropriate `Role` and pack the arguments into `Content`.

#### Scenario: NewSystemMessage shape
- **WHEN** `NewSystemMessage("hello")` is called
- **THEN** it SHALL return `Message{Role: RoleSystem, Content: []ContentPart{{Type: ContentPartTypeText, Text: "hello"}}}`

#### Scenario: NewUserMessage shape
- **WHEN** `NewUserMessage(ContentPart{Type: ContentPartTypeText, Text: "hi"})` is called
- **THEN** it SHALL return `Message{Role: RoleUser, Content: [...]}` with the given parts

#### Scenario: NewAssistantMessage shape
- **WHEN** `NewAssistantMessage(part1, part2)` is called
- **THEN** it SHALL return `Message{Role: RoleAssistant, Content: [...]}` with both parts in order

#### Scenario: NewToolMessage shape
- **WHEN** `NewToolMessage(ContentPart{Type: ContentPartTypeToolResult, ToolCallID: "1", ToolName: "t"})` is called
- **THEN** it SHALL return `Message{Role: RoleTool, Content: [...]}` with the given part

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

### Requirement: Role-text shortcut helpers

The `provider` package SHALL provide ergonomic shortcuts for the common case of a role-only message carrying a single text part:

- `UserText(text string) Message` -- equivalent to `NewUserMessage(TextPart(text))`
- `AssistantText(text string) Message` -- equivalent to `NewAssistantMessage(TextPart(text))`

`NewSystemMessage` already serves the system-role text shortcut role.

#### Scenario: UserText shape
- **WHEN** `UserText("hello")` is called
- **THEN** it SHALL return `Message{Role: RoleUser, Content: []ContentPart{TextPart("hello")}}`

#### Scenario: AssistantText shape
- **WHEN** `AssistantText("hi back")` is called
- **THEN** it SHALL return `Message{Role: RoleAssistant, Content: []ContentPart{TextPart("hi back")}}`

### Requirement: ToolMessageContentPart sealed interface

The `ToolMessageContentPart` sealed interface SHALL no longer exist. Tool messages carry `[]ContentPart` like every other role; producers SHALL populate only `ContentPartTypeToolResult` and `ContentPartTypeToolApprovalResponse` parts inside a tool-role `Message.Content`. Producer-side validation lives in the orchestration layer; the wire MUST trust the `Type` discriminator on each part.

#### Scenario: Tool message content uses flat ContentPart
- **WHEN** a tool-role `Message` is constructed
- **THEN** its `Content` SHALL be `[]ContentPart`, with each part's `Type` set to `ContentPartTypeToolResult` or `ContentPartTypeToolApprovalResponse`

#### Scenario: ToolMessageContentPart removed
- **WHEN** the `provider` package is inspected
- **THEN** the identifier `ToolMessageContentPart` SHALL NOT exist

### Requirement: ToolMessage content type expansion

`ToolMessage` SHALL no longer exist as a distinct type. Tool-role messages SHALL be expressed as `Message{Role: RoleTool, Content: []ContentPart}`. The flat `ContentPart` MUST carry any of the previously-allowed shapes (tool result, tool approval response) discriminated by `Type`. Mixed content (both tool-result and tool-approval-response parts) in the same message SHALL be valid.

#### Scenario: Tool-role message with mixed content
- **WHEN** a tool-role `Message` is constructed with both a tool-result `ContentPart` and a tool-approval-response `ContentPart`
- **THEN** both parts SHALL be valid in the same `Content` slice

#### Scenario: Tool-role message with only tool results
- **WHEN** a tool-role `Message` is constructed with only tool-result `ContentPart` entries
- **THEN** the message SHALL be valid

### Requirement: ImageContentPart removal

The provider package SHALL NOT define `ImageContentPart` (already removed in the prior content-model expansion). With this change the `UserContentPart` interface SHALL also no longer exist. Image content MUST be expressed as `ContentPart{Type: ContentPartTypeFile, MediaType: "image/...", Data: ...}` in user-role messages.

#### Scenario: Image as file-typed ContentPart
- **WHEN** image content appears in a user-role message
- **THEN** it SHALL be expressed as `ContentPart{Type: ContentPartTypeFile, Data: &DataContent{...}, MediaType: "image/png"}`

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

### Requirement: StreamPart carries APICallError directly

`StreamPart` SHALL include a field `APICallError *APICallError` (`json:"apiCallError,omitempty"`) populated only when `Type == PartError`. The previous `Error error` field SHALL be removed. Producers SHALL wrap any error into an `*APICallError` before emitting a `PartError` event so the retryability bit and HTTP status reach consumers across any boundary (including the wire).

#### Scenario: Error event carries APICallError
- **WHEN** a provider emits a stream-error event
- **THEN** it SHALL emit `StreamPart{Type: PartError, APICallError: &APICallError{...}}` with `IsRetryable`, `StatusCode`, `Message`, `ResponseBody`, and `Data` populated as appropriate

#### Scenario: Removed Error field
- **WHEN** the `StreamPart` struct is inspected
- **THEN** it SHALL NOT have an `Error error` field

### Requirement: PartCustom stream part constant

The provider package SHALL define `PartCustom StreamPartType = "custom"` as a stream part type constant. `StreamPart` SHALL include a `Kind string` field populated when `Type` is `PartCustom`. With this change the constant and field are unchanged; the stream part as a whole MUST round-trip losslessly through JSON.

#### Scenario: Custom content in stream
- **WHEN** a provider emits a `StreamPart{Type: PartCustom, Kind: "anthropic.cache-control"}`
- **THEN** the stream part SHALL carry the `Kind` field with the custom content identifier

### Requirement: PartReasoningFile stream part constant

The provider package SHALL define `PartReasoningFile StreamPartType = "reasoning-file"` as a stream part type constant. `PartFile` and `PartReasoningFile` SHALL use `StreamPart.Data *StreamFileData` with the same generated-file data contract.

#### Scenario: Reasoning file in stream round-trips
- **WHEN** a provider emits a `PartReasoningFile` with inline or URL-valued `Data` and a `MediaType`, and the part is JSON round-tripped
- **THEN** the decoded part SHALL preserve `Type`, the data variant and value, and `MediaType`

### Requirement: Generated stream file data matches LanguageModelV4

`StreamFileData` SHALL represent exactly the generated-file variants accepted by upstream `LanguageModelV4`: inline bytes or base64 data, and a URL. It SHALL NOT admit the prompt-only `reference` or `text` variants. `StreamPart.Data` SHALL encode inline data as `{"type":"data","data":<base64>}` and URLs as `{"type":"url","url":...}` for both `PartFile` and `PartReasoningFile`.

#### Scenario: Inline stream file data round-trips
- **WHEN** either generated stream file type carries inline bytes or base64 data
- **THEN** the wire SHALL contain the upstream `data` tagged variant and decoding SHALL preserve the payload

#### Scenario: URL stream file data round-trips
- **WHEN** either generated stream file type carries `https://example.com/image.png`
- **THEN** the wire SHALL contain `{"type":"url","url":"https://example.com/image.png"}` and decoding SHALL preserve the URL

#### Scenario: Empty inline stream file data round-trips
- **WHEN** either generated stream file type carries an inline data variant with an empty payload
- **THEN** the `data` discriminator and empty payload SHALL survive encoding and decoding

#### Scenario: Prompt-only variants are rejected by the stream file type
- **WHEN** `StreamFileData` directly decodes a `reference` or `text` tagged variant
- **THEN** decoding SHALL return an unsupported-variant error

#### Scenario: Missing inline data is rejected without terminating a stream
- **WHEN** `StreamFileData` directly decodes `{"type":"data"}` without the required `data` property
- **THEN** direct decoding SHALL fail, while `StreamPart` response decoding SHALL remain lenient and leave the unrepresentable file data unset

#### Scenario: Reasoning files propagate through orchestration
- **WHEN** `StreamText` receives interleaved reasoning text and `PartReasoningFile` events
- **THEN** its public reasoning result SHALL preserve both variants in provider order, emit a reasoning-file text stream part, retain the part in response messages and step content, and emit a `reasoning-file` UI chunk

#### Scenario: Public content preserves generated-file order and metadata
- **WHEN** regular files, text, reasoning files, tools, or sources are interleaved in provider output
- **THEN** `StepResult.Content` SHALL preserve recorded provider order for those parts and regular/reasoning file content SHALL retain provider metadata

### Requirement: GenerateContentPart Kind field

`GenerateContentPart` SHALL include a `Kind string` field for `Type: "custom"` content in non-streaming generate results. The `Kind` field SHALL also be used for `Type: "reasoning-file"` content. The field MUST be JSON-serializable (`json:"kind,omitempty"`) and round-trip losslessly through the wire.

#### Scenario: Custom content in generate result
- **WHEN** a non-streaming generate result contains custom content
- **THEN** the `GenerateContentPart` SHALL have `Type: "custom"` and `Kind` set to the custom content identifier

#### Scenario: Reasoning file in generate result
- **WHEN** a non-streaming generate result contains a reasoning file
- **THEN** the `GenerateContentPart` SHALL have `Type: "reasoning-file"` with `MediaType` and `Data` fields populated

### Requirement: StreamPart tool approval fields

`StreamPart` SHALL include `ApprovalID string` for `PartToolApprovalRequest` parts. `PartToolApprovalRequest` SHALL carry the approval request ID and the tool call ID that needs approval, matching the current upstream V4 stream part. The provider package SHALL NOT define `PartToolApprovalResult`, because current upstream V4 does not define a `tool-approval-result` stream part.

User decisions SHALL be represented as tool-message content parts with type `ContentPartTypeToolApprovalResponse`, not as provider stream parts.

#### Scenario: Tool approval request in stream
- **WHEN** a provider emits `StreamPart{Type: PartToolApprovalRequest, ApprovalID: "apr_123", ToolCallID: "call_456"}`
- **THEN** the stream part SHALL carry both the approval ID and the tool call ID

#### Scenario: Tool approval result stream part is absent
- **WHEN** the provider stream part constants are inspected
- **THEN** there SHALL NOT be a `PartToolApprovalResult` constant

#### Scenario: Approval decision uses tool message content
- **WHEN** a caller records an approval decision for an approval ID
- **THEN** the decision SHALL be represented as `ContentPart{Type: ContentPartTypeToolApprovalResponse, ApprovalID: <id>, Approved: <bool>}` in a tool-role message

### Requirement: Assistant approval request content part

The `provider` package SHALL define `ContentPartTypeToolApprovalRequest = "tool-approval-request"` for assistant-role model messages. A tool approval request content part SHALL carry `ApprovalID`, `ToolCallID`, and optional automatic-approval metadata. It SHALL be valid in assistant-role messages so orchestration can persist approval requests across the stateless two-call flow.

Provider prompt conversion SHALL use assistant approval request parts only for local correlation and missing-tool-result checks. Local approval request parts SHALL NOT be forwarded to provider APIs that do not accept them.

#### Scenario: Approval request content round-trips
- **WHEN** a `ContentPart` with `Type: ContentPartTypeToolApprovalRequest`, approval ID, and tool call ID is JSON round-tripped
- **THEN** the decoded value SHALL preserve the type, approval ID, and tool call ID

#### Scenario: Assistant message can carry approval request
- **WHEN** an assistant-role `provider.Message` is constructed with a tool call part followed by a tool approval request part
- **THEN** both parts SHALL be valid in the message content slice

#### Scenario: Local approval request is stripped before provider call
- **WHEN** provider request conversion receives an assistant message containing `ContentPartTypeToolApprovalRequest`
- **THEN** the converted provider API prompt SHALL omit that approval request part

#### Scenario: Provider-executed approval response is preserved
- **WHEN** provider request conversion receives a tool-role message containing a provider-executed `ContentPartTypeToolApprovalResponse`
- **THEN** the converted provider API prompt SHALL preserve the approval response when the provider supports it

### Requirement: Root content includes approval requests

The root SDK content model used by `StepResult.Content`, `StreamTextResult.Content`, and `GenerateTextResult.Content` SHALL include a tool approval request content variant. The variant SHALL carry approval ID, tool call ID, tool name, input, dynamic flag, title, provider-executed flag, and automatic-approval metadata when available.

#### Scenario: Step content records approval request
- **WHEN** a step emits an approval request for a tool call
- **THEN** `StepResult.Content` SHALL contain a tool approval request content value correlated with the same tool call ID

#### Scenario: Approval request content serializes for consumers
- **WHEN** a result containing approval request content is marshaled to JSON by a consumer
- **THEN** the approval ID and tool call ID SHALL be available in the serialized representation

### Requirement: Provider-emitted approval requests are surfaced by orchestration

When a provider emits `PartToolApprovalRequest`, orchestration SHALL surface it as a stream approval request and SHALL record it in step content. The request SHALL be associated with the previously emitted provider-executed tool call for the same tool call ID when one exists.

#### Scenario: Provider approval request becomes stream event
- **WHEN** a provider stream includes `PartToolApprovalRequest` for a provider-executed tool call ID
- **THEN** `StreamText` SHALL emit a tool approval request stream part with the same approval ID and tool call ID

#### Scenario: Provider approval request becomes content
- **WHEN** a provider approval request is processed in a completed step
- **THEN** the step content SHALL include a tool approval request content value for that approval ID

#### Scenario: Unknown provider approval tool call is still surfaced
- **WHEN** a provider emits `PartToolApprovalRequest` before orchestration has seen the matching tool call
- **THEN** `StreamText` SHALL still emit the approval request with the provided approval ID and tool call ID
- **AND** it SHALL NOT synthesize a local tool execution for that provider-executed request

### Requirement: ToolResultContentPart implements AssistantContentPart

Neither `ToolResultContentPart` nor `AssistantContentPart` SHALL exist after the flatten. The equivalent rule under the flat model: a `ContentPart` with `Type: ContentPartTypeToolResult` MUST be valid in an assistant-role `Message.Content` slice. This shape is used for provider-executed tool results carried back into multi-turn conversations; tooling SHALL accept it without warning.

#### Scenario: Tool-result ContentPart in assistant message
- **WHEN** an assistant-role `Message` is constructed with `ContentPart{Type: ContentPartTypeToolResult}`
- **THEN** the message SHALL be valid; tooling SHALL accept the assistant-role tool-result form

### Requirement: Warning type rename

The `Warning.Type` field SHALL use `"unsupported"` (not `"unsupported-setting"`) for features the model does not support. All existing code producing `"unsupported-setting"` warnings SHALL be updated.

#### Scenario: Warning type value
- **WHEN** a provider emits a warning for an unsupported feature
- **THEN** `Warning.Type` SHALL be `"unsupported"` (not `"unsupported-setting"`)

### Requirement: Warning compatibility variant

The `Warning` type SHALL support a `"compatibility"` type value for features used in a degraded/compatibility mode. The `"compatibility"` variant uses the same fields as `"unsupported"` (`Feature` and optional `Details`).

#### Scenario: Compatibility warning
- **WHEN** a provider uses a feature in compatibility mode
- **THEN** it SHALL emit a `Warning{Type: "compatibility", Feature: "featureName", Details: "explanation"}`

### Requirement: Warning type constants

The provider package SHALL define string constants for warning types: `WarnUnsupported = "unsupported"`, `WarnCompatibility = "compatibility"`, `WarnOther = "other"`.

#### Scenario: Warning constants used instead of string literals
- **WHEN** code creates a `Warning`
- **THEN** it SHALL use the defined constants for the `Type` field

### Requirement: CallOptions.Reasoning field

`CallOptions` SHALL include a `Reasoning *string` field for controlling model reasoning effort. The valid values SHALL be: `"provider-default"`, `"none"`, `"minimal"`, `"low"`, `"medium"`, `"high"`, `"xhigh"`. The provider package SHALL define string constants for these values.

#### Scenario: Reasoning field set
- **WHEN** `CallOptions` is constructed with `Reasoning` set to a pointer to `"high"`
- **THEN** the field SHALL carry the reasoning effort level for the provider to interpret

#### Scenario: Reasoning field nil
- **WHEN** `CallOptions` is constructed without setting `Reasoning`
- **THEN** the field SHALL be `nil`, indicating no reasoning preference

### Requirement: Anthropic provider ImageContentPart removal

The Anthropic provider's user-content conversion SHALL handle image media types on file-typed `ContentPart` (base64 or URL sources). The previous `case provider.FileContentPart` type-switch branch SHALL be replaced with discriminator dispatch on `cp.Type == ContentPartTypeFile`.

#### Scenario: Image via file-typed ContentPart in user message
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "image/jpeg", Data: &DataContent{Base64: "..."}}`
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with the image source

#### Scenario: Image via URL ContentPart
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: &DataContent{URL: "..."}}`
- **THEN** the provider SHALL produce a `BetaImageBlockParam` with a URL source

#### Scenario: Non-image file ContentPart unchanged
- **WHEN** the Anthropic conversion receives a `ContentPart{Type: ContentPartTypeFile, MediaType: "application/pdf"}`
- **THEN** the provider SHALL produce a `BetaRequestDocumentBlockParam`

### Requirement: Anthropic provider unsupported content warnings

The Anthropic provider's assistant-content conversion SHALL produce a warning and skip `ContentPart` values whose `Type` it does not support natively. Specifically, `ContentPartTypeCustom` and `ContentPartTypeReasoningFile` parts MUST emit a `Warning{Type: "unsupported"}` and be omitted from the converted assistant content.

#### Scenario: Custom-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeCustom}`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the part

#### Scenario: Reasoning-file-typed ContentPart in assistant message to Anthropic
- **WHEN** the conversion encounters a `ContentPart{Type: ContentPartTypeReasoningFile}`
- **THEN** the provider SHALL add a warning with `Type: "unsupported"` and skip the part

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

Bytes and raw JSON inputs SHALL be copied. `DataType` SHALL use private selection only when an empty payload requires it; otherwise it SHALL infer exactly one arm from non-nil bytes or non-empty base64, non-empty URL, non-empty reference, or non-empty text. `DataContent{}` and conflicting values SHALL remain invalid. For a conflict, `DataType` SHALL return the selected or first inferred candidate with `ok == false`; callers MUST NOT treat that candidate as valid.

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
