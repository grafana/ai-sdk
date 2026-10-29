# gateway-execution-runtime Specification

## Purpose
Provide one protocol-independent runtime that every Gateway API adapter uses to select and invoke a model, consume its stream, validate the provider output lifecycle, classify failures, capture attempts and apply host request policy. Each adapter then owns only its request mapping, representation, wire encoding, error documents and its own protocol state machine.

## Requirements

### Requirement: Shared adapter runtime
Every Gateway API adapter SHALL execute language-model requests through the shared execution runtime. The runtime SHALL NOT write HTTP responses, set status codes or headers, or encode wire documents. Adapters SHALL own representation, encoding, framing, response commitment, error documents and their own protocol state machine. The runtime SHALL own provider output lifecycle validation. The runtime SHALL NOT be a public Go package.

#### Scenario: Runtime outcomes are protocol-neutral
- **WHEN** a call fails or succeeds through the runtime
- **THEN** the adapter SHALL receive a typed outcome, a failure category and the execution metadata to encode, and no response bytes SHALL have been written by the runtime

#### Scenario: Two adapters over one runtime
- **WHEN** ProviderWire V4 and a second adapter are constructed from runtimes with the same selector and limits
- **THEN** neither SHALL require changes to the runtime, the selector or the other adapter

### Requirement: Runtime construction and limits
Runtime construction SHALL require a non-nil selector and positive stream idle and drain durations, and SHALL fail otherwise. Production SHALL use fixed defaults of 30 seconds idle and 1 second drain, which SHALL NOT be operator settings. The runtime SHALL NOT take a total duration or any request, response, frame or part-count limit.

#### Scenario: Invalid construction
- **WHEN** the selector is nil or a duration is zero or negative
- **THEN** construction SHALL fail before any request is served

#### Scenario: Settings do not construct a handler
- **WHEN** the plugin validates its settings
- **THEN** validation SHALL NOT construct a runtime or an adapter, and no setting SHALL change the idle or drain duration

### Requirement: Call-level host controls
At the start of a call, the runtime SHALL remove the call-level `gateway` provider-option namespace from the mapped options and pass its raw JSON to the selector, so it never reaches native models, middleware or telemetry. The runtime SHALL NOT reserve any other namespace, or `gateway` at any other scope.

#### Scenario: Gateway controls reach only the selector
- **WHEN** a request carries call-level `providerOptions.gateway`
- **THEN** the selector SHALL receive its raw JSON and the invoked model's options SHALL NOT contain a `gateway` namespace

#### Scenario: Other namespaces are ordinary
- **WHEN** a request carries `grafana` or `grafana-ai-sdk` at any scope, or `gateway` in a message, part or tool
- **THEN** the namespace SHALL reach the model unchanged at its scope, like any other namespace

#### Scenario: Configured selector refuses gateway controls
- **WHEN** configured selection receives call-level `gateway` controls
- **THEN** it SHALL apply `models`, `order` and `only` under gateway-request-routing, and SHALL fail any other control with the unsupported-gateway-control category, with zero model calls

### Requirement: Protected call headers
Before selection, the runtime SHALL refuse a call whose headers include a credential-bearing name, compared without case sensitivity, with the invalid-request category and the protected-call-header reason. The set SHALL include at least `authorization`, `proxy-authorization`, `x-access-token`, `x-grafana-id`, `x-api-key`, `api-key`, `openai-api-key`, `anthropic-api-key`, `openai-organization`, `openai-project`, `x-scope-orgid`, `x-cloud-org-id` and `x-access-policy-id`.

#### Scenario: Credential header in any case
- **WHEN** a call's headers include `X-ACCESS-TOKEN` or `Authorization`
- **THEN** the call SHALL fail with the protected-call-header reason, zero selections and zero model calls, and the outcome SHALL NOT carry the header name or value

#### Scenario: Ordinary header
- **WHEN** a call's headers include only non-credential names
- **THEN** they SHALL reach the model with their original case

### Requirement: Synchronous selection
The runtime SHALL select once per call, synchronously under the request context, and SHALL validate that the selection has a non-empty, valid UTF-8 identity and a non-nil V4 model. A panicking or invalid selection SHALL fail as internal. A context that has ended before or during selection SHALL decide the outcome, and a selection returned after it ended SHALL be discarded without invoking a model.

#### Scenario: Invalid or panicking selector
- **WHEN** the selector panics, or returns an empty identity, an invalid UTF-8 identity, a nil model or a model that is not V4
- **THEN** the call SHALL fail with the internal category and zero model calls

#### Scenario: Selection returns after the deadline
- **WHEN** the selector returns a valid selection after the request context's deadline expired
- **THEN** the call SHALL fail with the timeout category and zero model calls

#### Scenario: Selection error
- **WHEN** the selector returns an error
- **THEN** the call SHALL fail with the category that selection classification assigns to that error

