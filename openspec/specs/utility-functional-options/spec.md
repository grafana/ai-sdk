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
- **WHEN** `WriteUIMessageStream` is called with options such as `WithUIMessageStreamGenerateID`, `WithUIMessageStreamOriginalMessages`, `WithUIMessageStreamMessageMetadata`, `OnUIMessageStreamFinish`, or `OnUIMessageStreamError`
- **THEN** the resulting UI message stream applies the configured callback behavior

#### Scenario: Pointer options are not accepted
- **WHEN** a caller attempts to pass `UIMessageStreamOptions` or `*UIMessageStreamOptions` as a UI message stream option
- **THEN** the code SHALL fail to compile

#### Scenario: Nil option values are ignored
- **WHEN** a caller accidentally passes a nil `UIMessageStreamOption`
- **THEN** `WriteUIMessageStream` treats that option as absent and continues using the remaining options

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
