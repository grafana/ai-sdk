# Spec Delta

## Purpose

Define the protocol-independent execution runtime every Gateway API adapter uses to select a logical model, run it under shared deadlines, own its stream, classify failures and observe attempts, so adapters own only their wire contract.

## ADDED Requirements

### Requirement: One execution runtime for every adapter
The Gateway SHALL provide one protocol-independent execution runtime that every API adapter uses for model selection, invocation, stream lifecycle, failure classification and attempt observation. Adapters SHALL own only request decoding and mapping, wire encoding, error documents and their protocol state machine. Adapters SHALL NOT reimplement selection, deadlines, invocation workers, stream draining or provider-error classification.

#### Scenario: Adapter composes with the runtime
- **WHEN** a test drives a minimal non-ProviderWire handler that only decodes a model ID and writes plain responses through the runtime
- **THEN** it SHALL get the same selection, deadline, cancellation, drain, classification and attempt behavior as ProviderWire, without adapter-local copies

#### Scenario: ProviderWire uses the runtime
- **WHEN** ProviderWire serves unary and streaming requests
- **THEN** its observable behavior SHALL be unchanged by moving those capabilities into the runtime

### Requirement: Host request policy at the runtime boundary
Before selection, the runtime SHALL apply host request policy to every adapter's mapped call options. It SHALL pass the call-level `gateway` provider-option namespace to selection as host controls, never to a model. It SHALL refuse the reserved `grafana` and `grafana-ai-sdk` namespaces and credential-bearing call headers as invalid requests with a closed policy reason. Adapters SHALL NOT apply this policy themselves.

#### Scenario: Gateway controls reach selection
- **WHEN** an adapter maps a request whose call-level provider options contain a `gateway` namespace
- **THEN** selection SHALL receive that namespace as host controls and no model SHALL receive it as a provider option

#### Scenario: Reserved namespace or protected header
- **WHEN** mapped call options contain a `grafana` or `grafana-ai-sdk` namespace, or a credential-bearing header such as `Authorization`, `X-Api-Key` or `X-Access-Token`
- **THEN** the runtime SHALL return an invalid-request failure with the reserved-namespace or protected-header reason before selection

#### Scenario: Same policy for every adapter
- **WHEN** ProviderWire and the test adapter submit the same mapped call options
- **THEN** both SHALL receive the same policy outcome

### Requirement: Access-aware selection
The host SHALL supply one selector that chooses configured-account or request-BYOK selection from the verified caller's access. Every adapter SHALL use that selector, so an adapter serves both populations with one handler. Configured selection SHALL refuse host controls it does not support. BYOK selection SHALL decode its accounts from the host controls.

#### Scenario: One handler serves both populations
- **WHEN** a configured-account caller and a BYOK caller reach the same adapter route
- **THEN** the same adapter handler SHALL serve both, with catalog selection for the first and request-account selection for the second

#### Scenario: Caller without access
- **WHEN** a caller has neither configured nor BYOK access
- **THEN** selection SHALL return the permission failure without catalog or provider work

### Requirement: Selection under the execution deadline
The runtime SHALL start one execution deadline immediately before selection and SHALL run the host-owned selector at most once per request. Selector panics, `nil` models, invalid identities and results that arrive after the deadline or cancellation SHALL become runtime failures without invoking a model. A selector that ignores cancellation SHALL NOT extend handler latency; it MAY retain its worker.

#### Scenario: Selector ignores cancellation
- **WHEN** the selector blocks past the execution deadline
- **THEN** the runtime SHALL return a timeout failure promptly, never invoke a model, and discard the late selection

#### Scenario: Selector panics or returns an invalid selection
- **WHEN** the selector panics or returns an empty identity or nil model
- **THEN** the runtime SHALL return an internal failure without invoking a model

### Requirement: Shared execution limits
The model duration, stream idle duration and stream drain duration SHALL be one shared runtime configuration validated once at startup and supplied to every adapter. Adapters SHALL NOT define their own duration settings.