### Requirement: Synchronous unary invocation
The runtime SHALL invoke `DoGenerate` once per unary call, synchronously under the request context. A panic or a `nil, nil` result SHALL fail as internal. A result or error returned after the request context ended SHALL be discarded and the context outcome reported. A model that ignores cancellation SHALL hold only its own request until it returns.

#### Scenario: Panicking model
- **WHEN** `DoGenerate` panics
- **THEN** the call SHALL fail with the internal category and other in-flight calls SHALL be unaffected

#### Scenario: Nil result
- **WHEN** `DoGenerate` returns no result and no error
- **THEN** the call SHALL fail with the internal category

#### Scenario: Late result is discarded
- **WHEN** `DoGenerate` returns a result after the request context was canceled or its deadline expired
- **THEN** the call SHALL fail with the cancellation or timeout category, and neither the result nor an attempt observed after the context ended SHALL be reported

### Requirement: Single-owner stream setup
The runtime SHALL invoke `DoStream` once per streaming call, synchronously under a model context derived from the request context. Before handing a stream to the adapter, it SHALL fail setup on a panic, an error, a missing result or a nil stream. The runtime SHALL cancel and drain every returned stream exactly once, including one returned with an error or after the request context ended.

#### Scenario: Setup failure with a stream
- **WHEN** `DoStream` returns both an error and a non-nil stream
- **THEN** setup SHALL fail with that error's category, and the stream SHALL be canceled and drained once without delaying the outcome

#### Scenario: Late stream
- **WHEN** `DoStream` returns a valid stream after the request context ended
- **THEN** setup SHALL fail with the context outcome and the stream SHALL be drained once

#### Scenario: Established stream
- **WHEN** `DoStream` returns a valid stream while the request context is live
- **THEN** the adapter SHALL receive it before any part is read, and SHALL decide when to commit its response

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

### Requirement: Bounded stream drain
Closing a stream SHALL cancel the model context and start one asynchronous drain that reads until the channel closes or the drain duration expires, whichever comes first. Closing more than once SHALL have no further effect. The drain SHALL NOT delay the adapter, and SHALL NOT count parts.

#### Scenario: Permanently ready stream
- **WHEN** a canceled stream channel stays ready with parts after close
- **THEN** the drain SHALL stop when the drain duration expires

#### Scenario: Provider closes within the drain
- **WHEN** the provider closes its channel within the drain duration
- **THEN** the drain SHALL consume the remaining parts and exit

#### Scenario: Repeated close
- **WHEN** an adapter closes a stream that was already closed
- **THEN** no second drain SHALL start

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options, protected call header, routing controls, no candidates or attempt bound. Model not found MAY carry the fallback-model reason. Classification SHALL never panic.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Routing selection errors
- **WHEN** selection fails with malformed routing controls, a filter that excludes every candidate, a composed chain above the attempt bound or an unknown fallback model
- **THEN** the category SHALL be invalid request with the routing-controls, no-candidates or attempt-bound reason respectively, or model not found with the fallback-model reason

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout, the transport category or upstream, and a status outside 100–599 SHALL be internal

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone, and every document other than the unsupported-gateway-control message and the new routing documents SHALL equal those written before this change

#### Scenario: Inconsistent tool history
- **WHEN** a call fails because its prompt tool history is inconsistent
- **THEN** selection classification SHALL return invalid request with no reason, before the provider-error fallback

#### Scenario: Invalid output
- **WHEN** a stream wait or a unary call reports invalid output
- **THEN** provider classification SHALL return internal

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. Configured and BYOK selections SHALL carry the attempt expected at each chain position, with its destination and the requested ID or `models` entry it belongs to, and SHALL publish an overview checked against them. Selections with no attribution SHALL NOT publish an overview.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and the response SHALL carry an overview with the requested selector, no canonical identity and those two native attempts in order

#### Scenario: Routed configured attempts
- **WHEN** a configured call routed across two models tries one candidate of each and the second is selected
- **THEN** the overview SHALL list both attempts with their configured identities and the requested model's canonical ID, and the routing summary SHALL group them by model

#### Scenario: Routed BYOK attempts
- **WHEN** a BYOK call routed across `anthropic/x` and `openai/y` observes an attempt whose native identity does not match the destination expected at its position
- **THEN** no overview SHALL be published

#### Scenario: Attempt after sealing
- **WHEN** an attempt is observed after the call's outcome was decided
- **THEN** it SHALL NOT appear in the overview

#### Scenario: Selection without attribution
- **WHEN** a selector returns a valid selection with neither configured nor request-account attribution and its model reports fallback attempts
- **THEN** the attempts SHALL be captured and sealed, and no overview SHALL appear in the response

#### Scenario: Test adapter sees BYOK attempts
- **WHEN** the test-only adapter and ProviderWire run the same BYOK fake-model scenarios
- **THEN** both SHALL report the same attempt view, including omitted canonical identity

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

