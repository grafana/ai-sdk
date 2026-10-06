## Purpose

Define middleware behavior for simulating streamed language-model output from non-streaming generation results while preserving content and response metadata.

## Requirements

### Requirement: Simulate streaming from generate results
`SimulateStreaming` SHALL return a `Middleware` that intercepts `DoStream` calls, calls `DoGenerate` on the inner model instead, and converts the generate result into a synthetic stream of `provider.StreamPart` events.

#### Scenario: Text content converted to stream parts
- **WHEN** `DoStream` is called on a model wrapped with `SimulateStreaming`
- **AND** the inner model's `DoGenerate` returns a result with text content
- **THEN** the stream SHALL emit `stream-start`, `text-start`, `text-delta` (with the full text), `text-end`, `finish`, in that order
- **AND** the `stream-start` part SHALL carry any warnings from the generate result
- **AND** the `finish` part SHALL carry the finish reason and usage from the generate result

#### Scenario: Reasoning content converted to stream parts
- **WHEN** the inner model's `DoGenerate` returns a result containing reasoning content
- **THEN** the stream SHALL emit `reasoning-start`, `reasoning-delta`, `reasoning-end` events for the reasoning content

#### Scenario: Non-text content passed through
- **WHEN** the inner model's `DoGenerate` returns content parts that are not text or reasoning (e.g., tool calls)
- **THEN** those parts SHALL be emitted directly into the stream

#### Scenario: DoGenerate passes through unmodified
- **WHEN** `DoGenerate` is called on a model wrapped with `SimulateStreaming`
- **THEN** the call SHALL pass through to the inner model without interception

#### Scenario: Response metadata preserved
- **WHEN** `DoStream` completes via the simulated stream
- **THEN** the `StreamResult.Request` and `StreamResult.Response` SHALL carry the values from the inner model's `GenerateResult`
