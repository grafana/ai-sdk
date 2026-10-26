## ADDED Requirements

### Requirement: Platform-owned request size
The Gateway SHALL NOT apply its own request-body byte bound or offer a request-size setting. The plugin host's route message size SHALL bound each request, body included, before any Gateway code runs. A refused caller's body SHALL NOT be read. Settings named `max_request_bytes` or `max_model_duration` SHALL be ignored, whatever their value.

#### Scenario: Request above the former bound
- **WHEN** an authorized language-model request with an inline file is above 1 MiB and below the host route message size
- **THEN** it SHALL reach mapping and selection and succeed like any other valid request

#### Scenario: Request above the host message size
- **WHEN** a request exceeds the host route message size
- **THEN** the host SHALL reject it in its own error format, with zero selection and zero model calls, and the Gateway SHALL NOT be required to produce a ProviderWire error document

#### Scenario: Refused caller
- **WHEN** access policy refuses a caller on a body-carrying route
- **THEN** the permission document SHALL be written without reading the request body

#### Scenario: Removed settings are ignored
- **WHEN** an operator sets `max_request_bytes` or `max_model_duration`, including to a non-numeric or non-positive value
- **THEN** startup SHALL succeed and the value SHALL have no effect on request size or duration

### Requirement: Shared provider attempt bound
One fixed bound of 8 provider attempts per request SHALL limit both configured chains and BYOK accounts. A configured model's primary plus fallbacks SHALL be at most 8, checked at configuration load. A BYOK request's selected-provider accounts SHALL be at most 8, checked when the request is decoded. The bound SHALL NOT be an operator setting.

#### Scenario: Configured chain at and above the bound
- **WHEN** a configured model has a primary and 7 fallbacks, or a primary and 8 fallbacks
- **THEN** the first SHALL load and the second SHALL fail configuration loading with the existing secret-safe cardinality error

#### Scenario: BYOK accounts at and above the bound
- **WHEN** the selected provider's BYOK array has 8 accounts, or 9 accounts
- **THEN** the first SHALL be accepted and the second SHALL receive the fixed BYOK credentials 400 with zero native calls

#### Scenario: Unused BYOK provider is not counted
- **WHEN** an unused provider's BYOK array has 9 or more well-formed accounts
- **THEN** the request SHALL be accepted, and those accounts SHALL NOT count toward the bound

## MODIFIED Requirements

### Requirement: Outbound model transport bounds
Every Gateway-constructed model, including each configured provider type and each BYOK account, SHALL use one shared model HTTP client. That client SHALL reject redirects and wait at most 10 seconds for response headers. It SHALL fail a response whose media type is not `text/event-stream` once its decompressed body exceeds 16 MiB, and a `text/event-stream` response once a single event exceeds 16 MiB after decompression. An event SHALL be the bytes between SSE event boundaries, with LF, CRLF or CR line endings. None of these bounds SHALL be an operator setting.

#### Scenario: Decompressed bytes are counted
- **WHEN** a provider returns a gzip-encoded body under 16 MiB compressed whose decompressed size exceeds 16 MiB
- **THEN** the read SHALL fail with the transport's response-too-large error once the decompressed bytes exceed the bound

#### Scenario: Each request has its own bound
- **WHEN** two sequential requests through the shared client each return just under 16 MiB
- **THEN** both SHALL succeed, because the bound applies to each response independently and not to the client or the logical call

#### Scenario: Response exceeds the bound
- **WHEN** a non-SSE provider response exceeds 16 MiB after decompression
- **THEN** the call SHALL fail through the existing fallback and safe-error classification without returning a partial result

#### Scenario: Long stream within the event bound
- **WHEN** a `text/event-stream` response totals more than 16 MiB and every event is within 16 MiB
- **THEN** the stream SHALL be read to its end without a response-too-large error

#### Scenario: Single event exceeds the bound
- **WHEN** one event of a `text/event-stream` response exceeds 16 MiB after decompression
- **THEN** the read SHALL fail with the transport's response-too-large error, which reaches the caller through the existing transport failure path

#### Scenario: Event boundaries for every line ending
- **WHEN** a `text/event-stream` response separates events with blank lines using LF, CRLF or CR line endings, including a CRLF split across reads
- **THEN** each event SHALL be counted from zero, and a stream of events that are each just under 16 MiB SHALL succeed for every line ending

### Requirement: Execution time bounds
The language-model handler SHALL bound selection and the logical model call by the request context's deadline, which the platform's route deadline sets, and SHALL add no total duration of its own. It SHALL bound each established stream by a fixed 30-second idle duration between represented parts, and each post-terminal drain by a fixed 1-second drain duration. Drain SHALL be bounded by its duration alone, without a part count.

#### Scenario: Request deadline ends the call
- **WHEN** the request context's deadline expires during selection, an attempt or stream consumption
- **THEN** the model context SHALL be canceled, no later attempt SHALL begin, and the handler SHALL write an error before commitment or a terminal event after it, if the writer is still usable
- **AND** its category SHALL follow the request context's error: timeout for `DeadlineExceeded`, cancellation for `Canceled`

