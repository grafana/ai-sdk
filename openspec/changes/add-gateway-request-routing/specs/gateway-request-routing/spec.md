# Spec Delta

## Purpose

Let authorized Gateway callers route one request across their own authorized models and providers with the Vercel Gateway `models`, `order` and `only` controls, and BYOK callers bound provider start time, without adding destinations or a second request dialect.

## ADDED Requirements

### Requirement: Routing control grammar
The Gateway SHALL accept call-level `providerOptions.gateway.models`, `order` and `only` as arrays of nonempty strings, decoded with ordinary Go JSON decoding. An absent, null or empty `models` or `order` SHALL mean the control is absent. Field names and array values SHALL be compared exactly, without case folding or trimming. Controls at any other scope SHALL remain ordinary namespaces.

#### Scenario: Pinned client controls reach selection
- **WHEN** the registered TypeScript Gateway client or the Go Grafana client sends `models`, `order` and `only`
- **THEN** the selector SHALL receive them unchanged from the request body, with no custom request dialect or header

#### Scenario: Malformed control
- **WHEN** `models`, `order` or `only` is not an array, or contains a non-string or an empty string
- **THEN** the call SHALL fail with the fixed routing-controls document and zero provider requests

#### Scenario: Conditional models entry
- **WHEN** a `models` entry is an object such as `{ "model": ..., "when": ... }`
- **THEN** the call SHALL fail with the fixed routing-controls document, matching upstream's rejection of conditional entries on non-decision requests

#### Scenario: Controls never reach models
- **WHEN** a routed call invokes any candidate
- **THEN** the candidate's options, middleware and telemetry SHALL NOT contain `models`, `order`, `only` or any other call-level `gateway` member

### Requirement: Supported control set
For configured access, the accepted controls SHALL be `models`, `order` and `only`. For BYOK access, they SHALL be `byok`, `models`, `order`, `only` and `providerTimeouts` with only its `byok` member. Any other control, member or case variant SHALL fail with the fixed unsupported-control document before provider I/O.

#### Scenario: Unsupported Vercel control
- **WHEN** either access mode sends `sort`, `has`, `zeroDataRetention`, `disallowPromptTraining`, `user`, `tags`, `caching`, `serviceTier`, `quotaEntityId` or `idempotencyKey`
- **THEN** the call SHALL fail with the unsupported-control document and SHALL NOT silently ignore the control

#### Scenario: Credential control from a configured caller
- **WHEN** a configured caller sends `byok` or `providerTimeouts`
- **THEN** the call SHALL fail with the unsupported-control document, with zero catalog resolutions and zero provider requests

#### Scenario: Unsupported control document
- **WHEN** the unsupported-control document is written
- **THEN** its message SHALL list the supported controls and SHALL NOT echo the rejected control's name or value

### Requirement: Model fallback composition
The effective chain SHALL be the requested model's candidates followed by each `models` entry's candidates, in array order. Each model's candidates SHALL keep that model's own order after filtering and preference. Later models SHALL be attempted only through the existing fallback decider, after every remaining candidate of earlier models.

#### Scenario: Configured primary and fallback model
- **WHEN** the requested route has candidates A1 and A2, `models` names route B with candidate B1, and A1 and A2 fail eligibly before commitment
- **THEN** candidates SHALL run once each in the order A1, A2, B1

#### Scenario: Primary succeeds
- **WHEN** the requested model's first candidate is selected
- **THEN** no `models` entry SHALL be invoked

#### Scenario: Noneligible failure stops the chain
- **WHEN** a candidate fails with an error the default decider does not retry
- **THEN** no later candidate of the same or a later model SHALL run

#### Scenario: Each call starts from its own chain
- **WHEN** two calls to the same route carry different routing controls
- **THEN** each call SHALL use only its own composed order, and the route's configured order SHALL remain unchanged for calls without controls

### Requirement: Exclusive provider filter
When `only` is present, each model's candidates SHALL be restricted to those whose provider slug appears in `only`, before preference is applied. A model with no remaining candidates SHALL be skipped. An empty `only` SHALL exclude every candidate.

