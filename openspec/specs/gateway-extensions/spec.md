# gateway-extensions Specification

## Purpose
Define the Gateway extension contract that lets Go modules add per-call behavior to a distribution through typed phases, and the host pipeline that runs those phases around every model call with fixed ordering, ownership filtering, failure containment and observation.

## Requirements

### Requirement: Experimental public extension package
A public Go package in the Gateway module SHALL define the extension contract: definitions, phase interfaces, request information, normalized response parts, the observation record, the source and key-ownership vocabulary and the denial and unavailable sentinels. It SHALL import no Gateway internal package and SHALL be documented as v0 and experimental, so its API MAY change in any minor release.

#### Scenario: Extension from another module
- **WHEN** a Go module outside the Gateway module defines an extension and passes it to the public entrypoint
- **THEN** it SHALL compile against the public packages only

#### Scenario: Package documentation states stability
- **WHEN** a developer reads the package documentation
- **THEN** it SHALL state that the API is v0, experimental and may break in minor releases

### Requirement: Extension definitions
An extension definition SHALL declare a name, a scope of `operator` or `tenant`, a failure policy of `fail-closed` or `skip`, and a factory. It MAY declare key ownerships, a transform priority, a JSON Schema for its configuration, secret names and whether it is enabled by default. Names SHALL be 1 to 63 lowercase letters, digits or hyphens, starting and ending with a letter or digit.

#### Scenario: Invalid definition
- **WHEN** a definition has an invalid name, an unknown scope or failure policy, an unknown or repeated key ownership, no factory, a configuration schema that does not compile, or an invalid or repeated secret name
- **THEN** startup SHALL fail before any request is served, naming the extension and the invalid field

#### Scenario: Duplicate names
- **WHEN** two definitions in the distribution's options share a name
- **THEN** startup SHALL fail naming the duplicate

### Requirement: Explicit registration
Extensions SHALL run only when the distribution passes their definitions to the public entrypoint's options. There SHALL be no global or import-time registry. A function SHALL return the definitions the shipped Gateway registers, which are none in this version, and the shipped backend SHALL pass exactly those.

#### Scenario: Importing registers nothing
- **WHEN** a distribution imports the public extension and composition packages without passing definitions
- **THEN** no extension SHALL run

#### Scenario: Shipped build
- **WHEN** the shipped backend starts
- **THEN** it SHALL pass the shipped definitions, which are empty, and no mandatory extension, and every request SHALL behave as without extensions

### Requirement: Phase interfaces
An extension instance SHALL take part in a phase only by implementing that phase's interface: admit, transform request, inspect request, inspect response or observe. An instance that implements none of them SHALL fail startup. Extensions SHALL NOT receive the model or a continuation, and SHALL NOT change identity, selection, credentials or the model's result other than through these phases.

#### Scenario: Instance without phases
- **WHEN** a factory returns an instance that implements no phase interface
- **THEN** startup SHALL fail naming the extension

#### Scenario: Partial implementation
- **WHEN** an instance implements only admit and observe
- **THEN** only those phases SHALL run for it

### Requirement: Phase order
For each call, the host SHALL run admit, then request transforms, then request inspection, then invoke the selected model, then response inspection on each output part, then observe. Admit SHALL see request information only, never the prompt. No phase SHALL run before the call has a valid selection.

#### Scenario: Full pipeline
- **WHEN** one extension implements every phase and a call succeeds
- **THEN** its phases SHALL run in the order admit, transform request, inspect request, inspect response for each normalized part, observe, with exactly one model invocation between inspect request and the first inspect response

#### Scenario: Selection fails
- **WHEN** selection fails or host request policy refuses the request
- **THEN** no extension phase SHALL run

### Requirement: Gate and inspector order
Admit, request inspection and response inspection SHALL each run across extensions in name order and SHALL stop at the first denial. Extensions after the denying one SHALL NOT run that phase or any later request or response phase for the call.

#### Scenario: Second admitter denies
- **WHEN** extensions `a`, `b` and `c` implement admit and `b` denies
- **THEN** `a` and `b` SHALL have run admit, `c` SHALL NOT, and no transform, inspection or model invocation SHALL occur

### Requirement: Transform order
Request transforms SHALL run tenant-scope extensions first, then operator-scope extensions; within a scope by ascending priority, then by name. Each transform SHALL receive the parameters the previous one returned.

#### Scenario: Mixed scopes and priorities
- **WHEN** operator extension `o1` with priority 0, tenant extension `t2` with priority 5 and tenant extension `t1` with priority 5 transform the request
- **THEN** they SHALL run as `t1`, `t2`, `o1`, and `o1` SHALL receive the parameters returned by `t2`

