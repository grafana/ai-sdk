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
An extension definition SHALL declare a name, a scope of `operator` or `tenant`, a failure policy of `fail-closed` or `skip`, and a factory. It MAY declare key ownerships, sources, a transform priority, a JSON Schema for its configuration, secret names and whether it is enabled by default. Names SHALL be 1 to 63 lowercase letters, digits or hyphens, starting and ending with a letter or digit.

#### Scenario: Invalid definition
- **WHEN** a definition has an invalid name, an unknown scope or failure policy, no factory, a configuration schema that does not compile, or an invalid or repeated secret name
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

### Requirement: Key ownership and source filtering
Key ownership SHALL be `operator` for operator-catalog selections and `customer` for every other source. An extension SHALL apply to a call when its declared ownerships, which default to both, include the call's ownership and its declared sources, when present, include the call's source. An extension that does not apply SHALL run no phase and get no observe record.

#### Scenario: Operator-owned extension on a BYOK call
- **WHEN** an extension declares ownership `operator` and a request BYOK call runs
- **THEN** none of its phases SHALL run for that call

#### Scenario: Undeclared ownership
- **WHEN** an extension declares no ownership or source
- **THEN** it SHALL apply to operator-catalog, routed and request BYOK calls

#### Scenario: Narrowed by source
- **WHEN** an extension declares ownership `customer` and source `stack-catalog`
- **THEN** it SHALL NOT apply to request BYOK calls

#### Scenario: Inconsistent declaration
- **WHEN** a definition declares ownership `operator` and source `request-byok`
- **THEN** startup SHALL fail naming the extension

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

### Requirement: Tenant configuration resources
The Gateway's API group SHALL serve a namespaced `Middleware` kind in version `v1alpha1` that is not folder-scoped and declares no secure values. Its `spec` SHALL hold the extension `type`, a required `enabled` flag and, as optional fields, one typed member per public tenant type with configuration, named the lowerCamel form of the type. A resource's name SHALL equal its `spec.type`, so a namespace holds at most one resource per extension.

#### Scenario: Resource saved without a folder
- **WHEN** a writer creates a valid `Middleware` without a folder annotation
- **THEN** it SHALL be stored in the request namespace

#### Scenario: Name differs from type
- **WHEN** a writer creates or updates a `Middleware` whose name differs from `spec.type`
- **THEN** the write SHALL be refused

#### Scenario: Nested configuration kept as written
- **WHEN** a writer stores a valid `Middleware` whose member holds nested objects, arrays of objects and typed maps
- **THEN** reads SHALL return that member unchanged

### Requirement: Public tenant extension types
Tenant extension types SHALL be declared in a public, experimental package: each with a name, a description, an optional configuration schema, a default state and an example configuration that its schema accepts. The package SHALL bind an implementation to a type to produce a tenant-scope definition. The registry the shipped Gateway uses SHALL be the only source of members in the manifest, and SHALL be empty in this version.

#### Scenario: Private implementation of a public type
- **WHEN** a distribution binds its own implementation to a public type and passes the definition to the entrypoint
- **THEN** the Gateway SHALL start and admit resources of that type, with the manifest unchanged

#### Scenario: Invalid type
- **WHEN** a type has an invalid or repeated name, a member that collides with another type's or with `type` or `enabled`, a schema without an example it accepts, or an example without a schema
- **THEN** startup SHALL fail naming the type and the field

### Requirement: Additive tenant type evolution
Because the platform validates every write against the current schema without exempting unchanged fields, a public type's schema SHALL change only additively: new optional fields and nothing narrower. An incompatible change SHALL use a new type name.

#### Scenario: Required field added to a type
- **WHEN** a change to a public type would add a required field or narrow a field's type
- **THEN** it SHALL be made as a new type instead

### Requirement: Tenant definitions bind to public types
Startup SHALL fail, before any setting is read and also on hosts that start idle, when a tenant-scope definition's name is not a public type's, or when its configuration schema or default differs from that type's. Private extensions SHALL be operator-scope unless they implement a public tenant type.

