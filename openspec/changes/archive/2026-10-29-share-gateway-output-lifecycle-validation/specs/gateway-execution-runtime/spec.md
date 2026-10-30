# Spec Delta

## ADDED Requirements

### Requirement: Prompt tool history reconstruction
As the first step of a call, before host policy, the context check and selection, the runtime SHALL rebuild tool history from the prompt. It SHALL fail inconsistent history as an invalid request with no reason, zero selections and zero model calls. The pending provider-executed calls, by ID and tool name, SHALL seed stream and unary validation. History SHALL NOT change after it is rebuilt; each validator and the call's accessor SHALL use a copy.

#### Scenario: Deferred result
- **WHEN** the prompt holds a provider-executed tool call without a result
- **THEN** the call SHALL proceed, and that call SHALL be pending with its tool name

#### Scenario: Orphan result
- **WHEN** the prompt holds a tool result with no earlier call of the same ID
- **THEN** the call SHALL proceed, and a later prompt call with that ID SHALL fail the call as an invalid request

#### Scenario: Conflicting duplicates
- **WHEN** a prompt call ID repeats after its result, a second time, with another tool name, or where either call is not provider-executed, or a result for a known call names another tool or repeats
- **THEN** the call SHALL fail as an invalid request with zero selections and zero model calls

#### Scenario: Approval re-emission
- **WHEN** a pending provider-executed call is re-emitted once with the same name in a later assistant turn, with or without its result
- **THEN** the call SHALL proceed, and the call SHALL be pending only if no result followed

#### Scenario: History is not shared
- **WHEN** two validators are seeded from one rebuilt history and one of them accepts a result for a pending call
- **THEN** the other validator SHALL still treat that call as pending, and the history and the call's copy SHALL be unchanged

#### Scenario: History precedes policy and context
- **WHEN** a call has inconsistent history together with a protected call header, or its context has already ended
- **THEN** the call SHALL fail as an invalid request with no reason

#### Scenario: ProviderWire document
- **WHEN** ProviderWire receives inconsistent history
- **THEN** it SHALL write the same fixed 400 invalid-request document as before this change

### Requirement: Shared stream output validation
The runtime SHALL validate each provider part before an adapter receives it. The rules are: the start position, response segments, text and reasoning IDs, the tool input, call, preview, result and approval phases (including calls seeded from history), finish preconditions and a registered finish reason. A violation SHALL end the stream with the invalid-output outcome. The rules SHALL be those ProviderWire enforced before this change, including response segments.

#### Scenario: Start position
- **WHEN** a `stream-start` part arrives after any other part
- **THEN** the wait SHALL return invalid output, and a first `stream-start` SHALL be accepted

#### Scenario: Response segments
- **WHEN** a `response-metadata` or `finish` part arrives while text, reasoning or tool input is open or a preview is pending, in the stream or in history
- **THEN** the wait SHALL return invalid output, while an accepted `response-metadata` SHALL open a new segment, at the start of the stream, after content or after a `finish`

#### Scenario: Parts after a finish
- **WHEN** a part other than `response-metadata`, raw or provider error arrives after an accepted `finish`, including a second `finish`
- **THEN** the wait SHALL return invalid output

#### Scenario: Text and reasoning IDs
- **WHEN** a text start has an empty ID, overlaps an active text or reuses an ID within its segment, a text delta or end names an inactive ID, a reasoning start has an empty ID or reuses one within its segment, or a reasoning delta or end names an inactive ID
- **THEN** the wait SHALL return invalid output, while concurrent reasoning blocks with distinct IDs, and text or reasoning IDs reused in a later segment, SHALL be accepted

#### Scenario: Tool phases
- **WHEN** a tool part has an empty ID or a missing tool name, skips or repeats a phase, changes the tool name, reuses a tool ID in any segment, starts input for an ID seeded from history in any phase, carries a forbidden provider-executed, dynamic or preliminary flag, re-emits a history call without the provider-executed flag or a second time, or has an empty, invalid or `null` result
- **THEN** the wait SHALL return invalid output

#### Scenario: Results across segments and from history
- **WHEN** a stream emits the result of a call from an earlier segment, or of a call pending in history, with the same tool name
- **THEN** the part SHALL be accepted

#### Scenario: Approvals
- **WHEN** an approval names a call that is not a provider-executed call awaiting its first result, or one already approved, has an empty approval ID or reuses one anywhere in the stream, is provider-executed, or carries any dynamic or preliminary value, even false
- **THEN** the wait SHALL return invalid output

#### Scenario: Finish preconditions
- **WHEN** a finish carries warnings, lacks a finish reason or usage, or has an unregistered unified finish reason
- **THEN** the wait SHALL return invalid output

#### Scenario: Unknown part type
- **WHEN** a part has a type that is not a LanguageModelV4 stream part
- **THEN** the wait SHALL return invalid output

#### Scenario: Pass-through parts
- **WHEN** an error or raw part arrives, in a segment or after a `finish`
- **THEN** the runtime SHALL deliver it without changing segment, text, reasoning or tool state, and it SHALL still consume the start position, so a later `stream-start` SHALL return invalid output