### Requirement: Host policy after each transform
The host SHALL check the parameters each transform returns against host request policy: no protected call header and no call-level `gateway` provider-option namespace. A refused result SHALL be a failure of that extension under its failure policy, and SHALL never reach a provider.

#### Scenario: Transform adds a protected header
- **WHEN** a `fail-closed` transform adds an `Authorization` header
- **THEN** the call SHALL fail with the unavailable category and zero provider calls

#### Scenario: Skipped transform adds the gateway namespace
- **WHEN** a `skip` transform adds `providerOptions.gateway`
- **THEN** its result SHALL be discarded, the next transform SHALL receive the parameters it received, and no provider call SHALL carry the namespace
- **AND** the refusal SHALL be counted and logged as that extension's failure

### Requirement: Request inspection sees the final parameters
Request inspection SHALL receive the parameters that leave the last transform and its policy check, which SHALL be exactly the parameters passed to the selected model.

#### Scenario: Inspector after a default-instructions transform
- **WHEN** a transform adds a system message and an inspector runs
- **THEN** the inspector SHALL see the system message, and the model SHALL receive the same parameters the inspector saw

### Requirement: Normalized response parts
Response inspection SHALL receive one normalized part type for unary and streaming calls: text, reasoning, a complete tool call, and finish. A streaming text or reasoning part SHALL carry its delta, and a unary one its complete text, so one inspector implementation serves both. Other output SHALL pass uninspected.

#### Scenario: Same inspector for unary and stream
- **WHEN** the same output is produced once unary and once streamed in three text deltas
- **THEN** the inspector SHALL receive one text part and a finish for the unary call, and three text parts and a finish for the stream, with the same concatenated text

#### Scenario: Tool call
- **WHEN** a stream emits tool input deltas and then the tool call
- **THEN** the inspector SHALL receive one tool-call part with the tool name and complete input, before the tool-call part is delivered

### Requirement: Response abort
A response inspector denial SHALL abort the response. For a unary call, the call SHALL fail with the denial and no result. For a stream, the denied part SHALL NOT be delivered; the host SHALL emit one error part carrying the denial, stop the model stream and close.

#### Scenario: Unary abort
- **WHEN** an inspector denies a unary result's text part
- **THEN** the caller SHALL receive the permission document and none of the result

#### Scenario: Stream abort after commitment
- **WHEN** an inspector denies the third text delta of a committed stream
- **THEN** the caller SHALL receive the first two deltas, then one permission error event and the end of the stream, and no later part

### Requirement: Per-call state
Each applicable extension SHALL get its own request information value for each call, with a state slot that it alone reads and writes, shared across all of its phases of that call and never across calls or extensions.

#### Scenario: Reserve and commit
- **WHEN** an extension stores a reservation in its state during admit
- **THEN** its observe phase for the same call SHALL read that reservation, and a concurrent call SHALL NOT

### Requirement: Request information
Each phase SHALL receive request information: the route namespace, key ownership, selection source, caller type (user, service account or internal service), the identity actor's typed UID when present, correlation ID, the requested model ID as sent, the canonical model ID when the selection has one, and the planned candidates in order. It SHALL NOT carry credentials or other user claims.

#### Scenario: BYOK call
- **WHEN** a service account calls with request BYOK for `openai/gpt-5` with one account
- **THEN** admit SHALL see source `request-byok`, ownership `customer`, caller type service account, its `service-account:<uid>`, the route namespace, the call's correlation ID, requested model `openai/gpt-5`, no canonical model, and one planned candidate with model `openai/gpt-5`, provider `openai`, provider model `gpt-5` and no provider instance

#### Scenario: Alias of a configured model
- **WHEN** an internal service calls a configured model by its alias, and the model has a primary and one fallback
- **THEN** admit SHALL see caller type internal service, no identity UID, the alias as requested model, the canonical ID, and the two configured candidates, each naming the alias, the canonical ID, its provider instance, provider and provider model

#### Scenario: Routed call
- **WHEN** a configured call names fallback models with `models`
- **THEN** the canonical model SHALL be the requested model's canonical ID, and the planned candidates SHALL be the composed chain, each naming the requested model or `models` entry and canonical ID it belongs to

#### Scenario: Request information matches the access decision
- **WHEN** an internal service calls on behalf of a user in `cloud` mode
- **THEN** the caller type SHALL be internal service and the identity UID SHALL be that user's `user:<uid>`, as the access decision recorded

#### Scenario: Own copies
- **WHEN** one extension changes the candidates of its request information
- **THEN** other extensions' request information and the call's execution SHALL be unchanged