#### Scenario: Filter removes a provider
- **WHEN** a route has openai then anthropic candidates and `only` is `["anthropic"]`
- **THEN** only the anthropic candidate SHALL be invoked

#### Scenario: Filter skips a whole model
- **WHEN** the requested route has only openai candidates, `models` names an anthropic-backed route and `only` is `["anthropic"]`
- **THEN** the chain SHALL start with the fallback route's anthropic candidate

#### Scenario: Filter excludes every candidate
- **WHEN** `only` leaves no candidate in any model, including when `only` is empty
- **THEN** the call SHALL fail with the fixed no-candidates document and zero provider requests

### Requirement: Provider preference
When `order` is present, each model's remaining candidates whose provider slug appears in `order` SHALL move to the front in `order`'s sequence. Candidates sharing a slug, and candidates whose slug is not listed, SHALL keep their configured relative order. `order` SHALL NOT remove candidates.

#### Scenario: Preference reorders without filtering
- **WHEN** a route has candidates openai, anthropic and compatible-x, and `order` is `["anthropic"]`
- **THEN** the attempt order SHALL be anthropic, openai, compatible-x

#### Scenario: Filter then preference
- **WHEN** `only` is `["anthropic", "openai"]` and `order` is `["openai"]` on a route with anthropic, compatible-x and openai candidates
- **THEN** the attempt order SHALL be openai, anthropic

#### Scenario: Unlisted slug
- **WHEN** `order` or `only` names a slug that no candidate has
- **THEN** the slug SHALL have no effect and SHALL NOT fail the call by itself

### Requirement: Provider slug identity
A candidate's provider slug SHALL be the `provider` value that configured discovery and `gateway.execution` report for it, meaning the provider type or the configured compatible provider name. For BYOK, it SHALL be the selector's provider prefix. Provider instance names SHALL NOT act as slugs.

#### Scenario: Instance name is not a slug
- **WHEN** `only` names a configured provider instance rather than its provider
- **THEN** that instance's candidates SHALL be excluded unless their provider slug also matches

### Requirement: Repeated models and candidates
A `models` entry that resolves to a model already in the chain, including the requested model or the same route through an alias, SHALL be skipped. A candidate whose provider instance and backend model already appear earlier in the composed chain SHALL be skipped. BYOK selectors SHALL be compared exactly.

#### Scenario: Alias of the requested route
- **WHEN** `models` names an alias of the requested route
- **THEN** the route's candidates SHALL appear once

#### Scenario: Shared candidate across routes
- **WHEN** two routes in the chain share a provider instance and backend model
- **THEN** that destination SHALL be attempted at most once per call

### Requirement: Authorized destinations only
Routing SHALL use only destinations the caller can already select directly. Configured callers SHALL name public route IDs or aliases from their catalog. Every candidate SHALL use its startup-constructed account and endpoint. Controls SHALL NOT supply credentials, endpoints, provider instances, backend model IDs or another access mode's accounts.

#### Scenario: Backend model ID named as a fallback
- **WHEN** a configured caller's `models` entry names a backend model ID or provider instance that is not a public route ID or alias
- **THEN** the call SHALL fail with the fixed fallback-model-not-found document and zero provider requests

#### Scenario: BYOK selector from a configured caller
- **WHEN** a configured caller's `models` entry is a `provider/model` string that is not a public route ID or alias
- **THEN** it SHALL be treated as an unknown fallback model, and no request account SHALL be constructed

#### Scenario: Configured route from a BYOK caller
- **WHEN** a BYOK caller's `models` entry names a configured public route ID or alias
- **THEN** it SHALL be decoded only as a BYOK selector, and no configured account SHALL become eligible

### Requirement: BYOK fallback models
For BYOK access, each `models` entry SHALL be a `provider/model` selector with the same grammar as the requested selector. Its candidates SHALL be the caller's supplied accounts for that provider, in array order. `order` SHALL have no effect within a BYOK model, which has one provider. No configured or system credential SHALL be used as a fallback.

