# gateway-attempt-failure-evidence Specification

## Purpose

Define request-local Gateway execution attribution and native failure delivery through existing client-visible responses, independently of operator capture.

## Requirements

### Requirement: Request-local actual execution projection

The Gateway SHALL publish a compact overview for one finalized configured direct or fallback invocation when complete attribution is available.

#### Scenario: Request-local actual execution projection policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** It SHALL use requested/canonical route identity once, ordered observed configured destinations and selected/failed/canceled outcomes with optional native failure summaries. Shared models SHALL NOT retain request execution state. Incomplete/mixed histories SHALL omit the optional overview rather than invent invocations. Projection SHALL NOT collect credentials or suppress provider-originated scalar echoes. Account authorization, request isolation and credential exclusion from configured discovery SHALL remain intact.

#### Scenario: Primary failure followed by secondary selection
- **WHEN** a configured primary fails and a secondary is actually selected
- **THEN** the overview SHALL report those two destinations in order with the primary's own native failure and no selected index or completion claim

#### Scenario: Cancellation prevents advancement
- **WHEN** cancellation prevents another candidate from being invoked
- **THEN** no unrun destination SHALL be reported and an incomplete decision history SHALL NOT be presented as complete

#### Scenario: Concurrent calls and late callback
- **WHEN** concurrent requests share models or a provider returns after request finalization
- **THEN** published attribution SHALL remain call-local and SHALL NOT be rewritten by the late callback or native result

#### Scenario: Selection has no configured attribution
- **WHEN** a request selector returns a model without catalog-owned configured attribution
- **THEN** the Gateway SHALL omit configured execution overviews without inventing configured identity or capturing a second history solely for diagnostics
- **AND** available event-local native summaries SHALL remain eligible independently of catalog provenance; original opaque native metadata, independent SDK observers and classified error behavior SHALL remain unchanged
- **AND** missing request-account attempt or unary/setup summary observation SHALL be treated as a capability gap, not confidentiality or a permanent output prohibition

### Requirement: Independent fallback and direct observation

Fallback capture SHALL use the shared request observer without changing its model observer, decider inputs, returned errors or first-part commitment.

#### Scenario: Independent fallback and direct observation policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** Disabled, panicking or saturated operator observation SHALL NOT suppress consumer attribution. Direct calls SHALL be observed at existing invocation/result/first-part ownership boundaries without one-candidate fallback, new readers or new cleanup owners. A first error part SHALL establish selection but not stream completion. Cancellation-owned setup SHALL NOT capture an unowned late native failure.

#### Scenario: Saturated operator sink
- **WHEN** physical operator capture drops records or its callback panics
- **THEN** the request observer SHALL retain the same candidate outcomes and primary output SHALL remain unchanged

#### Scenario: Direct stream fails before a part
- **WHEN** a direct stream returns invalid setup or closes before yielding a part
- **THEN** observation SHALL report a failed actual invocation without changing its existing HTTP/SSE lifecycle or invoking another model

### Requirement: Best-effort delivery and event-local current errors

Unary result/stream finish SHALL optionally carry providerMetadata.gateway.execution.

#### Scenario: Best-effort delivery and event-local current errors policy
- **WHEN** a configured invocation is observed and its result or failure is delivered
- **THEN** Unary/setup failures SHALL optionally add top-level providerMetadata; committed errors SHALL optionally add error.data.providerMetadata and a current nativeError summary. Existing mapped message/type/code/status/retry fields SHALL preserve classification and retry behavior. Post-selection native failures SHALL remain ordered event-local data and SHALL NOT be retained in finish history. No raw diagnostic tree, separate endpoint field, new event, accessor or public error type SHALL be added. Existing source/read and envelope fit limits SHALL remain coherent until the coordinated shared transport policy replaces them.

#### Scenario: Leading and later stream errors
- **WHEN** a selected provider emits multiple errors followed by valid content and finish
- **THEN** both clients SHALL retain each current summary in its own event, preserve error/content/finish order and expose the selected destination without finish error history or fallback replay

#### Scenario: Success overview does not fit
- **WHEN** a valid primary unary result or finish fits but enriched output does not
- **THEN** the original response SHALL remain successful with native metadata unchanged, including an opaque native gateway namespace when relocation cannot fit

#### Scenario: Provider-originated credential echo
- **WHEN** a native message, type or code echoes a caller-supplied or service-owned credential
- **THEN** returned summaries SHALL preserve that value without a credential inventory or value-based suppression, independently of operator logger field policy

#### Scenario: Failure enrichment does not fit
- **WHEN** the original classified failure fits but optional overview/current summary does not
- **THEN** the original classified failure SHALL be emitted without a new diagnostic-induced error

#### Scenario: Primary output is invalid
- **WHEN** original metadata, content or usage is invalid or oversized
- **THEN** the existing adaptation failure SHALL remain effective without relocation repairing it or another candidate replaying generation

### Requirement: Real runtime and consumer evidence

Tests SHALL exercise actual configured invocation counts through production handlers and authenticated command routes, both registered TypeScript and independent Go client access, consumer middleware, primary validation and exact complete-envelope limits. Frontend wire changes SHALL include schema-parsed cross-language assembly. Synthetic provider inputs SHALL remain focused tests, not recorded conformance evidence. Historical reviews SHALL NOT be claimed as approval of the rewrite.

#### Scenario: Both clients inspect secondary success and exhausted chain
- **WHEN** the authenticated command serves successful secondary, noneligible primary and exhausted heterogeneous routes
- **THEN** native request counts and returned candidate-local summaries SHALL agree across both clients and invocation modes without enabling capture or raw chunks
