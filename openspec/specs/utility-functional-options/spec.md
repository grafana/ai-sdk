# Utility Functional Options Specification

## Purpose
Define ergonomic, scoped functional options for utility APIs that previously required nil or pointer option arguments.

## Requirements

### Requirement: ConvertToModelMessages uses functional options
`ConvertToModelMessages` SHALL have the signature `func ConvertToModelMessages(messages []UIMessage, opts ...ConvertOption) ([]provider.Message, error)`. Calls with no options SHALL use default conversion settings.

#### Scenario: Called with no options
- **WHEN** `ConvertToModelMessages` is called as `ConvertToModelMessages(messages)`
- **THEN** the function converts messages using default settings without requiring a `nil` argument

#### Scenario: Called with incomplete tool filtering option
- **WHEN** `ConvertToModelMessages` is called with `WithIgnoreIncompleteToolCalls()`
- **THEN** the function applies incomplete tool call filtering during conversion

#### Scenario: Called with tools option
- **WHEN** `ConvertToModelMessages` is called with `WithTools(tools)`
- **THEN** the same option configures the tool set used for UI message conversion
- **AND** the option remains valid for `StreamText` and `GenerateText`

#### Scenario: Persisted successful tool output uses model output conversion
- **WHEN** a completed client-executed or provider-executed UI tool part has state `output-available`
- **AND** its named tool defines `ToModelOutput`
- **THEN** `ConvertToModelMessages` calls `ToModelOutput` with the persisted tool call ID, input, and output
- **AND** uses the returned `provider.ToolResultOutput` in the converted provider message

#### Scenario: Persisted tool model output conversion fails
- **WHEN** `ToModelOutput` returns an error while converting a persisted successful tool output
- **THEN** `ConvertToModelMessages` returns that error
- **AND** orchestration using `WithMessages` does not invoke the provider

#### Scenario: Pointer options are not accepted
- **WHEN** a caller attempts to pass `ConvertOptions` or `*ConvertOptions` as a conversion option
- **THEN** the code SHALL fail to compile

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `ConvertOption`
- **THEN** `ConvertToModelMessages` treats that option as absent and continues using the remaining options

### Requirement: WriteUIMessageStream uses functional options
`WriteUIMessageStream` SHALL have the signature `func WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts ...UIMessageStreamOption) error`. Calls with no options SHALL stream using default UI message stream settings.

#### Scenario: Called with no options
- **WHEN** `WriteUIMessageStream` is called as `WriteUIMessageStream(w, result)`
- **THEN** the function streams the result to the response using default settings without requiring a `nil` argument

#### Scenario: Called with output filtering options
- **WHEN** `WriteUIMessageStream` is called with options such as `WithUIMessageStreamReasoning(false)`, `WithUIMessageStreamSources(true)`, `WithUIMessageStreamStart(false)`, or `WithUIMessageStreamFinish(false)`
- **THEN** the resulting SSE stream applies those output filtering settings

#### Scenario: Called with callback options
- **WHEN** `WriteUIMessageStream` is called with options such as `WithUIMessageStreamGenerateID`, `WithUIMessageStreamOriginalMessages`, `WithUIMessageStreamMessageMetadata`, `OnUIMessageStreamStepEnd`, `OnUIMessageStreamFinish`, or `OnUIMessageStreamError`
- **THEN** the resulting UI message stream applies the configured callback behavior

#### Scenario: Pointer options are not accepted
- **WHEN** a caller attempts to pass `UIMessageStreamOptions` or `*UIMessageStreamOptions` as a UI message stream option
- **THEN** the code SHALL fail to compile

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `UIMessageStreamOption`
- **THEN** `WriteUIMessageStream` treats that option as absent and continues using the remaining options

### Requirement: UI tool error text matches the model's tool result
UI message streams SHALL send the error text of a failed application tool call as the text the model receives in its tool result, so converted UI messages reproduce the model's history. Stream `error` chunks SHALL stay masked by default. Provider-executed tool execution errors SHALL keep their original text; invalid provider-executed calls SHALL use the stream error text because the model receives no tool result for them.