### Requirement: Invalid output outcome
On invalid output in a stream, the runtime SHALL cancel the model context and start the stream's one drain before returning. Its error SHALL be classified as internal. The attempt view for invalid output, streamed or unary, SHALL equal the view of a successful call. The violating part SHALL NOT be delivered to the adapter.

#### Scenario: Drain on invalid output
- **WHEN** a stream part violates a lifecycle rule and the provider keeps sending parts
- **THEN** the model context SHALL be canceled, one drain SHALL start, and a later close SHALL start no second drain

#### Scenario: Classification and view
- **WHEN** an adapter classifies invalid output and projects its view
- **THEN** the category SHALL be internal, and a single-candidate overview SHALL report the selected attempt

#### Scenario: ProviderWire output is unchanged
- **WHEN** ProviderWire receives invalid output as the first part, as a later part or after a written `finish`
- **THEN** it SHALL write the same frames as before this change, including the synthetic `stream-start`, any written `finish`, and one internal terminal error with the same execution metadata

### Requirement: Stream end decided by the runtime
The runtime SHALL decide whether a provider close ends a stream cleanly. A close after an accepted `finish`, with no lifecycle part since, SHALL return the finished outcome, a terminal success with no error that adapters SHALL NOT classify as a failure. Any other close SHALL return the closed outcome. Idle timeout, deadline and cancellation SHALL keep their outcomes after a `finish`.

#### Scenario: Close after finish
- **WHEN** the provider closes the stream after a valid `finish`, with only raw or provider error parts after it
- **THEN** the wait SHALL return finished, and the call SHALL be reported as a success

#### Scenario: Premature close
- **WHEN** the provider closes the stream before any `finish`, or after a `response-metadata` that followed the last `finish`
- **THEN** the wait SHALL return closed

#### Scenario: Stall after finish
- **WHEN** the idle duration expires, the deadline passes or the request is canceled after an accepted `finish` and before the provider closes
- **THEN** the wait SHALL return idle timeout, deadline or canceled respectively, as before this change

#### Scenario: ProviderWire end of stream
- **WHEN** ProviderWire's stream ends with finished, closed or a context outcome
- **THEN** it SHALL write the same frames as before this change: nothing more after finished, and its terminal error otherwise

### Requirement: Shared unary output validation
Before returning a unary result, the runtime SHALL check call uniqueness and result correlation (both including history), approvals, previews left pending at the end, a registered unified finish reason and the flags on non-tool parts. It SHALL validate only a non-nil result returned with no error. A violation SHALL fail the call with the invalid-output error. The rules SHALL be those ProviderWire enforced before this change.

#### Scenario: Call shape and uniqueness
- **WHEN** a tool call has an empty ID or name or is preliminary, or repeats an ID other than as one provider-executed re-emission, with the same name, of a history call that has no preview and no final result
- **THEN** the call SHALL fail with invalid output

#### Scenario: Result correlation
- **WHEN** a result names an unknown call, another tool name or a completed call, or its value is empty, invalid JSON or `null`
- **THEN** the call SHALL fail with invalid output, while a result for a call pending in history SHALL be accepted

#### Scenario: Approvals and previews
- **WHEN** an approval names an unknown, non-provider-executed, previewed, completed or already-approved call, or has an empty or reused approval ID, or a preview has no final result by the end of the content
- **THEN** the call SHALL fail with invalid output

#### Scenario: Finish reason and flags
- **WHEN** the unified finish reason is not registered, a content part has an unknown type, or a non-tool part is provider-executed, dynamic or preliminary
- **THEN** the call SHALL fail with invalid output

#### Scenario: ProviderWire document
- **WHEN** ProviderWire receives invalid unary output
- **THEN** it SHALL write the same 500 internal-error document as before this change, with the same execution metadata

### Requirement: Adapter representation checks
Adapters SHALL NOT own output lifecycle rules. They own checks about their own representation and MAY keep content-validity checks that are not lifecycle rules. An adapter that refuses a part as invalid SHALL end the stream. It MAY omit parts it does not represent only when they carry no lifecycle state beyond the start position. An adapter that evaluates lifecycle rules internally SHALL call the runtime's rules with its own state, seeded from a copy of the history.

#### Scenario: Representation rejection
- **WHEN** a part passes lifecycle validation but ProviderWire cannot represent it, such as invalid UTF-8, malformed metadata or a token count outside the JavaScript safe-integer range
- **THEN** ProviderWire SHALL reject that part with the same internal terminal error as before this change

#### Scenario: Content-validity checks stay in ProviderWire
- **WHEN** a part passes lifecycle validation but fails ProviderWire's source, custom-kind, file-data, non-negative token-count, response-identity or recovered-panic check
- **THEN** ProviderWire SHALL reject it with the same internal terminal error or 500 document as before this change

#### Scenario: Omitted raw parts
- **WHEN** ProviderWire receives a raw part that was not requested or that it cannot project
- **THEN** it SHALL omit the part and keep streaming, as before this change

