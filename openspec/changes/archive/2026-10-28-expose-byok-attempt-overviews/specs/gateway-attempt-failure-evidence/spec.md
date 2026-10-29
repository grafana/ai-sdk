# Spec Delta

## ADDED Requirements

### Requirement: Request-account execution attribution
The Gateway SHALL publish a compact overview for one finalized request-account (BYOK) invocation that reached at least one account attempt, using the requested `provider/model` selector and each observed attempt's native destination in account order. It SHALL NOT emit canonical, configured-instance, account or credential identity, and SHALL omit the overview when observation is incomplete or inconsistent.

#### Scenario: Direct success
- **WHEN** a BYOK call with one supplied account is selected in unary or streaming mode
- **THEN** the unary result or stream finish SHALL carry one selected attempt with the native provider and model, no `canonicalModelId` and no `providerInstance`, and the provider SHALL receive exactly one request

#### Scenario: Earlier account fails and a later account is selected
- **WHEN** the first account fails with an eligible error before unary success or any stream part and the second account is selected
- **THEN** the unary result or stream finish SHALL list a failed attempt with the first account's own native summary followed by a selected attempt, and the provider SHALL receive exactly two requests

#### Scenario: Noneligible failure
- **WHEN** the first account fails with a non-retryable error such as 401
- **THEN** the unary or setup error document SHALL carry one failed attempt with that native summary and unchanged fixed classification, and no later account SHALL be invoked

#### Scenario: Exhausted accounts
- **WHEN** every supplied account fails with an eligible error
- **THEN** the error document SHALL list every invoked account's failed attempt with its own native summary in array order, and the fixed classification SHALL remain unchanged

#### Scenario: Native preflight failure
- **WHEN** an OpenAI account returns an HTTP-200 initial SSE error before any native part
- **THEN** that attempt SHALL report failed with the summary the adapter's returned error provides, and advancement SHALL follow the default decider unchanged

#### Scenario: Rejected before any account attempt
- **WHEN** a BYOK call fails selection, credential validation or consumed native-option validation before any account is invoked
- **THEN** no overview SHALL be published and the existing fixed error document SHALL remain unchanged

#### Scenario: Observation does not match the request destination
- **WHEN** an observed attempt names a provider or model other than the request's native destination, or more attempts are observed than accounts were supplied
- **THEN** the overview SHALL be omitted rather than published

#### Scenario: Selected stream later fails
- **WHEN** a BYOK stream is selected and later emits native errors before finish or terminal failure
- **THEN** each committed error SHALL carry the overview and its event-local `nativeError`, the overview SHALL keep the attempt selected without a completion claim, and no account SHALL be replayed

## MODIFIED Requirements

### Requirement: Request-local actual execution projection

The Gateway SHALL publish a compact overview for one finalized configured direct or fallback invocation when complete attribution is available.

#### Scenario: Request-local actual execution projection policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** It SHALL use requested/canonical route identity once, ordered observed configured destinations and selected/failed/canceled outcomes with optional native failure summaries. Shared models SHALL NOT retain request execution state. Incomplete/mixed histories SHALL omit the optional overview rather than invent invocations. Projection SHALL NOT extract credentials from requests; runtime delivery SHALL apply the configured-key summary policy without filtering other provider-originated values. Account authorization, request isolation and credential exclusion from configured discovery SHALL remain intact.

#### Scenario: Primary failure followed by secondary selection
- **WHEN** a configured primary fails and a secondary is actually selected
- **THEN** the overview SHALL report those two destinations in order with the primary's own native failure and no selected index or completion claim

#### Scenario: Cancellation prevents advancement
- **WHEN** cancellation prevents another candidate from being invoked
- **THEN** no unrun destination SHALL be reported and an incomplete decision history SHALL NOT be presented as complete

#### Scenario: Concurrent calls and late callback
- **WHEN** concurrent configured or BYOK requests share models or transports, or a provider returns after request finalization
- **THEN** published attribution SHALL remain call-local, SHALL NOT mix another request's attempts, summaries or credentials, and SHALL NOT be rewritten by the late callback or native result