#### Scenario: Default tool errors keep the model's text
- **WHEN** a tool fails or the model calls an unavailable tool and no error callback or redaction option is set
- **THEN** the UI error text equals the tool result text sent to the model in the next step
- **AND** a stream error in the same response is sent as `An error occurred.`

### Requirement: Tool error redaction is configurable
`WithUIMessageStreamRedactToolErrors(true)` SHALL replace application tool error text with the `OnUIMessageStreamError` result, or `An error occurred.` without one. Redaction SHALL default to enabled when `OnUIMessageStreamError` is set and disabled otherwise.

#### Scenario: An error callback keeps redacting tool errors
- **WHEN** `OnUIMessageStreamError` is set without `WithUIMessageStreamRedactToolErrors`
- **THEN** tool error chunks carry the callback result
- **AND** `WithUIMessageStreamRedactToolErrors(false)` limits the callback to stream errors

### Requirement: Invalid tool calls are reported once
An invalid application tool call SHALL produce one `tool-input-error` chunk; the tool error StreamText records for the model SHALL NOT add a `tool-output-error` chunk.

#### Scenario: Invalid tool calls are reported once
- **WHEN** the model calls an unavailable application tool
- **THEN** the UI stream carries one `tool-input-error` chunk for the call and no `tool-output-error` chunk
- **AND** the error callback, when redacting, is called once for that call

### Requirement: ToUIMessageStream uses functional options
`(*StreamTextResult).ToUIMessageStream` SHALL have the signature `func (r *StreamTextResult) ToUIMessageStream(opts ...UIMessageStreamOption) <-chan UIMessageChunk`. Calls with no options SHALL use default UI message stream settings.

#### Scenario: Called with no options
- **WHEN** a caller calls `result.ToUIMessageStream()`
- **THEN** the method returns a `UIMessageChunk` stream using default settings without requiring an empty `UIMessageStreamOptions{}` value

#### Scenario: Called with UI message stream options
- **WHEN** a caller calls `result.ToUIMessageStream(WithUIMessageStreamSources(true), WithUIMessageStreamReasoning(false))`
- **THEN** the stream applies those options in order

#### Scenario: Repeated scalar and callback options use last value
- **WHEN** a caller passes the same scalar or callback UI message stream option more than once
- **THEN** the later option value wins

#### Scenario: Explicit empty original messages preserve presence
- **WHEN** a caller calls `result.ToUIMessageStream(WithUIMessageStreamOriginalMessages())`
- **THEN** the stream treats original messages as explicitly present with length zero
- **AND** continuation message ID and finish-callback behavior match the previous `UIMessageStreamOptions{OriginalMessages: []UIMessage{}}` behavior

#### Scenario: Nil original message slice preserves presence
- **WHEN** a caller expands a nil `[]UIMessage` into `WithUIMessageStreamOriginalMessages(messages...)`
- **THEN** the stream still treats original messages as explicitly present
- **AND** it does not collapse the option into the omitted-original-messages case

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `UIMessageStreamOption` to `ToUIMessageStream`
- **THEN** the method treats that option as absent and continues using the remaining options

### Requirement: UI stream step metadata precedes the boundary
`ToUIMessageStream` SHALL emit metadata returned for a `StreamFinishStep` as a separate `message-metadata` chunk immediately before the corresponding `finish-step` chunk. A nil result SHALL NOT emit a metadata chunk for that step. This requirement applies equally when the result is streamed through the HTTP or Agent helpers.

#### Scenario: Finish-step metadata is visible to a step snapshot
- **WHEN** the message metadata callback returns metadata for a `StreamFinishStep` after text output
- **THEN** the UI stream emits `text-end`, `message-metadata`, then `finish-step`, in that order
- **AND** the assembled response at the step boundary includes the step metadata

### Requirement: UI streams expose step-end callbacks
`ToUIMessageStream` SHALL accept `OnUIMessageStreamStepEnd` and `CreateUIMessageStreamParams` SHALL accept `OnStepEnd`, both with `UIMessageStreamOnStepEndState`. A configured callback SHALL run once per emitted `finish-step`, after applying the chunk and before forwarding it, even without a stream-finish callback.