#### Scenario: Cross-provider BYOK fallback
- **WHEN** the requested selector is `anthropic/x`, `models` is `["openai/y"]`, `byok` supplies one key for each, and the Anthropic attempt fails eligibly
- **THEN** the OpenAI account SHALL be attempted with model `y` and only its own key

#### Scenario: Fallback provider without accounts
- **WHEN** a `models` entry names a supported provider for which `byok` supplies no accounts
- **THEN** the call SHALL fail with the fixed BYOK credentials document and zero provider requests

#### Scenario: Invalid fallback selector
- **WHEN** a BYOK `models` entry has no provider/model split, an unsupported provider or an empty model
- **THEN** the call SHALL fail with the fixed BYOK selector document and zero provider requests

### Requirement: Upfront routing diagnostics
Routing failures that are known from the request and the caller's catalog SHALL be decided before any provider request. Malformed controls, unknown fallback models, filters that exclude every candidate and composed chains above the attempt bound SHALL each use their own fixed document, which never echoes supplied values.

#### Scenario: Unknown fallback model
- **WHEN** a configured `models` entry is not a public route ID or alias
- **THEN** the response SHALL be HTTP 404 model_not_found with the fixed fallback-model-not-found message

#### Scenario: Diagnostic precedence
- **WHEN** a request has an unsupported control, malformed routing controls, an unknown requested model and an unknown fallback model at once
- **THEN** the call SHALL fail with the first applicable of: unsupported control, routing controls, the existing model-not-found document for the requested model, fallback model not found, no candidates, attempt bound

#### Scenario: Diagnostics carry no overview
- **WHEN** a routing diagnostic ends the call
- **THEN** no execution overview or routing metadata SHALL be published, because no candidate was invoked

### Requirement: Composed chain attempt bound
The composed chain, after filtering and skipping repeats, SHALL contain at most the shared bound of 8 provider attempts. A longer chain SHALL fail before any provider request and SHALL NOT be truncated.

#### Scenario: Chain at and above the bound
- **WHEN** the composed chain has 8 candidates, or 9 candidates
- **THEN** the first SHALL run normally and the second SHALL fail with the fixed attempt-bound document and zero provider requests

### Requirement: Unchanged execution without routing controls
A call without `models`, `order`, `only` or `providerTimeouts` SHALL execute exactly as before this change: configured calls use their startup-composed route model and BYOK calls use their selected provider's accounts.

#### Scenario: Plain configured call
- **WHEN** a configured caller sends no gateway controls
- **THEN** the call SHALL invoke the prebuilt route with its configured order and no per-request chain construction

### Requirement: One logical chain per routed call
A routed call SHALL run all physical attempts beneath one logical middleware chain. Its logical identity SHALL be the requested model's canonical public ID for configured calls, and the requested selector for BYOK calls. Fallback models SHALL NOT become the logical identity or add logical observations.

#### Scenario: Fallback model serves the call
- **WHEN** a routed configured call is served by a `models` entry's candidate
- **THEN** logical logs, metrics and Agent Observability SHALL record one call under the requested route's canonical ID

#### Scenario: Private physical records
- **WHEN** a routed configured call attempts candidates from two routes
- **THEN** each attempt SHALL produce one private physical record with its composed candidate index, provider instance and backend model, correlated with the one logical call

### Requirement: Candidate-native request semantics across routed attempts
Every routed attempt SHALL receive the same mapped call options and read only its own native namespace through the existing per-attempt translation and native validation. The Gateway SHALL NOT intersect, translate or drop namespaces because candidates differ, and SHALL NOT refuse mapper-supported content because the chain is routed.

#### Scenario: Heterogeneous attempts consume their own options
- **WHEN** a routed chain spans Anthropic, OpenAI and compatible candidates, and the request carries per-provider reasoning and tool options at supported scopes
- **THEN** each fake native request SHALL contain only the values its adapter consumes from its own namespace, at the same scopes as a direct call