#### Scenario: Agreement at the same part
- **WHEN** the same parts or unary result are checked by the runtime and by ProviderWire's internal state machine and mapper
- **THEN** both SHALL reject at the same part, for every rule class

## MODIFIED Requirements

### Requirement: Shared adapter runtime
Every Gateway API adapter SHALL execute language-model requests through the shared execution runtime. The runtime SHALL NOT write HTTP responses, set status codes or headers, or encode wire documents. Adapters SHALL own representation, encoding, framing, response commitment, error documents and their own protocol state machine. The runtime SHALL own provider output lifecycle validation. The runtime SHALL NOT be a public Go package.

#### Scenario: Runtime outcomes are protocol-neutral
- **WHEN** a call fails or succeeds through the runtime
- **THEN** the adapter SHALL receive a typed outcome, a failure category and the execution metadata to encode, and no response bytes SHALL have been written by the runtime

#### Scenario: Two adapters over one runtime
- **WHEN** ProviderWire V4 and a second adapter are constructed from runtimes with the same selector and limits
- **THEN** neither SHALL require changes to the runtime, the selector or the other adapter

### Requirement: Stream wait outcomes
Each wait for a stream part SHALL return exactly one outcome: a part, closed, canceled, deadline, idle timeout, invalid output or finished. Deadline and canceled SHALL follow the request context's error. The idle duration SHALL start when setup returns the stream and restart at each later wait, after the adapter has represented the previous part. Every outcome other than a part SHALL cancel the model context before it is returned. Invalid output and finished SHALL be sticky.

#### Scenario: Each outcome
- **WHEN** a part arrives, the stream closes without a finished segment, the request is canceled, the deadline expires, no part arrives within the idle duration, a part violates a lifecycle rule or the stream closes after an accepted finish
- **THEN** the wait SHALL return part, closed, canceled, deadline, idle timeout, invalid output or finished respectively

#### Scenario: Activity resets idle time
- **WHEN** represented parts arrive within each idle interval for longer than one idle duration in total
- **THEN** no idle timeout SHALL occur

#### Scenario: Context ends while waiting
- **WHEN** the request context ends while a wait is pending
- **THEN** the model context SHALL be canceled before the outcome is returned

#### Scenario: Sticky terminal outcomes
- **WHEN** an adapter waits again after invalid output, while the provider keeps sending parts, or after finished
- **THEN** the wait SHALL return the same outcome without reading from the provider

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options or protected call header. Classification SHALL never panic.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout, the transport category or upstream, and a status outside 100–599 SHALL be internal

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone, and the documents SHALL equal those written before this change

#### Scenario: Inconsistent tool history
- **WHEN** a call fails because its prompt tool history is inconsistent
- **THEN** selection classification SHALL return invalid request with no reason, before the provider-error fallback

#### Scenario: Invalid output
- **WHEN** a stream wait or a unary call reports invalid output
- **THEN** provider classification SHALL return internal

### Requirement: Test-only adapter evidence
Tests SHALL include a minimal HTTP adapter, not ProviderWire, built only on the runtime and the host contract, with its own response and host-error format. For identical fake models, it SHALL show the same deadline, cancellation, drain, classification, policy, attempt-view and invalid-output outcomes as ProviderWire, with invalid output at the same part. It SHALL NOT be shipped.

#### Scenario: Matching outcomes
- **WHEN** the same fake model scenarios run through the test adapter and ProviderWire
- **THEN** both SHALL report the same category, reason, model-call count, drains and attempt view for every scenario

#### Scenario: Invalid output at the same part
- **WHEN** the same fake model emits a part that violates a stream lifecycle rule of any class, or returns a unary result that violates a unary rule
- **THEN** the test adapter SHALL receive invalid output after as many accepted parts as the provider parts ProviderWire processed before its internal terminal error, or SHALL receive the unary invalid-output error

#### Scenario: Stream end through the test adapter
- **WHEN** the fake model closes after a finish, closes after a later response-metadata, or stalls after a finish until the request is canceled
- **THEN** the test adapter SHALL report success, an internal failure or cancellation respectively, as ProviderWire does

#### Scenario: Inconsistent history through the test adapter
- **WHEN** the test adapter makes a call whose prompt history is inconsistent
- **THEN** it SHALL receive the invalid-request category with zero model calls, as ProviderWire does

#### Scenario: Translation through the test adapter
- **WHEN** the test adapter supplies a translator on a call over a fallback model
- **THEN** each attempt SHALL see the translation for its own target, as specified for per-attempt translation

### Requirement: Runtime ownership documentation
The execution package godoc SHALL state what the runtime owns and what each adapter owns, including that the runtime owns tool-history reconstruction, output lifecycle validation and the clean-close decision while adapters own representation checks. The Gateway architecture walkthrough and glossary SHALL name the runtime as the owner of stream idle time, drain and output lifecycle validation.

#### Scenario: Ownership is documented
- **WHEN** a developer reads the execution package godoc or the architecture walkthrough
- **THEN** they SHALL find the runtime's responsibilities, the adapter's responsibilities, and the owner of the idle and drain limits and of output lifecycle validation
