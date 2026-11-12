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

### Requirement: Mandatory extensions
A distribution MAY mark operator-scope extensions as mandatory by name. A mandatory extension SHALL have an enabled entry in the operator setting with a valid configuration and resolved secrets, or startup SHALL fail with a secret-safe reason. Marking an unknown name or a tenant-scope extension as mandatory SHALL fail startup.

#### Scenario: Mandatory extension disabled or unconfigured
- **WHEN** a mandatory extension has no entry, or its entry sets `enabled: false`
- **THEN** startup SHALL fail naming the extension

#### Scenario: Mandatory tenant extension
- **WHEN** a distribution marks a tenant-scope extension as mandatory
- **THEN** startup SHALL fail naming it

### Requirement: Extension instance lifecycle
Operator-scope extensions enabled by the operator setting, and tenant-scope extensions enabled by default, SHALL be constructed at startup, before requests are served, with a logger scoped to the extension, the definition's shared registerer, a clock and resolved secrets. A startup factory error SHALL fail startup. Per-namespace tenant instances follow "Per-namespace extension instances". Instances SHALL be shared by concurrent calls, and one call's phases SHALL never run concurrently.

#### Scenario: Factory contract allows more than one run
- **WHEN** a developer reads the factory documentation
- **THEN** it SHALL state that a factory may run more than once per process, once per configuration it is given, and that collectors are registered through the shared registerer

#### Scenario: Factory fails
- **WHEN** an extension's factory returns an error or panics at startup
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

### Requirement: Tenant configuration documentation
The extension guide SHALL describe the `Middleware` resource and its members, roles and admission rules, disabling after a rollback, how a public tenant type is added and projected, that private extensions are operator-scope unless they implement a public type, propagation within the TTL on each replica, read failures and their bound, behavior without API access, and what tenant extensions see.

#### Scenario: Extension author reads the guide
- **WHEN** an extension author reads the guide's tenant configuration section
- **THEN** they SHALL find that a tenant type's name, schema and default are public, that its implementation may be private, that type schemas change only additively, and that a factory may run once per namespace configuration

#### Scenario: Org admin reads the guide
- **WHEN** an org admin reads how a change takes effect
- **THEN** they SHALL find the TTL, that replicas converge independently, and that a failed read never falls back to a more permissive default

### Requirement: Tenant configuration evidence
Roles, type and name refusals and secret-safe refusals SHALL be proven end to end against Grafana with the built plugin. While the shipped registry is empty, acceptance, member rules, schema and operator-type refusals, disable-only updates, exact numbers, unreviewed deletes and namespace configuration reads SHALL be proven through the protocol test host with test-only types and definitions passed through internal options, never compiled into the shipped backend.

#### Scenario: End-to-end suite with the shipped build
- **WHEN** the plugin end-to-end suite runs against the pinned Grafana image
- **THEN** it SHALL exercise the roles, the refusals and the secret-safe responses and plugin logs through Grafana's API

#### Scenario: Namespace reads through the test host
- **WHEN** Gateway tests store different resources in two namespaces of the protocol test host and call the language-model route in each
- **THEN** each call SHALL apply its namespace's configuration, read over the platform API with the plugin's own token

#### Scenario: Shipped backend
- **WHEN** the plugin package is built
- **THEN** its backend SHALL register only the shipped types and definitions

### Requirement: Namespace tenant configuration
Each call's tenant-scope extensions SHALL follow the `Middleware` resources of the call's namespace. With no resource for an extension, it SHALL use its default state: on with an empty configuration when enabled by default, off otherwise. A resource with `enabled: false` SHALL turn it off. A resource with `enabled: true` SHALL run it with the configuration in its type's member. One namespace's resources SHALL NOT affect another namespace's calls.

#### Scenario: Two namespaces with different configurations
- **WHEN** namespace `a` stores `enabled: true` with one configuration for a default-off tenant extension, namespace `b` stores another configuration for it, and namespace `c` stores nothing
- **THEN** calls in `a` and `b` SHALL each run the extension with their own configuration, and calls in `c` SHALL NOT run it

#### Scenario: Disabling a default-on extension
- **WHEN** a namespace stores `enabled: false` for a tenant extension that is enabled by default
- **THEN** none of its phases SHALL run for that namespace's calls, and other namespaces SHALL keep running it

#### Scenario: Resource removed
- **WHEN** a namespace deletes its resource for an extension
- **THEN** after the change propagates, its calls SHALL use the extension's default state

