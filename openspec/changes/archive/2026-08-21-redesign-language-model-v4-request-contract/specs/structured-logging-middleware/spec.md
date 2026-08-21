## MODIFIED Requirements

### Requirement: Privacy-first capture policy

By default, the logger SHALL NOT log prompt/message content, generated text, reasoning text, tool inputs, tool outputs, file data, raw chunks, request bodies, response bodies, headers, provider options, provider metadata, opaque error messages, or file names.

By default, the logger MAY log safe scalar summaries and counts, including provider/model identity, transport identity when a routed backend differs, duration, outcome, success/failure, max output tokens, temperature, top-p, top-k, seed, reasoning effort value, stop sequence count, tool count, response format type, usage totals/subfields, finish reason, warning count/types, response metadata, stream part counts, and stream time to first content.

Sensitive fields SHALL only become eligible for logging when the corresponding `CaptureOptions` flag is enabled. Captured string and JSON payload attributes SHALL be bounded by `MaxStringLen`, `MaxJSONBytes`, or documented finite package defaults when those fields are zero.

When media capture is disabled and a captured or inspected `provider.ContentPart` is sanitized, the logger SHALL clear both `FilePartFilename` and `Filename` together with file data. Clearing only one filename field SHALL be a privacy defect because request and generated response/source values use different ownership fields.

#### Scenario: Default logging omits sensitive request fields

- **WHEN** a request includes a secret in the prompt, headers, provider options, tool input, request filename, and request body
- **AND** the logger is configured with zero-value `CaptureOptions`
- **THEN** no emitted log record SHALL contain that secret
- **AND** emitted records MAY include counts and scalar settings for the request

#### Scenario: Media redaction clears both filename fields

- **WHEN** media capture is disabled and content contains a secret in `FilePartFilename`, `Filename`, or both
- **THEN** logger sanitization SHALL clear both fields
- **AND** neither value SHALL appear in emitted structured payloads

#### Scenario: Capture options opt in to payload attrs

- **WHEN** `Capture.Inputs`, `Capture.Headers`, and `Capture.ProviderOptions` are enabled
- **THEN** prompt, header, and provider option attributes MAY be emitted
- **AND** those attributes SHALL still pass through the configured redactor before logging

#### Scenario: Metadata-only errors omit opaque messages

- **WHEN** a unary error, stream-open error, streamed error part, context cancellation, or timeout contains an opaque message
- **AND** `Capture.ErrorMessages` is false
- **THEN** no emitted record SHALL contain the opaque message
- **AND** error class/type, HTTP status, retryability, operation, outcome, timing, and model identity metadata SHALL remain available when applicable

#### Scenario: Error message capture is opt-in

- **WHEN** `Capture.ErrorMessages` is true
- **THEN** the bounded opaque error message MAY be emitted
- **AND** the message SHALL still pass through the configured redactor before logging

#### Scenario: Captured payloads are bounded

- **WHEN** a captured prompt or JSON payload exceeds the configured capture limit
- **THEN** the logged attribute value SHALL be summarized to stay within the configured or default bound
- **AND** the model call SHALL continue unchanged
