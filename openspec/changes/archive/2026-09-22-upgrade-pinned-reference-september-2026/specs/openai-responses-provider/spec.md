## MODIFIED Requirements

### Requirement: User and assistant message conversion
The provider SHALL convert user messages to `{ role: "user", content: [...] }`
input items mapping text parts to `input_text`, image parts to `input_image`
(via `image_url`, `file_id`, or data URI; honoring `imageDetail`), and file
parts to `input_file` (via `file_id`, `file_url`, or `filename` + `file_data`).
Reconstructed assistant text SHALL use string content in an easy-input message,
retaining phase but omitting stale item IDs. Stored assistant text SHALL use an
`item_reference` when store is true and an item ID is present. Unsupported
file media types SHALL emit a warning or error matching upstream behavior.

#### Scenario: User text and image
- **WHEN** a user message contains a text part and an image URL part
- **THEN** the input item content contains an `input_text` part and an `input_image` part with `image_url`

#### Scenario: PDF file part
- **WHEN** a user message contains an `application/pdf` file part with inline data
- **THEN** the input item content contains an `input_file` part carrying the file data and a derived filename

#### Scenario: Provider-reference image uses file ID
- **WHEN** an image file part carries a provider reference containing the active OpenAI or Azure provider key
- **THEN** the input item content SHALL contain an `input_image` part whose `file_id` is the referenced provider-specific identifier

#### Scenario: Provider-reference document uses file ID
- **WHEN** a non-image file part carries a provider reference containing the active OpenAI or Azure provider key
- **THEN** the input item content SHALL contain an `input_file` part whose `file_id` is the referenced provider-specific identifier

#### Scenario: Provider reference lacks the active provider
- **WHEN** a file part carries an empty provider reference or one without the active OpenAI or Azure provider key
- **THEN** request conversion SHALL return an error rather than emitting an empty file input or falling back to another data source

#### Scenario: Stored assistant text becomes an item reference
- **WHEN** store is true and an assistant text part carries an item ID
- **THEN** the request emits an item_reference item for that ID rather than inline text

#### Scenario: Reconstructed assistant text retains phase
- **WHEN** store is false or an assistant text part has no stored item ID
- **THEN** the request emits string content, including an explicitly empty string
- **AND** phase is preserved when present without emitting an incomplete output message

## ADDED Requirements

### Requirement: Internal parallel function wrappers
The provider SHALL expand an undeclared function named parallel only when its
nonempty tool_uses array contains object parameters and functions-prefixed
recipients that are all declared function tools. Expansion SHALL be atomic and
preserve original wrapper identity and child index/count in provider metadata.
Streaming SHALL buffer wrapper input until it can emit child input lifecycles;
unexpandable wrappers SHALL retain their original input deltas and call identity.

#### Scenario: Valid wrapper expands
- **WHEN** a wrapper references two declared function tools
- **THEN** generate returns two tool calls and stream emits each child's start/delta/end/call sequence
- **AND** IDs are the wrapper call ID suffixed with the zero-based child index

#### Scenario: Declared or invalid wrapper stays a normal call
- **WHEN** parallel is itself a declared function or any nested recipient/parameters are invalid
- **THEN** the original function call is retained without partial child execution

#### Scenario: Wrapper input ends before its final item
- **WHEN** a suppressed parallel wrapper reaches EOF without a valid final item
- **THEN** its original start and buffered deltas are flushed in output-index order before any finish
- **AND** no completed tool call or input-end is invented

#### Scenario: Stateful scalar results are grouped
- **WHEN** every child result has matching wrapper metadata and unique indexes in a conversation or previous-response continuation
- **THEN** one wrapper output contains child outputs in index order
- **AND** conversations omit the existing wrapper call while previous-response chains reconstruct it
- **AND** incomplete or conflicting groups remain ordinary child results

### Requirement: Recoverable malformed Responses stream events
Malformed JSON SSE data SHALL emit a nonretryable stream error without discarding
subsequent decodable events. Transport/setup failures SHALL retain their existing
preflight retry/error contract. The provider SHALL emit at most one finish, after
flushing pending input. A malformed-frame error SHALL survive later completed or
incomplete responses while retaining their usage and metadata. SDK transport and
authentication SHALL remain in control, and each acquired framing decoder SHALL
be constructed and closed exactly once.

#### Scenario: Malformed events surround valid output
- **WHEN** malformed JSON occurs before and after valid tool or text events
- **THEN** errors and valid output retain their order through one HTTP request
- **AND** subsequent valid events remain visible
- **AND** the final finish reason is error, including when malformed data follows a completion event

#### Scenario: Custom SDK decoder owns framing resources
- **WHEN** a configured SDK decoder consumes a framing prefix or owns resources
- **THEN** one decoder consumes the stream and that same instance is closed on completion or cancellation

### Requirement: Apply-patch calls contribute tool finish reasons
Client-executed apply-patch calls SHALL contribute to tool-calls finish mapping in
both generate and completed stream calls.

#### Scenario: Completed patch call finishes
- **WHEN** a response contains a completed local apply-patch call
- **THEN** its unified finish reason is tool-calls rather than stop