#### Scenario: Supported content through a routed chain
- **WHEN** a supported tool, file, reasoning or structured-output request is routed with `models`
- **THEN** it SHALL be admitted under the same protections as a direct or configured-fallback call

### Requirement: Routed commitment and ownership
Routing SHALL reuse the existing fallback selection: a unary result or the first stream part commits a candidate, and no later candidate SHALL run after commitment. Cancellation, the shared deadline, bounded cleanup and the absence of replay or exactly-once claims SHALL be unchanged.

#### Scenario: Committed stream error
- **WHEN** a routed stream's selected candidate emits an error part, first or later
- **THEN** the error SHALL be relayed in order and no later model or provider SHALL run

#### Scenario: Cancellation during a routed chain
- **WHEN** the request is canceled while an attempt is in progress
- **THEN** no later attempt SHALL start and the abandoned attempt SHALL be cleaned up within the existing drain bound

### Requirement: BYOK provider start timeout
`providerTimeouts.byok` SHALL map supported BYOK provider slugs to whole milliseconds from 1 to 600000. Each attempt for a listed provider SHALL fail as an eligible timeout if its provider has not sent the first response byte by then. The bound SHALL have no effect afterwards.

#### Scenario: Slow account falls through
- **WHEN** `providerTimeouts.byok.anthropic` is 2000, the first Anthropic account sends no response bytes for 2000 ms and a second account answers
- **THEN** the first attempt SHALL be canceled and recorded as failed with a timeout summary, and the second SHALL be selected

#### Scenario: Timeout after the provider started responding
- **WHEN** a provider sends its first response byte before the timeout and then takes longer than the timeout to finish a unary result or stream
- **THEN** the attempt SHALL continue, under the request deadline and stream idle limit only

#### Scenario: Last attempt times out
- **WHEN** the last candidate's provider start timeout expires
- **THEN** the call SHALL fail with the timeout category and carry the overview of all attempts

#### Scenario: Invalid timeout map
- **WHEN** `providerTimeouts.byok` names an unsupported provider, or a value that is not a whole number from 1 to 600000
- **THEN** the call SHALL fail with the fixed provider-timeouts document and zero provider requests

#### Scenario: Request cancellation is not a provider timeout
- **WHEN** the request is canceled before a provider start timeout expires
- **THEN** the call SHALL end with the cancellation category and no later attempt SHALL start

### Requirement: Routing observation independence
Routing metadata returned to the caller SHALL NOT depend on server observability capture settings. Consumer middleware around the Go Grafana client SHALL observe returned routing and native metadata through its own configuration. Server capture policy SHALL NOT strip returned routing or provider data.

#### Scenario: Metadata-only server capture
- **WHEN** a routed call runs with server payload capture disabled
- **THEN** the caller's response SHALL carry the same routing, execution and native metadata as with capture enabled

### Requirement: Routing evidence and support boundary
Evidence SHALL use both registered clients against the real command with fake native providers, and SHALL assert actual native requests and attempt order. Synthetic routing faults SHALL stay in focused tests, not recorded fixtures. Vercel service behavior SHALL be reported as design reference, separately from pinned-client compatibility evidence.

#### Scenario: Both clients route through the command
- **WHEN** configured-only, request-only and combined routing scenarios run through the pinned TypeScript client and the Go client, in unary and streaming modes, for configured and BYOK access
- **THEN** both clients SHALL observe the same attempt order, native requests, routing metadata and diagnostics

#### Scenario: Account boundary evidence
- **WHEN** BYOK and configured callers each send controls naming the other population's destinations
- **THEN** tests SHALL prove that no credential, account or endpoint of the other population was used, and SHALL NOT present fake accounts as BYOK provisioning proof

#### Scenario: Support boundary is documented
- **WHEN** a reader compares this Gateway with Vercel's service
- **THEN** the Gateway documentation SHALL state the supported controls, slug identity, the absence of system-credential fallback for BYOK, and the refused controls