#### Scenario: No platform deadline
- **WHEN** the request context carries no deadline
- **THEN** the handler SHALL apply no total deadline, and the call SHALL end only at completion, caller cancellation, an attempt's response-header timeout or the stream idle duration
- **AND** a unary Anthropic attempt whose token budget the Anthropic SDK estimates above 10 minutes SHALL fail before any native request, as `anthropic-unary-context-deadline` specifies for a call without a deadline

#### Scenario: Continuously ready provider during drain
- **WHEN** a canceled provider channel stays ready with parts after terminal handling
- **THEN** the drain SHALL stop when the drain duration expires, regardless of how many parts it consumed

#### Scenario: Many provider parts within the deadlines
- **WHEN** a provider emits more than 100,000 valid parts before finish within the request deadline and idle duration
- **THEN** every part SHALL be represented in order and the finish SHALL be written

### Requirement: Adapters own no size accounting
A Gateway API adapter SHALL NOT reject, omit or truncate a request-valid or output-valid value because of its size, cardinality or encoded length. Size SHALL be bounded only by the platform request size, the outbound transport, the execution time bounds and the plugin host.

#### Scenario: Large outputs succeed
- **WHEN** a provider returns any of the following, each valid and within the outbound bound:
  - a unary result whose encoded document exceeds 8 MiB;
  - a stream event whose complete frame exceeds 1 MiB;
  - a start with more warnings than a 1 MiB frame could hold;
  - raw usage above 1,048,576 bytes;
- **THEN** the Gateway SHALL return it complete and successful

#### Scenario: Large failure documents succeed
- **WHEN** an HTTP or SSE failure carries an execution overview and native summaries whose encoded document exceeds 64 KiB
- **THEN** the complete failure document SHALL be written with its overview and summaries

#### Scenario: Validity still fails
- **WHEN** an output of any size contains invalid UTF-8, an unknown discriminator, a non-object metadata namespace, non-object raw usage or an unsafe token count
- **THEN** the existing safe error behavior SHALL apply, without partial output

### Requirement: Documented limit ownership
The "Limits" section of `ai-gateway/README.md` SHALL list each retained Gateway limit with exactly one owning boundary, and each platform limit with its source. It SHALL also list each removed limit with the bound that replaces it, and the trade-offs of this placement.

#### Scenario: Ledger content
- **WHEN** an operator reads the Limits section
- **THEN** it SHALL name the Gateway owner of the provider attempt bound, the outbound response and event bytes, the header timeout and redirect rejection, and the idle and drain durations
- **AND** it SHALL name the platform as the owner of the request deadline, the request message size, admission, concurrency and process memory, with the source of each
- **AND** it SHALL list the removed `max_request_bytes` and `max_model_duration` settings, the former per-response stream bound, and the former unary, frame, part-count, raw-usage, error-document and summary-source limits, with their replacements
- **AND** it SHALL state what bounds a call when the platform deadline is disabled, that unary Anthropic calls with a large token budget then fail unless they set `maxOutputTokens` or stream, and that a platform deadline appears as a cancellation in Gateway telemetry
- **AND** it SHALL state that per-request memory grows with response size, JSON encoding expansion and concurrency, and recommend setting `GOMEMLIMIT` to about 85–90% of the container memory limit

### Requirement: Large-output client evidence
Automated tests SHALL prove that both registered clients (the pinned TypeScript Gateway client and the Go `providers/grafana` client) send large requests and consume large outputs through the real plugin route host. They SHALL NOT add or modify recorded provider fixtures.

#### Scenario: Both clients through the test host
- **WHEN** a fake native provider behind the built plugin returns a unary result above 8 MiB, a stream frame above 1 MiB and raw usage above 1 MiB, each within the outbound bound
- **THEN** both clients SHALL receive the complete values in unary and streaming mode, and no file under `test/conformance/*/recorded/` SHALL change

#### Scenario: Long stream through the test host
- **WHEN** a fake native provider behind the built plugin streams more than 16 MiB in total, with every event within 16 MiB
- **THEN** both clients SHALL receive the complete stream and its finish

#### Scenario: Large request through the test host
- **WHEN** a client sends a request above 1 MiB with an inline file through the built plugin
- **THEN** the fake native provider SHALL receive the file and the call SHALL succeed

#### Scenario: Oversized request through the test host
- **WHEN** a client sends a request above the plugin's route message size through the built plugin
- **THEN** the request SHALL fail before selection, with zero native calls

## REMOVED Requirements

### Requirement: Route-owned request-body bound
**Reason**: The plugin host already receives each route request as one gRPC message and caps it at its route message size, before Gateway code runs. The 1 MiB `max_request_bytes` was a stricter duplicate that rejected valid inline file inputs. #518's design D1 is reversed; see this change's design D2.
**Migration**: Remove `max_request_bytes` from plugin configuration; any value is ignored. Size requests within the host route message size. Hosts that serve the ProviderWire handler outside the plugin host bound the body themselves, as the ProviderWire testserver does.