### Requirement: Observation once per call
Observe SHALL run once per call for each applicable extension that implements it, after the outcome is decided, including denied, failed and cancelled calls. Its record SHALL carry the outcome (success, denied, error, cancelled or invalid output), the denying extension and phase when denied, usage as the logical observers report it, the physical attempts, start time and duration.

#### Scenario: Observation is off the response path
- **WHEN** an observer takes longer than the call itself
- **THEN** a unary result SHALL be returned, and a stream SHALL close, without waiting for the observer

#### Scenario: Denied at admit
- **WHEN** extension `b` denies at admit
- **THEN** every applicable observer SHALL receive one record with outcome denied, extension `b` and phase admit, no usage and no attempts

#### Scenario: Provider error and cancellation
- **WHEN** one call fails with a provider error and another stream is cancelled by the caller
- **THEN** each observer SHALL receive exactly one record per call, with outcome error and cancelled respectively

#### Scenario: Observer fails
- **WHEN** observe returns an error or panics
- **THEN** the call's result, error and stream SHALL be unchanged, other observers SHALL still run, and the failure SHALL be counted

### Requirement: Bounded asynchronous observation
Observations SHALL be dispatched after the unary result is returned or the stream is closed, each with a context that keeps the call's values but not its cancellation, bounded by a fixed per-call duration. The host SHALL track observations in flight; after shutdown begins, new observations SHALL be dropped and counted.

#### Scenario: Slow observer at shutdown
- **WHEN** shutdown begins while an observation is running
- **THEN** the final flush SHALL wait for it within the shared deadline, and an observation dispatched after the flush started SHALL be dropped and counted

### Requirement: Failure policies
A phase that returns an error other than a denial, panics, or returns parameters refused by host policy SHALL be a failure. Under `fail-closed`, the call SHALL fail with the unavailable category. Under `skip`, that phase's effect SHALL be discarded and the call SHALL continue. Every failure SHALL be logged and counted without the error's text.

#### Scenario: Fail-closed admit failure
- **WHEN** a `fail-closed` admitter returns a connection error
- **THEN** the call SHALL fail with the unavailable document, zero provider calls and an observe record with outcome error

#### Scenario: Skipped inspector failure
- **WHEN** a `skip` response inspector returns an error for a part
- **THEN** the part SHALL be delivered and the failure counted

### Requirement: Panic containment
Every call into extension code, including factories, phases and close, SHALL recover a panic and treat it as a failure of that extension. A panic SHALL NOT end the process, another call or another extension's phase.

#### Scenario: Panicking inspector in a stream
- **WHEN** a `fail-closed` response inspector panics on a stream part
- **THEN** the stream SHALL end with one unavailable error event, other concurrent calls SHALL be unaffected, and the panic SHALL be counted

### Requirement: Pipeline placement
The pipeline SHALL wrap every selection, whatever its source, inside the logical observers and outside the selected model and its fallback chain. Extension outcomes SHALL NOT be retried by fallback, and request phases SHALL complete before any provider call.

#### Scenario: Denial is not retried
- **WHEN** a configured model has two candidates and an inspector denies its request
- **THEN** zero provider calls SHALL occur and the caller SHALL receive the permission document

#### Scenario: Response denial after fallback
- **WHEN** the first candidate fails, the second succeeds and an inspector denies its output
- **THEN** exactly two provider calls SHALL occur and the caller SHALL receive the permission document

#### Scenario: Routed and BYOK calls
- **WHEN** routed and request BYOK calls run with an extension that applies to them
- **THEN** its phases SHALL run once per call, not once per candidate

### Requirement: Operator extension configuration
Operator-scope extensions SHALL be configured by the operator extension setting. An extension with no entry SHALL use its default state and an empty configuration. An entry SHALL enable the extension unless it sets `enabled: false`. Configuration SHALL be validated against the extension's schema before its factory runs, and each declared secret SHALL be resolved from the environment variable its entry names.

#### Scenario: Default-enabled extension without an entry
- **WHEN** an operator extension enabled by default has no entry and its schema accepts an empty object
- **THEN** its factory SHALL run once at startup with an empty configuration

#### Scenario: Invalid configuration
- **WHEN** an entry's configuration fails the extension's schema
- **THEN** startup SHALL fail naming the extension and the failing location, without configuration values

### Requirement: Tenant extensions without namespace configuration
Until namespace configuration exists, a tenant-scope extension SHALL run in every namespace where it applies when it is enabled by default, with an empty configuration, and SHALL NOT run otherwise. The operator extension setting SHALL NOT configure tenant-scope extensions.

#### Scenario: Tenant extension in the operator setting
- **WHEN** the operator setting names a tenant-scope extension
- **THEN** startup SHALL fail naming it

