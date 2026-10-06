## ADDED Requirements

### Requirement: ConvertToModelMessages accepts a single optional pointer for options
`ConvertToModelMessages` SHALL have the signature `func ConvertToModelMessages(messages []UIMessage, opts *ConvertOptions) ([]provider.Message, error)`. When `opts` is `nil`, the function SHALL use zero-value defaults.

#### Scenario: Called with nil options
- **WHEN** `ConvertToModelMessages` is called with `nil` as the opts parameter
- **THEN** the function converts messages using default settings (no incomplete tool call filtering)

#### Scenario: Called with explicit options
- **WHEN** `ConvertToModelMessages` is called with `&ConvertOptions{IgnoreIncompleteToolCalls: true}`
- **THEN** the function applies the provided options during conversion

#### Scenario: Compile-time rejection of multiple option arguments
- **WHEN** a caller attempts to pass more than one option argument to `ConvertToModelMessages`
- **THEN** the code SHALL fail to compile

### Requirement: WriteUIMessageStream accepts a single optional pointer for options
`WriteUIMessageStream` SHALL have the signature `func WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts *UIMessageStreamOptions) error`. When `opts` is `nil`, the function SHALL use zero-value defaults.

#### Scenario: Called with nil options
- **WHEN** `WriteUIMessageStream` is called with `nil` as the opts parameter
- **THEN** the function streams the result to the response using default settings

#### Scenario: Called with explicit options
- **WHEN** `WriteUIMessageStream` is called with `&UIMessageStreamOptions{SendReasoning: boolPtr(true)}`
- **THEN** the function applies the provided options to the stream

#### Scenario: Compile-time rejection of multiple option arguments
- **WHEN** a caller attempts to pass more than one option argument to `WriteUIMessageStream`
- **THEN** the code SHALL fail to compile

### Requirement: ReadUIMessageStream accepts a single optional pointer for options
`ReadUIMessageStream` SHALL have the signature `func ReadUIMessageStream(stream <-chan UIMessageChunk, opts *ReadStreamOption) <-chan UIMessage`. When `opts` is `nil`, the function SHALL use the default ID generator.

#### Scenario: Called with nil options
- **WHEN** `ReadUIMessageStream` is called with `nil` as the opts parameter
- **THEN** the function uses `GenerateID()` for the message ID

#### Scenario: Called with explicit options
- **WHEN** `ReadUIMessageStream` is called with `&ReadStreamOption{GenerateID: customFn}`
- **THEN** the function uses the provided ID generator

#### Scenario: Compile-time rejection of multiple option arguments
- **WHEN** a caller attempts to pass more than one option argument to `ReadUIMessageStream`
- **THEN** the code SHALL fail to compile
