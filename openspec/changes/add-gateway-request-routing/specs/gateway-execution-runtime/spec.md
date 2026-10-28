# Spec Delta

## MODIFIED Requirements

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
- **THEN** it SHALL apply `models`, `order` and `only` under gateway-request-routing, and SHALL fail any other control with the unsupported-gateway-control category, with zero model calls

### Requirement: Closed failure classification
The runtime SHALL classify every failure into one closed category: invalid request, invalid routing, BYOK credentials, BYOK selector, unsupported gateway control, model not found, permission, rate limit, overload, failed dependency, upstream, timeout, cancellation or internal. Invalid request MAY carry a closed reason: reserved provider options or protected call header. Invalid routing SHALL carry one closed reason: routing controls, provider timeouts, no candidates or attempt bound. Model not found MAY carry the fallback-model reason. Classification SHALL never panic.

#### Scenario: Selection errors
- **WHEN** selection fails with account access denied, invalid BYOK credentials, an invalid BYOK selector, an unsupported gateway control, reserved provider options or an unknown model
- **THEN** the category SHALL be permission, BYOK credentials, BYOK selector, unsupported gateway control, invalid request with the reserved reason, or model not found respectively, and any other error SHALL be classified as a provider error

#### Scenario: Routing selection errors
- **WHEN** selection fails with malformed routing controls, an invalid provider timeout map, a filter that excludes every candidate, a composed chain above the attempt bound or an unknown fallback model
- **THEN** the category SHALL be invalid routing with the routing-controls, provider-timeouts, no-candidates or attempt-bound reason respectively, or model not found with the fallback-model reason

#### Scenario: Provider errors
- **WHEN** invocation or setup fails
- **THEN** an unsupported request or unsupported functionality SHALL be invalid request, context cancellation SHALL be cancellation, an expired deadline SHALL be timeout, provider status 408 or 504 SHALL be timeout, 429 rate limit, 503 or 529 overload, another 4xx failed dependency and any other status upstream
- **AND** a transport error SHALL be timeout when it reports a timeout and upstream otherwise, and any other error SHALL be internal
- **AND** an expired BYOK provider start timeout SHALL be timeout

#### Scenario: In-band stream errors
- **WHEN** a stream error part carries a provider error with no status
- **THEN** it SHALL be cancellation, timeout, the transport category or upstream, and a status outside 100–599 SHALL be internal

#### Scenario: Adapter documents
- **WHEN** ProviderWire encodes a failure
- **THEN** it SHALL choose its fixed document from the category and reason alone, and every document other than the unsupported-gateway-control message and the new routing documents SHALL equal those written before this change

### Requirement: Request-local attempt capture
Every selection SHALL get its own attempt capture, scoped to the call's context. Attempts observed after the capture is sealed SHALL be ignored. Configured and BYOK selections SHALL carry their ordered model groups, each with its requested ID and expected destinations, and SHALL publish an overview checked against them. Selections with no attribution SHALL NOT publish an overview.

#### Scenario: Concurrent calls are isolated
- **WHEN** two calls run concurrently on one runtime with different fallback outcomes
- **THEN** each call's capture SHALL contain only its own attempts

#### Scenario: BYOK attempts
- **WHEN** a BYOK call tries two accounts and the second is selected
- **THEN** the capture SHALL hold a failed then a selected attempt, and the response SHALL carry an overview with the requested selector, no canonical identity and those two native attempts in order

#### Scenario: Routed configured attempts
- **WHEN** a configured call routed across two models tries one candidate of each and the second is selected
- **THEN** the overview SHALL list both attempts with their configured identities and the requested model's canonical ID, and the routing summary SHALL group them by model

#### Scenario: Routed BYOK attempts
- **WHEN** a BYOK call routed across `anthropic/x` and `openai/y` observes an attempt whose native identity matches neither group's destination at its position
- **THEN** no overview SHALL be published

#### Scenario: Attempt after sealing
- **WHEN** an attempt is observed after the call's outcome was decided
- **THEN** it SHALL NOT appear in the overview

#### Scenario: Selection without attribution
- **WHEN** a selector returns a valid selection with neither configured nor request-account attribution and its model reports fallback attempts
- **THEN** the attempts SHALL be captured and sealed, and no overview SHALL appear in the response

#### Scenario: Test adapter sees BYOK attempts
- **WHEN** the test-only adapter and ProviderWire run the same BYOK fake-model scenarios
- **THEN** both SHALL report the same attempt view, including omitted canonical identity

### Requirement: Access-aware selection
One selector SHALL serve every access mode, so each adapter needs one runtime. It SHALL branch on the verified caller's account access. Configured access SHALL resolve through the catalog and apply routing controls under gateway-request-routing. BYOK access SHALL decode request accounts and routing controls from those controls. Any other access SHALL fail with the permission category.

#### Scenario: Configured access
- **WHEN** a configured caller calls a catalog model without `gateway` controls
- **THEN** the selector SHALL resolve it through the catalog and invoke only configured accounts

#### Scenario: Configured routed access
- **WHEN** a configured caller sends `models`, `order` or `only`
- **THEN** the selector SHALL resolve every named model through the catalog and compose only configured candidates

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