### Requirement: Mandatory extensions
A distribution MAY mark operator-scope extensions as mandatory by name. A mandatory extension SHALL have an enabled entry in the operator setting with a valid configuration and resolved secrets, or startup SHALL fail with a secret-safe reason. Marking an unknown name or a tenant-scope extension as mandatory SHALL fail startup.

#### Scenario: Mandatory extension disabled or unconfigured
- **WHEN** a mandatory extension has no entry, or its entry sets `enabled: false`
- **THEN** startup SHALL fail naming the extension

#### Scenario: Mandatory tenant extension
- **WHEN** a distribution marks a tenant-scope extension as mandatory
- **THEN** startup SHALL fail naming it

### Requirement: Extension instance lifecycle
Enabled extensions SHALL be constructed at startup, before the plugin serves requests, with a logger scoped to the extension, the process metrics registerer with the extension's name as a constant label, a clock and their resolved secrets. A factory error SHALL fail startup. Instances SHALL be shared by concurrent calls, and the phases of one call SHALL never run concurrently.

#### Scenario: Factory contract allows more than one run
- **WHEN** a developer reads the factory documentation
- **THEN** it SHALL state that a factory may run more than once per process, once per configuration it is given

#### Scenario: Factory fails
- **WHEN** an enabled extension's factory returns an error or panics
- **THEN** startup SHALL fail naming the extension, without the error's text

#### Scenario: Concurrent calls
- **WHEN** many calls run concurrently through one extension
- **THEN** each call's phases SHALL run sequentially and see only that call's state

### Requirement: Physical attempts in the observation record
The record's attempts SHALL be the call's physical attempts, projected as the execution overview projects them: in order, each with provider, provider instance when configured, provider model, outcome (selected, failed or canceled) and status code when known, the i-th attempt being the i-th planned candidate. They SHALL carry no error text, credentials or other provider-originated values.

#### Scenario: Fallback
- **WHEN** a configured call's primary fails with HTTP 503 and its fallback is selected
- **THEN** the record SHALL carry a failed attempt with status 503 for the primary, then a selected attempt for the fallback

#### Scenario: Single candidate
- **WHEN** a configured model with one candidate answers
- **THEN** the record SHALL carry one selected attempt for that candidate

#### Scenario: Output refused after the model answered
- **WHEN** a response inspector denies the output of a single-candidate call
- **THEN** the record SHALL carry one selected attempt, as the overview does

#### Scenario: Refused before the model
- **WHEN** an extension denies at admit or request inspection
- **THEN** the record SHALL carry no attempts

#### Scenario: Stream
- **WHEN** a stream's primary fails before its first part, its fallback streams, the adapter publishes the overview at the first part, and the stream then closes
- **THEN** the record dispatched at stream end SHALL carry both attempts

#### Scenario: Single-candidate stream without output
- **WHEN** a single-candidate stream closes before its first part
- **THEN** the record SHALL carry one failed attempt, as the overview does

#### Scenario: Attempts without attribution
- **WHEN** the observed attempts cannot be attributed to the planned candidates, so the execution overview is omitted
- **THEN** the record SHALL carry no attempts rather than invented ones

### Requirement: Unreplaceable attempt capture
Extensions SHALL NOT be able to replace, wrap or extend the runtime's attempt capture, or observe physical attempts other than through the record. The execution overview and the record SHALL come from the same capture, and each observer SHALL receive its own copy of the attempts.

#### Scenario: Extension derives its own attempt observer
- **WHEN** an extension derives a context with its own fallback attempt observer in a request phase
- **THEN** that observer SHALL receive no attempt of the call, and the overview and records SHALL be unchanged

#### Scenario: Observer changes its attempts
- **WHEN** one observer changes the attempts in its record
- **THEN** other observers' records and the call's overview SHALL be unchanged

### Requirement: Key ownership filtering
Key ownership SHALL be `operator` for operator-catalog selections and `customer` for every other source, including sources added later. An extension SHALL apply to a call when its declared ownerships, which default to both, include the call's ownership. The call's source SHALL be informational for extensions and SHALL NOT decide where an extension applies. An extension that does not apply SHALL run no phase and get no observe record.

#### Scenario: Operator-owned extension on a BYOK call
- **WHEN** an extension declares ownership `operator` and a request BYOK call runs
- **THEN** none of its phases SHALL run for that call

#### Scenario: Undeclared ownership
- **WHEN** an extension declares no ownership
- **THEN** it SHALL apply to operator-catalog, routed and request BYOK calls

#### Scenario: Customer-owned extension
- **WHEN** an extension declares ownership `customer`
- **THEN** it SHALL apply to request BYOK and stack-catalog calls and SHALL NOT apply to operator-catalog calls