#### Scenario: Tenant definition without a public type
- **WHEN** a distribution passes a tenant-scope definition whose name no public type has
- **THEN** startup SHALL fail naming the extension

#### Scenario: Definition differs from its type
- **WHEN** a tenant-scope definition has a public type's name but another schema or default
- **THEN** startup SHALL fail naming the extension and the differing field

### Requirement: Tenant configuration member projection
Each manifest member SHALL be projected from its type's schema, keeping JSON types, properties, items, closed objects, typed maps, required fields and descriptions, and dropping value constraints and defaults. The manifest SHALL have no enum on `spec.type`, no union across members and no required member. A type whose projection is not structural, or whose example pruning changes, SHALL be refused. A drift check SHALL guarantee the committed manifest equals the projection.

#### Scenario: Value constraints stay in admission
- **WHEN** a type's schema has an enum, a pattern or a bound on a field
- **THEN** the manifest member SHALL declare only the field's JSON type, and admission SHALL check the constraint

#### Scenario: Schema the platform would prune
- **WHEN** a type's schema has an open object, an empty object, an object without an explicit `additionalProperties: false`, an untyped node or a combinator
- **THEN** the type SHALL be refused before it reaches the manifest

#### Scenario: Drift
- **WHEN** the registry changes without regenerating the manifest
- **THEN** the repository checks SHALL fail

### Requirement: Tenant configuration admission
Admission SHALL accept a create or update only when the spec sets no member other than its type's, `spec.type` names a tenant-scope extension the distribution implements, a type with configuration has its member when `enabled` is `true`, a type without configuration has no member, and a present member matches that type's full schema. Operator-scope, unregistered and unimplemented types SHALL be refused with the same reason. Members SHALL be decoded as written, so numbers are not rounded.

#### Scenario: Valid configuration
- **WHEN** a writer creates a `Middleware` for an implemented tenant type with a member its schema accepts
- **THEN** the write SHALL be accepted, whether `enabled` is `true` or `false`

#### Scenario: Member of another type
- **WHEN** the spec sets a member other than its type's, or more than one member
- **THEN** the write SHALL be refused naming only the type's member

#### Scenario: Enabled without its member
- **WHEN** an enabled resource of a type with configuration has no member
- **THEN** the write SHALL be refused

#### Scenario: Disabled without a member
- **WHEN** a writer creates a disabled resource of an implemented type without its member
- **THEN** the write SHALL be accepted

#### Scenario: Configuration that fails the schema
- **WHEN** the member fails the type's schema, including a constraint the manifest does not declare
- **THEN** the write SHALL be refused naming the member and the failing location

#### Scenario: Operator or unimplemented type
- **WHEN** `spec.type` names an operator-scope extension, or a public type this build does not implement
- **THEN** the write SHALL be refused with the reason given for an unregistered type

#### Scenario: Large integer
- **WHEN** a member holds an integer above 2^53 and the write is accepted
- **THEN** admission and the stored resource SHALL keep its exact value

#### Scenario: Shipped build
- **WHEN** the shipped Gateway, which implements no tenant extension, reviews a `Middleware` create
- **THEN** it SHALL refuse it

### Requirement: Disable-only updates
An update that sets `spec.enabled` to `false` and keeps the stored member, or removes it, SHALL be accepted whatever the stored type, including a type the running build does not implement, a member that no longer matches its schema or bound, and a member the running manifest no longer declares. Any other update of a resource whose type is not implemented SHALL be refused.

#### Scenario: Disabling after a rollback
- **WHEN** a resource was stored by a build that implements its type, and a build that does not reviews an update that only sets `enabled: false`
- **THEN** the update SHALL be accepted

#### Scenario: Disabling with the member removed
- **WHEN** an update sets `enabled: false` and omits the stored member
- **THEN** the update SHALL be accepted

#### Scenario: Enabling or reconfiguring an unknown type
- **WHEN** a build that does not implement the stored type reviews an update that keeps `enabled: true` or changes the member
- **THEN** the update SHALL be refused

