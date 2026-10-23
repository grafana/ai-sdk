## MODIFIED Requirements

### Requirement: Raw stream events

When a streaming request set `includeRawChunks` to true, the handler SHALL write each `PartRaw` with a valid JSON value as `{"type":"raw","rawValue":<value>}` in provider order, after the credential projection below. A `PartRaw` without a value, with invalid JSON or invalid UTF-8, or with an object that repeats a key after unescaping SHALL be dropped. When the request did not set `includeRawChunks` to true, every `PartRaw` SHALL be dropped. Dropped raw parts SHALL NOT end the stream. Raw events SHALL NOT be counted against a part limit or bounded by a frame size, and SHALL NOT be truncated.

#### Scenario: Requested raw parts stream in order
- **WHEN** the adapter emits raw parts between normalized parts and the caller requested raw output
- **THEN** both the registered TypeScript client and the Go client SHALL receive the raw values in the same position relative to normalized parts

#### Scenario: Unrequested raw parts are dropped
- **WHEN** the adapter emits raw parts and the caller omitted `includeRawChunks` or set it false
- **THEN** the SSE body SHALL contain no raw event and the stream SHALL finish normally

#### Scenario: Raw part without a usable value
- **WHEN** the adapter emits a raw part whose native frame was not valid JSON, whose value is not valid UTF-8, or whose value repeats an object key
- **THEN** the handler SHALL drop it and continue the stream

#### Scenario: Oversized raw event
- **WHEN** a requested raw event encodes to a frame above the former 1 MiB limit
- **THEN** the complete raw event SHALL be written in order and the stream SHALL continue to its finish
