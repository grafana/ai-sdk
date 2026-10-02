## MODIFIED Requirements

### Requirement: Ordered direct text routes
The Gateway SHALL construct each public route from one required primary and zero or more ordered direct provider/backend candidates. Every invocation SHALL begin at the primary and consider later candidates only in configured order; no prior winner, request value, health signal, or concurrent call SHALL reorder or skip candidates. Configured fallback SHALL admit every request capability already supported by the strict mapper for that invocation mode, subject to the existing protocol and consumption-backed native protections, without an additional text-only eligibility policy.

#### Scenario: Later call follows original order
- **WHEN** one call is served by a fallback candidate and a later call uses the same canonical route
- **THEN** the later call SHALL begin again with the configured primary

#### Scenario: Client attempts to select a backend
- **WHEN** a request carries an unrecognized or reserved provider/fallback selection control
- **THEN** existing strict validation or host policy SHALL reject or ignore it according to its owning contract and SHALL NOT alter candidate order

#### Scenario: Mapped capability does not change ordering
- **WHEN** a supported function-tool, file, reasoning or ordinary-options request encounters eligible pre-commit failures in a three-candidate route
- **THEN** invoked candidates SHALL follow configured order with no capability-based reordering, silent primary-only execution or request mutation

### Requirement: Deterministic fallback and privacy evidence
Automated evidence SHALL cover unary setup selection, streaming setup selection, premature EOF, leading and later provider error parts, multiple ordered error parts, non-retryable failures, cancellation races, nil/invalid results, result-plus-error and late setup, blocked downstream consumers, silent and continuously ready abandoned providers, time/part-bounded cleanup, observer panic/saturation, recursion-shaped YAML rejection, duplicate/missing references, one logical record, physical winner correlation, and the existing credential/observation protections. Both registered TypeScript and independent Go clients SHALL exercise direct and configured-fallback unary and streaming requests for representative supported function definitions/history/choices, file arms/presence, reasoning and headers/options. Fake native requests SHALL prove candidate-specific option consumption without cross-provider translation or route-wide intersections. Tests SHALL use deterministic fake models/transports and focused race tests; they SHALL NOT present synthetic provider inputs as recorded conformance evidence or claim live acceptance or full output-derived continuation.

#### Scenario: Fallback verification runs
- **WHEN** the root and isolated Gateway verification suites run with the committed module boundary
- **THEN** focused tests SHALL prove selection, ordering, lifecycle bounds, logical/physical separation and applicable privacy without changing the registered upstream baseline or public fixtures unnecessarily

#### Scenario: Both clients reach native candidates
- **WHEN** equivalent representative mapped requests run through both clients, both invocation modes and direct/configured-fallback routes
- **THEN** model-boundary assertions SHALL retain supported selections/presence/scopes and fake native requests SHALL prove existing adapter-specific consumption
- **AND** unrelated namespaces SHALL NOT be translated, intersected or promoted by the Gateway

#### Scenario: Concurrent requests share immutable models
- **WHEN** concurrent requests with distinct scoped options/history use the same constructed candidates and logical chain
- **THEN** their values and correlation SHALL remain isolated, original mapped inputs SHALL remain unchanged and each invocation SHALL have one logical observation

## ADDED Requirements

### Requirement: Mapped capability fallback boundary
Configured candidates SHALL compose directly through reusable fallback beneath the single logical middleware chain. Every attempted candidate SHALL receive the same supported mapped CallOptions, retaining function definitions/history/choices, ordinary file selections and filename presence, reasoning content/controls, ordinary headers and opaque options at their original supported scopes. Empty namespace objects and active members including null, false, zero, empty strings, arrays and nested objects SHALL NOT create a separate fallback restriction. Native adapters SHALL retain their own namespace interpretation and concrete consumption-backed protections; mapped eligibility SHALL NOT imply acceptance by every backend. The Gateway SHALL NOT filter to a candidate intersection, translate native options or replace option loss with blanket refusal.