#### Scenario: Standalone streams use existing step boundaries
- **WHEN** `CreateUIMessageStream` receives a written or merged `finish-step` chunk
- **THEN** it assembles the chunks forwarded by the standalone stream and invokes `OnStepEnd` for the boundary
- **AND** it does not synthesize other step boundaries

#### Scenario: Step-end state is cumulative and isolated
- **WHEN** the stream emits multiple step boundaries
- **THEN** each callback receives the cumulative `ResponseMessage` and `Messages` history at its own boundary
- **AND** mutations to one snapshot do not change the original history, the stream, or another snapshot
- **AND** the last step's response equals the finish callback's response when no later chunks change the assembled message

#### Scenario: Continuation replaces the last original assistant message
- **WHEN** the response keeps the last original assistant message's ID
- **THEN** the step-end state marks `IsContinuation` and the accumulated response replaces that last entry in `Messages`
- **AND** when there is no such continuation, the step-end state clears `IsContinuation` and appends the response instead

#### Scenario: Assembly uses the original history at stream construction
- **WHEN** a step-end or stream-finish callback is configured
- **THEN** the stream captures the supplied history before starting to produce output and uses that same history for ID selection, assembly, and callback history
- **AND** changes to the input history after construction do not change the callbacks' response or continuation

#### Scenario: A step-end callback panic does not change the wire
- **WHEN** a step-end callback panics
- **THEN** the stream recovers the panic and notifies its configured error handler, if any
- **AND** it forwards the boundary and subsequent chunks without emitting an error chunk for that panic

#### Scenario: Assembly errors do not change the wire
- **WHEN** a chunk cannot be applied while assembling the step-end snapshot
- **THEN** the callback reports the error in `AssemblyError` with the partial snapshot, consistent with finish callback assembly
- **AND** the chunk is forwarded unchanged

### Requirement: StreamUIMessage uses reader functional options
`StreamUIMessage` SHALL have the signature `func StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage`. Calls with no options SHALL use default reader settings.

#### Scenario: Called with no options
- **WHEN** `StreamUIMessage` is called as `StreamUIMessage(stream)`
- **THEN** the function uses `GenerateID()` for a fallback message ID only when the stream does not provide one before the first emitted snapshot needs an ID

#### Scenario: Called with custom reader ID option
- **WHEN** `StreamUIMessage` is called with `WithUIMessageReaderGenerateID(customFn)`
- **THEN** the function uses the provided ID generator for a fallback message ID only when the stream does not provide one before the first emitted snapshot needs an ID
- **AND** the custom generator is called at most once

#### Scenario: Stream-provided ID avoids fallback generation
- **WHEN** `StreamUIMessage` receives a start chunk with `MessageID` before the first emitted snapshot
- **THEN** emitted snapshots use the stream-provided ID
- **AND** a custom fallback ID generator is not called

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `UIMessageReaderOption`
- **THEN** `StreamUIMessage` treats that option as absent and continues using the remaining options

### Requirement: AssembleUIMessage uses reader functional options
`AssembleUIMessage` SHALL have the signature `func AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error)`. Calls with no options SHALL use default reader settings.

#### Scenario: Called with no options
- **WHEN** `AssembleUIMessage` is called as `AssembleUIMessage(stream)`
- **THEN** the function uses `GenerateID()` for a fallback message ID when the drained stream does not provide one

#### Scenario: Called with custom reader ID option
- **WHEN** `AssembleUIMessage` is called with `WithUIMessageReaderGenerateID(customFn)`
- **THEN** the function uses the provided ID generator for a fallback message ID when the drained stream does not provide one
- **AND** the custom generator is called at most once

#### Scenario: Stream-provided ID avoids fallback generation
- **WHEN** `AssembleUIMessage` drains a stream that provided a start chunk with `MessageID`
- **THEN** the returned message uses the stream-provided ID
- **AND** a custom fallback ID generator is not called

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `UIMessageReaderOption`
- **THEN** `AssembleUIMessage` treats that option as absent and continues using the remaining options