### Requirement: Tenant configuration checked when read
At read time the Gateway SHALL check each enabled resource's member against its type's schema, as admission does, and SHALL pass the factory the stored member with numbers unchanged. A spec that sets another member, lacks a required one or can't be decoded, a failing member, or a failing or panicking factory SHALL apply the extension's failure policy in that namespace. It SHALL be logged and counted without configuration values, and SHALL NOT fail startup or affect other namespaces.

#### Scenario: Schema tightened after storage
- **WHEN** a stored enabled member no longer matches its `skip` extension's schema
- **THEN** that extension SHALL NOT run for the namespace's calls, the failure SHALL be counted, and calls SHALL continue

#### Scenario: Fail-closed extension with invalid configuration
- **WHEN** a `fail-closed` tenant extension's stored member fails its schema
- **THEN** calls in that namespace to which it applies SHALL fail with the unavailable category before any provider call

#### Scenario: Large integer
- **WHEN** a stored member holds the integer `9007199254740993`, nested inside an object
- **THEN** the factory SHALL receive that exact number

#### Scenario: Member of another type
- **WHEN** a stored spec sets a member other than its type's, or more than one member
- **THEN** that extension SHALL apply its failure policy in that namespace

#### Scenario: Factory fails for one namespace
- **WHEN** a tenant extension's factory fails for one namespace's configuration
- **THEN** that namespace SHALL apply the failure policy, and another namespace with a valid configuration SHALL run the extension

### Requirement: Tenant types not implemented at read time
An enabled resource whose type this build does not implement as a tenant-scope extension SHALL be skipped. It SHALL be logged once per namespace and type while the namespace stays cached, and counted. A disabled resource of such a type SHALL be ignored without a log or count, since it has no effect.

#### Scenario: Rollback leaves an enabled resource
- **WHEN** a namespace stores an enabled resource whose type the running build doesn't implement, and three calls run
- **THEN** all three calls SHALL run without it, and the Gateway SHALL emit one log record and one count for that namespace and type

#### Scenario: Disabled resource of a type not implemented
- **WHEN** that resource is disabled
- **THEN** the Gateway SHALL neither log nor count it

### Requirement: Tenant configuration read failures
A failed read SHALL NOT be treated as a missing resource. A failure is any error, timeout, refusal, unexpected status or response the Gateway can't decode, including a refused or not-found list. With no configuration loaded for the namespace, each tenant extension SHALL apply its failure policy. A failed read SHALL never apply a more permissive default.

#### Scenario: First read fails
- **WHEN** the first read for a namespace fails, and that namespace has a `skip` tenant extension that is enabled by default and a `fail-closed` one
- **THEN** the `skip` extension SHALL NOT run, calls to which the `fail-closed` one applies SHALL fail with the unavailable category, and neither SHALL use its default state

#### Scenario: Listing refused
- **WHEN** the platform answers a namespace's list with `403` or `404`
- **THEN** the Gateway SHALL treat it as a failed read, not as a namespace without resources

### Requirement: Last-known-good tenant configuration
When a refresh fails, the Gateway SHALL keep serving the namespace's last successfully read configuration for at most 10 minutes after that read. After that bound, each tenant extension in the namespace SHALL apply its failure policy until a read succeeds.

#### Scenario: Refresh fails within the bound
- **WHEN** a namespace's configuration was read successfully and refreshes fail for the next 9 minutes
- **THEN** its calls SHALL keep the last-known-good configuration, including any extension it disables

#### Scenario: Bound exceeded
- **WHEN** refreshes keep failing for more than 10 minutes after the last successful read
- **THEN** its calls SHALL apply each tenant extension's failure policy, and the next successful read SHALL restore the stored configuration

### Requirement: Tenant configuration reads as the plugin
The Gateway SHALL read `Middleware` resources with the plugin's own identity, on a context that carries no caller identity or cancellation, with a read timeout of its own. A caller's permissions, identity, deadline or cancellation SHALL NOT affect a read or its result.

#### Scenario: Caller without read access
- **WHEN** a caller that can't read `Middleware` resources makes the first call in a namespace
- **THEN** the read SHALL be made as the plugin and its result applied to the call

#### Scenario: Caller cancels during the first read
- **WHEN** the first caller in a namespace cancels while the read is in flight
- **THEN** the read SHALL complete within its own timeout, and the next call SHALL use its result

### Requirement: Tenant configuration cache
The Gateway SHALL cache each namespace's configuration, including the absence of resources, for a TTL of 1 minute. A call SHALL wait only for a namespace's first read, and concurrent calls SHALL share one read. After the TTL, calls SHALL use the cached configuration while one background refresh runs. A failed read SHALL be retried no earlier than a fixed backoff. The cache SHALL hold a bounded number of namespaces, evicting the least recently used.

