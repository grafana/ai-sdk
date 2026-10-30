# Spec Delta

## ADDED Requirements

### Requirement: Typed Gateway request options
The client SHALL export `GatewayOptions`, a provider option for the `gateway` namespace with `Models`, `Order`, `Only` and `BYOK`. BYOK accounts SHALL expose `APIKey`, `BaseURL`, `Organization` and `Project`. It SHALL NOT expose controls the Gateway refuses. Raw `gateway` options SHALL keep working unchanged.

#### Scenario: Typed and raw options are equivalent
- **WHEN** a call carries `GatewayOptions` with models, order, only and ordered BYOK accounts for two providers
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
- **THEN** their captured request bodies SHALL match, including array order and BYOK account order

### Requirement: Gateway options are not validated by the client
The client SHALL encode `GatewayOptions` without checking model IDs, slugs, account fields or control combinations. It SHALL NOT apply routing itself, resolve or probe fallback models, or issue more than one Gateway request per invocation. The Gateway's documents SHALL be returned through the existing error classification.

#### Scenario: Unknown fallback model reaches the Gateway
- **WHEN** a caller's `Models` names a model the Gateway cannot find
- **THEN** the client SHALL send it unchanged and return the Gateway's fallback-model-not-found error as a `GatewayError` with the model-not-found category

#### Scenario: Routed call is one Gateway request
- **WHEN** a call carries `Models` with three entries
- **THEN** the client SHALL issue exactly one Gateway request and SHALL NOT contact any listed model itself

### Requirement: Gateway client routing support boundary
`GatewayOptions` SHALL be documented as a Go adaptation of the supported subset of upstream `GatewayProviderOptions`. The client SHALL return `gateway.execution` and `gateway.routing` unchanged as provider metadata, as upstream does, without typed readers.

#### Scenario: Routing metadata is passed through
- **WHEN** a result, stream part or error carries Gateway routing metadata
- **THEN** the client SHALL return it unchanged in `ProviderMetadata` or the `*provider.APICallError` data, and the documentation SHALL state that metadata is best-effort, that absence does not mean no attempt ran, and that a successful attempt means selection rather than completion
