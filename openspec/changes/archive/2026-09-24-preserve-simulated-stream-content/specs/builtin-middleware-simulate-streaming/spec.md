## MODIFIED Requirements

### Requirement: Simulate streaming from generate results
`SimulateStreaming` SHALL return a `Middleware` that intercepts `DoStream` calls, calls `DoGenerate` on the inner model instead, and converts the generate result into a synthetic stream of `provider.StreamPart` events. It SHALL preserve every applicable field of the supported `LanguageModelV4` generated content variants in the equivalent stream parts, using the established Go source and file representations, and SHALL terminate its emitting goroutine when the call context is canceled even if the stream is not being consumed.

#### Scenario: Text content converted to stream parts
- **WHEN** `DoStream` is called on a model wrapped with `SimulateStreaming`
- **AND** the inner model's `DoGenerate` returns a result with nonempty text content and provider metadata
- **THEN** the stream SHALL emit `stream-start`, `response-metadata`, `text-start` (with the text provider metadata), `text-delta` (with the full text), `text-end`, `finish`, in that order
- **AND** the `stream-start` part SHALL carry any warnings from the generate result
- **AND** the `finish` part SHALL carry the finish reason, usage, and provider metadata from the generate result

#### Scenario: Empty generated text
- **WHEN** the inner model's `DoGenerate` returns empty text followed by nonempty text
- **THEN** the empty text SHALL emit no text parts and consume no generated text ID
- **AND** the nonempty text SHALL retain its provider metadata on `text-start`

#### Scenario: Reasoning content converted to stream parts
- **WHEN** the inner model's `DoGenerate` returns a result containing reasoning content
- **THEN** the stream SHALL emit `reasoning-start` (with reasoning provider metadata), `reasoning-delta` (with the full reasoning text), `reasoning-end` events for the reasoning content
- **AND** each emitted reasoning block SHALL have its own ID, including when its text is empty

#### Scenario: Non-text content passed through
- **WHEN** the inner model's `DoGenerate` returns content parts that are not text or reasoning
- **THEN** those parts SHALL be emitted directly into the stream as equivalent `provider.StreamPart` events, in original order and without consuming text/reasoning IDs
- **AND** tool calls SHALL retain call ID, tool name, stringified JSON input, execution/dynamic flags and provider metadata
- **AND** tool results SHALL retain call ID, tool name, raw JSON result, error flag, preliminary flag (including explicit false), dynamic flag and provider metadata
- **AND** tool approval requests SHALL retain approval ID, call ID and provider metadata
- **AND** URL and document sources SHALL retain source type, ID, URL when present, title when present, document media type/filename when present, and provider metadata, serialized in the registered upstream source wire shape
- **AND** files and reasoning files SHALL retain their media type, data (including empty bytes, base64 and URL alternatives), and provider metadata
- **AND** custom content SHALL retain its kind and provider metadata

#### Scenario: DoGenerate passes through unmodified
- **WHEN** `DoGenerate` is called on a model wrapped with `SimulateStreaming`
- **THEN** the call SHALL pass through to the inner model without interception

#### Scenario: Response metadata preserved
- **WHEN** `DoStream` completes via the simulated stream
- **THEN** the `StreamResult.Request` and `StreamResult.Response` SHALL carry the values from the inner model's `GenerateResult`
- **AND** a `response-metadata` part SHALL be emitted even if no response metadata is supplied

#### Scenario: Consumer abandons a full simulated stream
- **WHEN** the simulated stream's channel is full and its consumer stops reading
- **AND** the call context is canceled
- **THEN** the emitter SHALL stop without waiting indefinitely for a channel send, and SHALL close the stream channel
