# Spec Delta

## ADDED Requirements

### Requirement: Typed Gateway request options
The client SHALL export `GatewayOptions`, a provider option for the `gateway` namespace with `Models`, `Order`, `Only`, `BYOK` and `ProviderTimeouts.BYOK`. BYOK accounts SHALL expose `APIKey`, `BaseURL`, `Organization` and `Project`. It SHALL NOT expose controls the Gateway refuses. Raw `gateway` options SHALL keep working unchanged.

#### Scenario: Typed and raw options are equivalent
- **WHEN** a call carries `GatewayOptions` with models, order, only, ordered BYOK accounts for two providers and provider timeouts
- **THEN** the request body SHALL be semantically equal to the same call with the equivalent raw `gateway` option

#### Scenario: Unset fields are absent
- **WHEN** a `GatewayOptions` field is unset
- **THEN** its member SHALL be absent from the request, and unset BYOK account fields SHALL be absent from their account object

#### Scenario: Explicit empty only is preserved
- **WHEN** `Only` is set to an empty, non-nil list
- **THEN** the request SHALL carry `"only": []`, so the Gateway applies its exclude-every-candidate rule

### Requirement: Gateway options match the pinned client
For every representable value, a call with `GatewayOptions` SHALL produce the same semantic request body and outer headers as the registered TypeScript Gateway client given the equivalent `providerOptions.gateway` object, in unary and streaming modes.

#### Scenario: Differential request capture
- **WHEN** the Go client with `GatewayOptions` and the pinned client with the equivalent `GatewayProviderOptions` value send the same call
- **THEN** their captured request bodies SHALL match, including array order, BYOK account order and provider-timeout values

### Requirement: Gateway options are not validated by the client
The client SHALL encode `GatewayOptions` without checking model IDs, slugs, account fields, timeout ranges or control combinations. It SHALL NOT apply routing itself, resolve or probe fallback models, or issue more than one Gateway request per invocation. The Gateway's documents SHALL be returned through the existing error classification.

#### Scenario: Invalid timeout reaches the Gateway
- **WHEN** a caller sets a BYOK provider timeout of 0
- **THEN** the client SHALL send it unchanged and return the Gateway's provider-timeouts error as a `GatewayError` with the invalid-request category

#### Scenario: Routed call is one Gateway request
- **WHEN** a call carries `Models` with three entries
- **THEN** the client SHALL issue exactly one Gateway request and SHALL NOT contact any listed model itself

### Requirement: Typed Gateway metadata reader
The client SHALL export a reader that decodes `gateway.execution` and `gateway.routing` from provider metadata into typed values, using ordinary Go JSON decoding. It SHALL report absence when the `gateway` namespace is missing or has neither member, and an error when a present member does not decode. Unknown members SHALL be ignored. Inputs SHALL be unchanged.

#### Scenario: Unary result with routing
- **WHEN** a unary result's metadata carries an execution overview and a routing summary
- **THEN** the reader SHALL return the requested and canonical IDs, each attempt's provider, model, instance, outcome and failure summary, and each model attempt with its provider attempts, in served order

#### Scenario: Stream finish
- **WHEN** a stream's finish part carries the overview
- **THEN** the reader SHALL decode it from that part's provider metadata

#### Scenario: Absent namespace
- **WHEN** metadata has no `gateway` namespace, or one with neither `execution` nor `routing`
- **THEN** the reader SHALL report absence without an error

#### Scenario: Malformed member
- **WHEN** `gateway.routing` is present but is not an object of the documented shape
- **THEN** the reader SHALL return an error and SHALL NOT return a partial value

#### Scenario: Large numeric failure code
- **WHEN** an attempt's failure code is a number beyond float precision, or a string
- **THEN** the reader SHALL retain it exactly, without float conversion

### Requirement: Gateway metadata reader for errors
The client SHALL export a reader that finds Gateway metadata in a returned error, using its `*provider.APICallError` data: the error document's top-level `providerMetadata` for unary and setup errors, and `error.data.providerMetadata` for stream error parts. It SHALL follow the typed reader's absence and decoding rules.

#### Scenario: All attempts failed
- **WHEN** a unary routed call fails after three failed attempts
- **THEN** the error reader SHALL return the overview with three failed attempts and their summaries, and the routing summary without a final provider

#### Scenario: Committed stream error
- **WHEN** a stream error part carries Gateway metadata in its error data
- **THEN** the error reader SHALL decode it from that part's API call error

#### Scenario: Error without Gateway metadata
- **WHEN** an error is not an API call error, or its data has no Gateway metadata
- **THEN** the error reader SHALL report absence without an error

### Requirement: Typed options keep credential capture boundaries
Typed BYOK accounts SHALL remain caller-owned request data under the existing capture guidance. The default logging redactor SHALL protect the `apiKey` of typed accounts as it does for raw options.

#### Scenario: Logged typed BYOK options
- **WHEN** Go logging captures provider options for a call with typed BYOK accounts
- **THEN** each `apiKey` SHALL be redacted, and the other account fields and `providerTimeouts.byok` SHALL be retained

### Requirement: Gateway client routing support boundary
`GatewayOptions` SHALL be documented as a Go adaptation of the supported subset of upstream `GatewayProviderOptions`. The metadata readers SHALL be documented as a Go extension, because upstream types `providerMetadata.gateway` as an open record.

#### Scenario: Reader is not routing authority
- **WHEN** a caller reads routing metadata
- **THEN** the documentation SHALL state that metadata is best-effort, that absence does not mean no attempt ran, and that a successful attempt means selection rather than completion