#### Scenario: Invalid duration
- **WHEN** a duration is non-positive, or the idle duration exceeds the model duration
- **THEN** startup SHALL fail before serving routes

### Requirement: Bounded unary invocation
The runtime SHALL invoke `DoGenerate` once on a contained worker under the remaining execution deadline. It SHALL return promptly on cancellation or deadline expiry, turn panics and `nil, nil` results into internal failures, and discard results that arrive after the caller has stopped waiting. A model that ignores cancellation MAY retain its worker.

#### Scenario: Model ignores cancellation
- **WHEN** `DoGenerate` blocks past the deadline and later returns a result
- **THEN** the caller SHALL receive the timeout failure promptly and the late result SHALL be discarded

#### Scenario: Model panics
- **WHEN** `DoGenerate` panics
- **THEN** the caller SHALL receive an internal failure

### Requirement: Single-owner stream lifecycle
The runtime SHALL invoke `DoStream` once and give the returned stream exactly one owner, which the adapter reads part by part. Each wait SHALL report a validated part, clean close, invalid output, cancellation, total deadline or idle timeout; the idle timer SHALL restart after each part the adapter accepts. Closing SHALL cancel provider work and start one drain bounded by the drain duration. A stream returned after the runtime stopped waiting SHALL be drained by its setup owner.

#### Scenario: Wait outcomes
- **WHEN** a provider sends parts, goes idle longer than the idle duration, or outlives the execution deadline, or the request is canceled
- **THEN** waits SHALL report part, idle timeout, total timeout or cancellation respectively, and cancellation SHALL take precedence when the request context is done

#### Scenario: Permanently ready stream after close
- **WHEN** the adapter closes a stream whose channel stays permanently ready
- **THEN** close SHALL return immediately and the drain SHALL stop within the drain duration

#### Scenario: Late stream from setup
- **WHEN** the deadline expires during `DoStream` and the provider later returns a stream
- **THEN** the runtime SHALL cancel and drain that stream exactly once without exposing it to the adapter

#### Scenario: Invalid setup result
- **WHEN** `DoStream` panics, returns an error together with a stream, or returns a nil result or channel
- **THEN** the adapter SHALL receive a setup failure before commitment and any returned channel SHALL be drained

### Requirement: Closed failure categories
The runtime SHALL classify selection, provider, transport and context failures into one closed set of categories: invalid request, model not found, permission, BYOK credentials, BYOK selector, unsupported gateway control, rate limit, overload, failed dependency, upstream, timeout, cancellation and internal. Every adapter SHALL map these categories to its own error documents and SHALL NOT classify provider errors itself. Classification SHALL NOT expose causes.

#### Scenario: Provider status classification
- **WHEN** a provider fails with HTTP 408/504, 429, 503/529, another 4xx or another status
- **THEN** the category SHALL be timeout, rate limit, overload, failed dependency or upstream respectively

#### Scenario: Transport and context failures
- **WHEN** the failure is a transport timeout, another transport error, context cancellation or deadline expiry
- **THEN** the category SHALL be timeout, upstream, cancellation or timeout respectively

#### Scenario: Same failure across adapters
- **WHEN** ProviderWire and the test adapter receive the same provider failure
- **THEN** both SHALL observe the same category

### Requirement: Attempt observation and execution views
The runtime SHALL observe fallback attempts for every selection, configured or BYOK, and seal observation when the outcome is decided. It SHALL project an execution overview only for configured selections, which carry catalog identity, and current native failure summaries for every selection, with configured-key protection. Observation SHALL be request-local, SHALL ignore attempts after sealing, and SHALL be available to every adapter.

#### Scenario: Late attempt after sealing
- **WHEN** a fallback attempt is reported after the runtime has returned the outcome
- **THEN** it SHALL NOT appear in the published overview

#### Scenario: BYOK selection
- **WHEN** a BYOK selection tries two accounts and the second succeeds
- **THEN** both attempts SHALL be captured in order, no configured overview SHALL be projected, and event-local native summaries SHALL remain available