### Requirement: Access-aware selection
One selector SHALL serve every access mode, so each adapter needs one runtime. It SHALL branch on the verified caller's account access. Configured access SHALL resolve through the catalog and apply routing controls under gateway-request-routing. BYOK access SHALL decode request accounts and routing controls from those controls. Any other access SHALL fail with the permission category.

#### Scenario: Configured access
- **WHEN** a configured caller calls a catalog model without `gateway` controls
- **THEN** the selector SHALL resolve it through the catalog and invoke only configured accounts

#### Scenario: Configured routed access
- **WHEN** a configured caller sends `models`, `order` or `only`
- **THEN** the selector SHALL resolve every named model through the catalog and compose only configured candidates

#### Scenario: BYOK access
- **WHEN** a BYOK caller sends valid `gateway.byok` accounts
- **THEN** the selector SHALL build the model from those accounts, with zero catalog resolutions

#### Scenario: No account access
- **WHEN** a request reaches the selector without configured or BYOK access
- **THEN** the call SHALL fail with the permission category, with zero catalog resolutions and zero provider requests

#### Scenario: BYOK account bound
- **WHEN** a BYOK request supplies more than 8 accounts for the selected provider
- **THEN** the call SHALL fail with the BYOK credentials category and the fixed BYOK 400 document, with zero provider requests

#### Scenario: BYOK disabled
- **WHEN** the Gateway runs in `self-hosted` mode
- **THEN** no caller SHALL reach BYOK selection

### Requirement: Per-request option translator
An adapter MAY supply one option translator with each call. The runtime SHALL make it available to every physical attempt of that call only. A call without a translator SHALL reach every attempt with unchanged options.

#### Scenario: No translator
- **WHEN** an adapter makes a call without a translator
- **THEN** every attempt SHALL receive the options the runtime passed to the model, unchanged

#### Scenario: Concurrent calls are isolated
- **WHEN** two concurrent calls on one runtime use different translators, or one uses none, over the same configured model or the same BYOK account shape
- **THEN** each attempt SHALL see only its own call's translation

### Requirement: Translation at the candidate boundary
The host SHALL wrap every configured candidate and every BYOK account once, when it is constructed. The wrapper SHALL sit directly outside that candidate's native option validation and inside fallback. Each attempt SHALL call the translator with its target and its own copy of the top-level option maps and slices. Every native option validation SHALL check the translated options.

#### Scenario: Heterogeneous fallback chain
- **WHEN** a configured model's Anthropic primary fails retryably and its OpenAI fallback succeeds, under a translator that writes target-specific options
- **THEN** the Anthropic request SHALL carry only the Anthropic translation, and the OpenAI request SHALL carry only the OpenAI translation

#### Scenario: BYOK accounts are translated per attempt
- **WHEN** a BYOK call's first account fails retryably and its second account succeeds, under a translator
- **THEN** each account's native request SHALL carry its own translation of the original options

#### Scenario: Translated Anthropic options are validated
- **WHEN** a translator produces Anthropic options on a configured or BYOK Anthropic candidate
- **THEN** native option validation SHALL accept valid translated options, and SHALL refuse invalid ones with the unsupported-request outcome and no provider request for that attempt

#### Scenario: Single-candidate model
- **WHEN** a configured model has no fallbacks
- **THEN** its only candidate SHALL still be translated

### Requirement: Host policy on translated options
The host SHALL re-check each attempt's translated options before the attempt reaches native option validation. Translated options that carry a call-level `gateway` namespace or a protected call header SHALL fail that attempt.

#### Scenario: Translator injects the reserved namespace
- **WHEN** a translator adds a call-level `gateway` namespace
- **THEN** the attempt SHALL fail before any provider contact

#### Scenario: Translator injects a protected header
- **WHEN** a translator adds `Authorization` or another protected call header, in any case
- **THEN** the attempt SHALL fail before any provider contact, and the outcome SHALL NOT carry the header name or value

### Requirement: Translation failures end the call
A translator panic, or translated options that fail host policy, SHALL fail that attempt as non-retryable: fallback SHALL NOT try later candidates, and the call SHALL fail with the internal category. An error the translator returns SHALL be an ordinary attempt failure, classified and retried as a native validation error would be.

#### Scenario: Panicking translator
- **WHEN** a translator panics on the first candidate of a two-candidate chain
- **THEN** the call SHALL fail with the internal category, with zero provider requests and no attempt on the second candidate, and other in-flight calls SHALL be unaffected

#### Scenario: Policy refusal does not fall back
- **WHEN** a translator injects a protected header for the first candidate only
- **THEN** the call SHALL fail with the internal category, with no attempt on the second candidate

#### Scenario: Translator error
- **WHEN** a translator returns the unsupported-request error for the first candidate and translates the second
- **THEN** the first attempt SHALL fail with no provider request, and the second candidate SHALL be attempted

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