Existing fallback ordering, decider eligibility, cancellation, attempt observation and bounded lifecycle ownership SHALL remain unchanged. Unary successful results SHALL select the candidate. Any first provider stream part, including stream-start, error or tool events, SHALL irrevocably select the candidate and SHALL be relayed exactly once in original order. No read-ahead beyond that part, post-selection codec failure, later provider error or unfinished output SHALL trigger another candidate. The Gateway SHALL NOT add a local function executor, parallel channel owner or stream replay. Consumer execution counts SHALL NOT imply an exactly-once provider guarantee.

#### Scenario: Scoped native options reach all attempts
- **WHEN** a mapped request carries options for multiple ordinary native namespaces at supported call/message/content/function-tool/file-result scopes and a primary fails eligibly before commitment
- **THEN** each attempted candidate SHALL receive those options unchanged at the same scopes and consume only according to its existing native namespace rules

#### Scenario: Empty and active ordinary namespaces remain eligible
- **WHEN** an otherwise supported request carries empty namespace objects or nested ordinary JSON containing null, false, zero, empty strings, arrays or objects
- **THEN** configured fallback SHALL NOT reject the request on semantic-emptiness or namespace-inventory grounds

#### Scenario: Setup failure and empty pre-part EOF
- **WHEN** setup fails eligibly or a candidate channel closes before any part while the request remains live
- **THEN** reusable fallback SHALL evaluate the failure using its existing decider and advance only to the next configured candidate when eligible

#### Scenario: Noneligible failure or cancellation
- **WHEN** a mapped request encounters a noneligible failure or cancellation before selection
- **THEN** no later candidate SHALL be invoked and cancellation SHALL preserve the existing context cause and bounded cleanup

#### Scenario: Stream-start or error commits the candidate
- **WHEN** a candidate emits stream-start or a provider error as its first part and later fails or closes
- **THEN** that first part SHALL select the candidate without looking ahead and no later candidate SHALL run

#### Scenario: Selected tool output cannot be replayed
- **WHEN** a selected unary result or streaming candidate supplies a function call and response adaptation subsequently fails or later provider output errors
- **THEN** configured fallback SHALL NOT re-enter selection or replay the call on another candidate

#### Scenario: Deferred or protected request remains refused
- **WHEN** a request activates an unsupported provider-tool, provider-executed history, approval, raw/custom/generated/structured codec, unconsumed host control or concrete native bypass
- **THEN** its existing owning validation/protection boundary SHALL remain effective before prohibited native I/O
- **AND** removal of the text guard SHALL NOT enable the deferred feature

#### Scenario: Capture policy does not govern admission
- **WHEN** a supported tool/file/reasoning/options request runs through configured fallback under metadata-only operator capture
- **THEN** capture policy SHALL NOT reject the mapped content or alter supported responses and one logical observation SHALL cover the physical attempts

## REMOVED Requirements

### Requirement: Text-only effect boundary
**Reason**: Issue #319 replaces categorical text-only admission with existing mapped-capability support and reusable first-part commitment; consumer function execution remains outside the Gateway.
**Migration**: Use Mapped capability fallback boundary and the unchanged reusable fallback-stream-error contract. Do not add replay, new codecs or exactly-once provider claims.

### Requirement: Direct tools do not enable effectful fallback
**Reason**: Supported function definitions, choices and history are no longer categorically refused on configured fallback routes.
**Migration**: Apply the same existing function mapping to direct/fallback routes and retain first-part/unary-success commitment, consumer ownership and deferred-family protections.

### Requirement: Empty message options retain text fallback eligibility
**Reason**: Semantic emptiness is no longer a fallback admission criterion for ordinary options at any supported scope.
**Migration**: Forward opaque supported options unchanged to every attempted candidate; keep reserved namespaces and concrete native bypass protections at their owning boundaries.