#### Scenario: Concurrent requests
- **WHEN** two requests run their attempts concurrently
- **THEN** each request's captured attempts SHALL contain only its own

### Requirement: Per-attempt native option translation
An adapter SHALL be able to supply, with its request, a translator that maps its API's request semantics to native options for one physical attempt, given that attempt's provider type and provider-option namespace. The runtime SHALL apply it at every configured candidate and BYOK account attempt, to a copy of the call options, and SHALL apply host request policy to the result. Without a translator, every attempt SHALL receive the mapped options unchanged.

#### Scenario: Heterogeneous fallback chain
- **WHEN** an adapter supplies a translator and a configured route falls back from an Anthropic candidate to an OpenAI candidate
- **THEN** each attempt SHALL receive the options translated for its own provider type, and neither SHALL see the other's translation

#### Scenario: BYOK accounts
- **WHEN** a BYOK request with two accounts supplies a translator
- **THEN** each account attempt SHALL receive options translated for the selected provider

#### Scenario: Translation violates host policy or panics
- **WHEN** a translator adds a reserved namespace or protected header, or panics
- **THEN** that attempt SHALL fail with a non-retryable internal failure without contacting the provider

#### Scenario: No translator
- **WHEN** an adapter supplies no translator
- **THEN** each attempt SHALL receive the mapped call options unchanged

### Requirement: Request history correlation
Before selection, the runtime SHALL reconstruct unresolved provider-owned tool calls from the request prompt for every adapter. Inconsistent history SHALL fail as an invalid request before selection. The reconstructed calls SHALL seed output lifecycle validation without retaining history payloads.

#### Scenario: Deferred provider result
- **WHEN** the prompt contains an unresolved provider-owned call and the output contains only its result
- **THEN** output validation SHALL accept the result without a repeated call

#### Scenario: Inconsistent history
- **WHEN** the prompt contains a provider result without its call, or conflicting duplicate calls
- **THEN** the runtime SHALL return an invalid-request failure before selection

### Requirement: Provider stream lifecycle validation
The runtime SHALL validate every provider stream part against the shared provider lifecycle before the adapter receives it. An invalid part SHALL end the stream with an invalid-output outcome, cancel provider work and start the bounded drain. The rules SHALL be those ProviderWire enforces today; adapters SHALL add only checks specific to what their protocol can represent or what their request requires.

#### Scenario: Lifecycle rules
- **WHEN** a provider sends parts in a stream
- **THEN** the runtime SHALL enforce the following, and report any violation as invalid output:
- a provider start only as the first part;
- at most one response-metadata part, before any payload part;
- text and reasoning IDs nonempty, valid UTF-8, unique and active for their deltas and ends;
- tool input start, deltas and end before the matching call, and results only after their call or a history-seeded call;
- at most one final result per call, with previews only before it;
- finish only with no open text, reasoning or tool-input block, no pending preview, no warnings, and a registered finish reason with usage

#### Scenario: Parts after finish
- **WHEN** a provider sends parts after a valid finish
- **THEN** the runtime SHALL suppress them and report the stream finished

#### Scenario: Same lifecycle for every adapter
- **WHEN** ProviderWire and the test adapter consume the same invalid provider stream
- **THEN** both SHALL receive the invalid-output outcome at the same part

### Requirement: Unary result lifecycle validation
The runtime SHALL validate a unary result's content lifecycle before returning it to the adapter: tool calls unique, results matching a current or history-seeded call with the same name, no result after a final result, no unfinished previews, and a registered finish reason. An invalid result SHALL be an invalid-output failure, and no other candidate SHALL be invoked to replace it.

#### Scenario: Mismatched result
- **WHEN** a unary result contains a tool result whose call is unknown or has a different name
- **THEN** the adapter SHALL receive an invalid-output failure

#### Scenario: Valid result
- **WHEN** a unary result contains text, a call and its final result
- **THEN** the adapter SHALL receive it unchanged