#### Scenario: Selection has no configured attribution
- **WHEN** a request selector returns a model without catalog-owned configured attribution or request-account attribution
- **THEN** the Gateway SHALL omit execution overviews without inventing identity or capturing a second history solely for diagnostics
- **AND** available event-local native summaries SHALL remain eligible independently of attribution; original opaque native metadata, independent SDK observers and classified error behavior SHALL remain unchanged

### Requirement: Best-effort delivery and event-local current errors

Unary result/stream finish SHALL optionally carry providerMetadata.gateway.execution.

#### Scenario: Best-effort delivery and event-local current errors policy
- **WHEN** a configured or request-account invocation is observed and its result or failure is delivered
- **THEN** Unary/setup failures SHALL optionally add top-level providerMetadata; committed errors SHALL optionally add error.data.providerMetadata and a current nativeError summary. Existing mapped message/type/code/status/retry fields SHALL preserve classification and retry behavior. Post-selection native failures SHALL remain ordered event-local data and SHALL NOT be retained in finish history. No raw diagnostic tree, separate endpoint field, new event, accessor or public error type SHALL be added. Enrichment SHALL be encoded once with the primary output and SHALL NOT be omitted for size; a failure document SHALL fall back to its fixed base document only when that encoding fails.

#### Scenario: Leading and later stream errors
- **WHEN** a selected provider emits multiple errors followed by valid content and finish
- **THEN** both clients SHALL retain each current summary in its own event, preserve error/content/finish order and expose the selected destination without finish error history or fallback replay

#### Scenario: Success overview does not fit
- **WHEN** a valid primary unary result or finish would fit the former unary or frame limit only without its overview
- **THEN** the response SHALL remain successful with the overview attached and a native `gateway` namespace relocated under `nativeMetadata`

#### Scenario: Provider-originated credential echo
- **WHEN** a native message, type or string code echoes a configured server API key
- **THEN** the Gateway SHALL replace exact matches before publishing the summary in execution metadata or event-local nativeError, independently of caller type and operator logger field policy
- **AND** caller-owned BYOK echoes SHALL remain unchanged unless they also match configured keys; status and numeric code SHALL remain unchanged

#### Scenario: Failure enrichment does not fit
- **WHEN** a classified failure would fit the former 64 KiB error limit only without its overview and current summary
- **THEN** the complete failure SHALL be emitted with them and with unchanged classification

#### Scenario: Failure enrichment cannot be encoded
- **WHEN** encoding a classified failure with its optional overview or current summary fails
- **THEN** the fixed base failure SHALL be emitted without a new diagnostic-induced error

#### Scenario: Primary output is invalid
- **WHEN** original metadata, content or usage is invalid
- **THEN** the existing adaptation failure SHALL remain effective without relocation repairing it or another candidate replaying generation

### Requirement: Real runtime and consumer evidence

Tests SHALL exercise actual configured and BYOK invocation counts through production handlers and authenticated command routes, both registered TypeScript and independent Go client access, consumer middleware, primary validation and exact complete-envelope limits. Frontend wire changes SHALL include schema-parsed cross-language assembly. Synthetic provider inputs SHALL remain focused tests, not recorded conformance evidence. Historical reviews SHALL NOT be claimed as approval of the rewrite.

#### Scenario: Both clients inspect secondary success and exhausted chain
- **WHEN** the authenticated command serves successful secondary, noneligible primary and exhausted heterogeneous routes
- **THEN** native request counts and returned candidate-local summaries SHALL agree across both clients and invocation modes without enabling capture or raw chunks

#### Scenario: Both clients inspect BYOK account attempts
- **WHEN** the authenticated Cloud command serves BYOK direct success, direct failure, failed-then-selected, noneligible and exhausted account chains with dummy keys in unary and streaming modes
- **THEN** both clients SHALL expose the same ordered attempts, outcomes and candidate-local summaries, native per-key request counts and order SHALL match the attempts, and no `canonicalModelId`, `providerInstance` or unused-account key SHALL appear
