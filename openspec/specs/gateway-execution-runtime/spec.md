# gateway-execution-runtime Specification

## Purpose
Provide one protocol-independent runtime that every Gateway API adapter uses to select and invoke a model, consume its stream, classify failures, capture attempts and apply host request policy. Each adapter then owns only its request mapping, wire encoding, error documents and protocol state machine.

## Requirements

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
The runtime SHALL classify every failure into one closed category: invalid request, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, provider stream, timeout, cancellation or internal. Provider stream SHALL be produced only for in-band stream error parts. Invalid request MAY carry a closed reason: reserved provider options or protected call header. Classification SHALL never panic.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal
- **AND** a provider error with no status that is not cancellation, timeout or a transport error SHALL otherwise remain upstream and SHALL never be provider stream

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout or the transport category when it wraps the corresponding cause, and provider stream otherwise
- **AND** a status outside 100–599 SHALL be internal, and a valid status SHALL use the same status mapping as provider errors

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone
- **AND** every category other than provider stream SHALL keep the document written before this change, and provider stream SHALL use the fixed status-less stream document defined by providerwire-v4-streaming-runtime

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. Configured calls SHALL publish the same execution overview as before. BYOK calls SHALL publish their captured attempts with request-account attribution. Selections with neither attribution SHALL NOT publish an overview.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and the response SHALL carry an overview with the requested selector, no canonical identity and those two native attempts in order

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
Tests SHALL include a minimal HTTP adapter, not ProviderWire, built only on the runtime and the host contract, with its own response and host-error format. For identical fake models, it SHALL show the same deadline, cancellation, drain, classification, policy and attempt-view outcomes as ProviderWire. It SHALL NOT be shipped.

#### Scenario: Matching outcomes
- **WHEN** the same fake model scenarios run through the test adapter and ProviderWire
- **THEN** both SHALL report the same category, reason, model-call count, drains and attempt view for every scenario

#### Scenario: Translation through the test adapter
- **WHEN** the test adapter supplies a translator on a call over a fallback model
- **THEN** each attempt SHALL see the translation for its own target, as specified for per-attempt translation

### Requirement: Runtime ownership documentation
The execution package godoc SHALL state what the runtime owns and what each adapter owns. The Gateway architecture walkthrough and glossary SHALL name the runtime as the owner of stream idle time and drain.

#### Scenario: Ownership is documented
- **WHEN** a developer reads the execution package godoc or the architecture walkthrough
- **THEN** they SHALL find the runtime's responsibilities, the adapter's responsibilities, and the owner of the idle and drain limits

### Requirement: Access-aware selection
One selector SHALL serve every access mode, so each adapter needs one runtime. It SHALL branch on the verified caller's account access. Configured access SHALL resolve through the catalog and refuse call-level `gateway` controls. BYOK access SHALL decode request accounts from those controls. Any other access SHALL fail with the permission category.

#### Scenario: Configured access
- **WHEN** a configured caller calls a catalog model without `gateway` controls
- **THEN** the selector SHALL resolve it through the catalog and invoke only configured accounts

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
