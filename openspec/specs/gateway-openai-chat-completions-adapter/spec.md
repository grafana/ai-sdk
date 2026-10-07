# gateway-openai-chat-completions-adapter Specification

## Purpose

Expose bounded OpenAI Chat Completions over configured Gateway providers while preserving native semantics and stable public streaming identity.

## Requirements

### Requirement: Adapter route and authentication
The Gateway SHALL own POST `/v1/chat/completions` with OpenAI JSON errors, strict one-token Bearer authentication in access-token mode, existing credential-stripped cloud authentication, canonical model resolution and one shared middleware stack. The adapter SHALL translate only between the public Chat Completions contract and the canonical provider domain; host composition SHALL apply native default translation at each candidate boundary, including configured compatible namespaces, without model allowlists.

#### Scenario: Conflicting adapter credentials
- **WHEN** Authorization is accompanied by X-Access-Token, X-Grafana-Id or a proxy assertion in access-token mode
- **THEN** the request fails with an OpenAI 401 envelope before body parsing or provider invocation.

#### Scenario: Unsupported adapter route or method
- **WHEN** a caller uses `/v1/models` or a non-POST Chat method
- **THEN** the Gateway returns an OpenAI 404 or 405 envelope respectively, including Allow for 405.

### Requirement: Explicit request capability matrix
The adapter SHALL accept the documented text/function/schema/reasoning subset, reject unsupported protocol fields before invocation, and pass mapped requests through configured fallback without extra capability restrictions. Provider capabilities SHALL remain with native adapters. Responses SHALL receive explicit store false and omitted/null Chat strict SHALL translate to false.

#### Scenario: Defaults differ from Responses
- **WHEN** a Chat request omits store and function strict
- **THEN** the Responses upstream sees store false and function strict false.

#### Scenario: Heterogeneous fallback preserves defaults
- **WHEN** configured candidates include Responses and a custom compatible namespace
- **THEN** every attempted candidate receives its matching Chat storage/schema/parallel defaults without option intersection or request mutation.

### Requirement: Valid adapter results
Unary and streaming outputs SHALL preserve available native response identity, one choice, validated finish reasons and truthful optional usage. Streams SHALL freeze identity at first emitted content, filling missing fields once. Native warnings SHALL remain successful diagnostics in the bounded optional grafana.warnings extension; later native identity SHALL use grafana.native_response without changing emitted identity. Unsupported content SHALL fail safely. Strict schema output SHALL be checked on successful stop, never on length/filter partial output.

#### Scenario: Fragmented function call
- **WHEN** a stream supplies function start, argument deltas, end and a matching final call
- **THEN** the adapter stream emits one stable tool index, initial ID/name/type once and argument fragments once.

#### Scenario: Empty tool input normalization
- **WHEN** a provider emits tool input start/end without argument deltas and a matching final call normalized to `{}`
- **THEN** the adapter emits `{}` exactly once and completes the tool call successfully.

#### Scenario: Invalid or premature stream termination
- **WHEN** a stream closes before finish, exceeds limits or contradicts function fragments
- **THEN** the adapter emits a safe OpenAI stream error where writable and does not emit DONE.

#### Scenario: Native warnings and delayed identity
- **WHEN** a provider returns unsupported-setting warnings and response identity before or after initial content
- **THEN** official OpenAI Go and JavaScript clients consume successful output and the extension, with stable stream id/model/created and bounded diagnostics.

### Requirement: Bounded lifecycle
The adapter SHALL enforce request/response/frame/part and total/idle/drain bounds, preserve request cancellation and process shutdown, and avoid resets of idle time for discarded metadata/reasoning. Documentation SHALL distinguish per-request bounds from global admission.

#### Scenario: No visible progress
- **WHEN** a provider continuously emits discarded parts without adapter-visible progress
- **THEN** the idle deadline still cancels execution.

#### Scenario: Provider returns after cancellation
- **WHEN** a cooperative delayed setup returns a stream after cancellation
- **THEN** a single owner drains it only for the configured bounded interval.

#### Scenario: Claimed stream fails without closing
- **WHEN** setup returns a nonnil stream plus an error or a committed stream fails
- **THEN** response completion does not wait for the drain budget, the provider context is cancelled, and exactly one asynchronous cleanup consumer exits on channel closure or drain expiry.

#### Scenario: Process shutdown with adapter SSE active
- **WHEN** shutdown begins with a committed adapter SSE response and unary requests active
- **THEN** their upstream contexts are cancelled, clients terminate, and the real command exits within its shutdown bound.

### Requirement: Explicit bounded JSON validation
The adapter SHALL validate a closed explicit protocol schema and decode typed DTOs using standard JSON semantics without reflective recursive field validation.

#### Scenario: Duplicate and incorrectly cased fields
- **WHEN** a request contains duplicate known keys or an incorrectly cased protocol key
- **THEN** duplicates use the last value and incorrectly cased keys fail before invocation.