#### Scenario: Disabling a configuration the schema no longer accepts
- **WHEN** an implemented type's schema no longer accepts the stored member and an update only sets `enabled: false`
- **THEN** the update SHALL be accepted

### Requirement: Tenant configuration size bound
Admission SHALL refuse a member whose JSON encoding exceeds 64 KiB.

#### Scenario: Oversized configuration
- **WHEN** a create or update carries a member larger than the bound
- **THEN** it SHALL be refused naming the member and the bound, without its content

### Requirement: Secret-safe tenant configuration refusals
Admission refusals SHALL be `400 Bad Request` with a reason naming the failing field path and, for schema failures, the failing location within the member. A reason MAY name the type and its member. Neither a reason nor any log or metric the plugin produces SHALL contain a configuration value, an undeclared key or the schema validator's own message.

#### Scenario: Refused write carrying sentinel values
- **WHEN** refused creates and updates carry sentinel strings as member values, as undeclared member keys and as undeclared spec members
- **THEN** no response body and no plugin log SHALL contain them

### Requirement: Tenant configuration admission scope
Admission SHALL review `CREATE` and `UPDATE` of `Middleware` only. Deletes SHALL NOT be reviewed, so a resource can always be deleted, including while the plugin is unavailable or no longer implements its type. Creates and updates SHALL fail closed while the plugin is unavailable.

#### Scenario: Delete
- **WHEN** a writer deletes a `Middleware`
- **THEN** the plugin SHALL receive no admission request, and the delete SHALL succeed even when the plugin would refuse every review

### Requirement: Tenant configuration roles
The manifest SHALL declare `aigateway:middlewarereader`, granting the viewer permission set on `Middleware` and bound to Viewer, and `aigateway:middlewarewriter`, granting the editor permission set on `Middleware` and bound to Admin. They SHALL share the manifest's roles map and bindings with the catalog roles, which SHALL stay unchanged.

#### Scenario: Viewer and Editor
- **WHEN** a Viewer or Editor gets or lists `Middleware` resources in their namespace
- **THEN** the request SHALL be allowed, and their create, update and delete requests SHALL be refused

#### Scenario: Admin
- **WHEN** an Admin creates, updates or deletes a `Middleware`
- **THEN** authorization SHALL allow the request, subject to admission

#### Scenario: Catalog roles unchanged
- **WHEN** the committed manifest is inspected
- **THEN** the catalog roles SHALL grant exactly what they granted before, and grant nothing on `Middleware`

### Requirement: Stored tenant configuration has no runtime effect
A stored `Middleware` SHALL NOT change which extensions run, or with which configuration, on any route. Tenant extensions SHALL keep running as "Tenant extensions without namespace configuration" requires.

#### Scenario: Stored configuration next to a running extension
- **WHEN** a namespace stores `enabled: true` for a tenant extension that is off by default, and `enabled: false` for one that is on by default
- **THEN** the first SHALL not run and the second SHALL run, as without the resources

### Requirement: Tenant configuration documentation
The extension guide SHALL describe the `Middleware` resource and its members, its roles, the admission rules, how to disable after a rollback, that creates and updates fail while the plugin is unavailable while deletes always work, that stored resources are not applied yet, how a public tenant type is added and projected, and that private extensions are operator-scope unless they implement a public type.

#### Scenario: Extension author reads the guide
- **WHEN** an extension author reads the guide's tenant configuration section
- **THEN** they SHALL find that a tenant type's name, schema and default are public, that its implementation may be private, and that type schemas change only additively

### Requirement: Tenant configuration evidence
Roles, type and name refusals and secret-safe refusals SHALL be proven end to end against Grafana with the built plugin. While the shipped registry is empty, acceptance, member rules, schema and operator-type refusals, disable-only updates, exact numbers and unreviewed deletes SHALL be proven through the protocol test host with test-only types and definitions passed through internal options, never compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, the refusals and the secret-safe responses and plugin logs through Grafana's API

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped types and definitions
