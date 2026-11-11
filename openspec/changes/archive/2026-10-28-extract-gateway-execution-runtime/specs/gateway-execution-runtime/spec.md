# Spec Delta

## Purpose

Provide one protocol-independent runtime that every Gateway API adapter uses to select and invoke a model, consume its stream, classify failures, capture attempts and apply host request policy. Each adapter then owns only its request mapping, wire encoding, error documents and protocol state machine.

## ADDED Requirements

### Requirement: Shared adapter runtime
Every Gateway API adapter SHALL execute language-model requests through the shared execution runtime. The runtime SHALL NOT write HTTP responses, set status codes or headers, or encode wire documents. Adapters SHALL own response commitment, framing, error documents and their protocol state machine. The runtime SHALL NOT be a public Go package.

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
- **THEN** the call SHALL fail with the invalid-request category and the reserved-provider-options reason, with zero model calls

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
Each wait for a stream part SHALL return exactly one outcome: a part, closed, canceled, deadline or idle timeout. Deadline and canceled SHALL follow the request context's error. The idle duration SHALL start when setup returns the stream and restart at each later wait, after the adapter has represented the previous part. Every outcome other than a part SHALL cancel the model context before it is returned.

#### Scenario: Each outcome
- **WHEN** a part arrives, the stream closes, the request is canceled, the deadline expires or no part arrives within the idle duration
- **THEN** the wait SHALL return part, closed, canceled, deadline or idle timeout respectively

#### Scenario: Activity resets idle time
- **WHEN** represented parts arrive within each idle interval for longer than one idle duration in total
- **THEN** no idle timeout SHALL occur

#### Scenario: Context ends while waiting
- **WHEN** the request context ends while a wait is pending
- **THEN** the model context SHALL be canceled before the outcome is returned

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

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. Configured calls SHALL publish the same execution overview as before. BYOK attempts SHALL be captured in order but SHALL NOT be published.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and no overview SHALL appear in the response

#### Scenario: Attempt after sealing
- **WHEN** an attempt is observed after the call's outcome was decided
- **THEN** it SHALL NOT appear in the overview

### Requirement: Test-only adapter evidence
Tests SHALL include a minimal adapter, not ProviderWire, built only on the runtime. For identical fake models, it SHALL show the same deadline, cancellation, drain, classification, policy and attempt-view outcomes as ProviderWire. It SHALL NOT be shipped.

#### Scenario: Matching outcomes
- **WHEN** the same fake model scenarios run through the test adapter and ProviderWire
- **THEN** both SHALL report the same category, reason, model-call count, drains and attempt view for every scenario

### Requirement: Runtime ownership documentation
The execution package godoc SHALL state what the runtime owns and what each adapter owns. The Gateway architecture walkthrough and glossary SHALL name the runtime as the owner of stream idle time and drain.

#### Scenario: Ownership is documented
- **WHEN** a developer reads the execution package godoc or the architecture walkthrough
- **THEN** they SHALL find the runtime's responsibilities, the adapter's responsibilities, and the owner of the idle and drain limits