#### Scenario: Concurrent first calls
- **WHEN** ten calls arrive for an uncached namespace at the same time
- **THEN** the Gateway SHALL make one read, and every call SHALL use its result

#### Scenario: Change applies within the TTL
- **WHEN** a resource changes and a call arrives more than one TTL after the namespace was last read
- **THEN** that call SHALL trigger a refresh, and calls after the refresh completes SHALL use the new configuration

#### Scenario: Failure backs off
- **WHEN** a refresh fails and further calls arrive before the backoff ends
- **THEN** no further read SHALL be made until the backoff ends

#### Scenario: Bound reached
- **WHEN** a new namespace is read while the cache holds its maximum number of namespaces
- **THEN** the least recently used namespace SHALL be evicted, and its next call SHALL read again

### Requirement: Independent replica convergence
Each Gateway process SHALL cache independently. A change SHALL take effect on each process within one TTL of that process's next call in the namespace, plus that read's duration. Processes MAY disagree inside that window.

#### Scenario: Two processes
- **WHEN** two Gateway processes serve one namespace and its resource changes
- **THEN** each SHALL apply the change after its own refresh, without coordination between them

### Requirement: Tenant configuration reads only when needed
The Gateway SHALL read a namespace's configuration only for a call to which at least one tenant-scope extension applies. With no tenant-scope extension registered, it SHALL make no reads.

#### Scenario: Shipped build
- **WHEN** the shipped backend, which registers no tenant extension, serves calls
- **THEN** it SHALL make no `Middleware` reads

#### Scenario: Extension limited to another key ownership
- **WHEN** the only tenant extension declares ownership `operator`, and a request BYOK call runs
- **THEN** that call SHALL NOT read the namespace's configuration

### Requirement: Per-namespace extension instances
A tenant extension enabled by a namespace's resource SHALL get its own instance, built once for each configuration revision of that namespace and reused while the configuration stays unchanged. An instance SHALL be closed after its namespace's configuration is replaced, evicted or expires, and only once no call or observation still uses it. Defaults SHALL use the instance built at startup.

#### Scenario: Unchanged refresh
- **WHEN** a refresh reads the same configuration
- **THEN** the namespace SHALL keep its instances, and no factory SHALL run

#### Scenario: Replaced during a stream
- **WHEN** a namespace's configuration changes while a stream that uses its instance is open
- **THEN** the stream SHALL keep that instance until it ends and its observation completes, then the instance SHALL be closed once

#### Scenario: Eviction
- **WHEN** a namespace with configured instances is evicted
- **THEN** its instances SHALL be closed after their in-flight calls and observations end

### Requirement: Extension metrics shared across instances
Every instance of one definition SHALL receive the same metrics registerer, with the extension's name as a constant label. Registering a collector that the same definition already registered SHALL succeed, so a definition that creates its collectors once can register them in every factory run. A different collector with the same descriptors SHALL still fail registration.

#### Scenario: Two namespaces register one counter
- **WHEN** a definition creates one counter and its factory registers it for two namespaces
- **THEN** both factory runs SHALL succeed, and both instances SHALL increment the same series

#### Scenario: Conflicting collector
- **WHEN** a factory creates a new collector with the same descriptors on each run
- **THEN** its second run SHALL fail registration, and that namespace SHALL apply the extension's failure policy

### Requirement: Tenant configuration without API access
When the host gives the plugin no API access, the Gateway SHALL make no reads, SHALL run every tenant-scope extension in its default state in every namespace, and SHALL log this once at startup when the build registers a tenant-scope extension.

#### Scenario: Self-hosted without API access
- **WHEN** a self-hosted host gives the plugin no API access, and the build registers a default-on and a default-off tenant extension
- **THEN** the first SHALL run and the second SHALL NOT in every namespace, and one log record SHALL state that tenant configuration is not applied

### Requirement: Tenant configuration writes refused without API access
When the plugin has no API access, admission SHALL refuse every `Middleware` create and update with `400`, before any other admission check, with a fixed reason stating that stored configuration would not be applied. Deletes SHALL stay unreviewed.

#### Scenario: Create without API access
- **WHEN** an Admin creates a valid `Middleware` on a Gateway without API access
- **THEN** the write SHALL be refused with `400` and the fixed reason

#### Scenario: Delete without API access
- **WHEN** an Admin deletes a stored `Middleware` on a Gateway without API access
- **THEN** the plugin SHALL receive no admission request, and the delete SHALL succeed

### Requirement: Operator setting excludes tenant extensions
The operator extension setting SHALL NOT configure tenant-scope extensions.

#### Scenario: Tenant extension in the operator setting
- **WHEN** the operator setting names a tenant-scope extension
- **THEN** startup SHALL fail naming it
