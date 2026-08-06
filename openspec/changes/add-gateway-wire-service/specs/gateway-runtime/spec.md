## ADDED Requirements

### Requirement: Runtime accepts a normalized gateway call

The repository SHALL provide a public `gateway/runtime` package whose generate and stream operations accept a `GatewayCall`. `GatewayCall` SHALL contain a typed `Protocol`, immutable original `RequestedModelID`, normalized `provider.CallOptions`, parsed `GatewayOptions`, immutable trusted `CallMetadata`, and separately identified policy-derived metadata. The package SHALL define `ProtocolLanguageModelV4`; future protocol constants MAY be added by the façades that use the runtime.

`GatewayOptions` SHALL represent the full registered `@ai-sdk/gateway@4.0.33` control namespace before provider invocation, including BYOK credentials, caching/privacy/capability filters, model fallbacks, provider allowlists/order, provider timeouts, quota/user/tags, service tier/sort intent, and service-owned extensions. Because the pinned type has a string index signature, unknown gateway keys SHALL be retained as valid opaque JSON in an extension map; policy or resolution MAY accept or reject them but codecs MUST NOT discard them. Credential-bearing and attribution fields SHALL remain private control data and MUST NOT be copied to errors, response metadata, or provider options.

`CallMetadata` SHALL contain a non-empty gateway request ID and host-supplied authenticated attributes. Runtime construction and typed accessors SHALL defensively copy metadata maps. Caller request-body values and provider headers MUST NOT become trusted metadata automatically. Policy-derived metadata SHALL remain separate and MUST NOT overwrite the request ID or authenticated attributes.

The runtime MUST NOT import `net/http`, provider-wire DTOs, or public façade packages. It SHALL default its total invocation timeout to 120 seconds, provide a positive option to override it, and reject nil required dependencies, nil options, and non-positive configured timeouts.

#### Scenario: LanguageModelV4 call is normalized

- **WHEN** the strict V4 adapter decodes a valid request
- **THEN** it SHALL invoke the runtime with `ProtocolLanguageModelV4`, the exact public model ID, provider call options, separately parsed gateway options, and trusted call metadata

#### Scenario: Caller values are not trusted metadata

- **WHEN** a request body or provider header claims a tenant, project, or gateway request ID
- **THEN** the runtime SHALL NOT treat it as authenticated metadata unless the host metadata source explicitly supplies that value

#### Scenario: Missing request ID is rejected

- **WHEN** a non-HTTP caller invokes the runtime with empty `CallMetadata.RequestID`
- **THEN** the runtime SHALL reject the call before policy or resolution

#### Scenario: Metadata maps are defensively copied

- **WHEN** the host mutates its source attribute map after constructing a call or a middleware mutates a returned map
- **THEN** stored trusted metadata and values observed by other middleware SHALL remain unchanged

#### Scenario: Runtime dependency boundary

- **WHEN** runtime imports and public types are inspected
- **THEN** they SHALL NOT import or expose `net/http`, `gateway/providerwire`, `gateway/providerwire/v4`, OpenAI, Anthropic, or frontend wire types

#### Scenario: Default total timeout

- **WHEN** runtime construction omits a total-timeout option
- **THEN** generate and stream invocations SHALL use a 120-second total timeout

### Requirement: Gateway options do not leak to providers

Gateway-owned options SHALL be removed from `provider.CallOptions.ProviderOptions` before model middleware or provider invocation. A call with gateway options MUST pass through policy and a call-aware resolver that can inspect them. The default adapter for `catalog.ModelResolver` SHALL support calls without additional routing controls and SHALL reject non-empty controls it cannot honor rather than silently ignoring or forwarding them.

#### Scenario: Gateway provider option is extracted

- **WHEN** a canonical request contains `providerOptions.gateway`
- **THEN** its validated value SHALL populate `GatewayCall.GatewayOptions` and the provider-bound options map SHALL not contain the `gateway` key

#### Scenario: Unknown gateway extension is retained

- **WHEN** `providerOptions.gateway` contains an unknown key with valid JSON
- **THEN** the value SHALL remain byte-equivalent opaque JSON in `GatewayOptions.Extensions` for policy/resolution to accept or reject

#### Scenario: Unsupported routing control fails closed

- **WHEN** the default catalog adapter receives a call with a routing control it cannot honor
- **THEN** it SHALL return a classified invalid or unsupported call failure before provider invocation

#### Scenario: Provider does not receive gateway controls

- **WHEN** policy and resolution accept a call containing gateway controls
- **THEN** model middleware and the provider SHALL receive only provider-bound options, never the gateway control namespace

### Requirement: Ordered call policy runs before resolution

The runtime SHALL accept an ordered set of call policies. Each policy SHALL receive the context and normalized `GatewayCall` and MAY reject the call or return transformed call options, gateway options, and separate policy-derived metadata. Policies MUST NOT change `Protocol`, original `RequestedModelID`, gateway request ID, or host-authenticated attributes. Every policy SHALL run before the call-aware resolver and before model middleware. This seam SHALL permit inspection and rejection of caller-controlled provider headers, downstream `Authorization` overrides, provider options, and request-specific routing/fallback controls. Host authentication remains outside the runtime.

#### Scenario: Prohibited downstream authorization is rejected

- **WHEN** call options contain a caller-controlled provider `Authorization` header prohibited by policy
- **THEN** policy SHALL reject the call before catalog resolution or provider invocation

#### Scenario: Policy transforms provider-bound input

- **WHEN** an ordered policy removes an allowed caller header or normalizes a gateway routing option
- **THEN** later policies and the resolver SHALL observe the transformed call

#### Scenario: Policy failure bypasses resolution

- **WHEN** a policy returns a forbidden or invalid-call failure
- **THEN** no resolver, model middleware, or provider method SHALL run and the private cause SHALL remain classifiable

### Requirement: Call-aware resolution preserves invocation identity

The runtime SHALL resolve the post-policy `GatewayCall` through a call-aware resolver returning `catalog.ResolvedModel`. Every successful resolution SHALL produce immutable invocation identity with `RequestedModelID`, `CanonicalModelID`, `ResolvedProviderID`, and `ResolvedModelID`. The requested value SHALL remain the exact original public ID, the canonical value SHALL be `catalog.ResolvedModel.ID`, and the resolved values SHALL be read from the resolved model before runtime middleware is attached.

Resolved identity is model-reported routing identity and MUST NOT be described as the backend attempt that actually executed. Middleware overrides and fallback selection MAY differ from these captured values. Runtime outcomes SHALL preserve all available identity on every success or failure after resolution. A nil resolved model SHALL become an internal failure without model invocation.

#### Scenario: Alias resolves to canonical route

- **WHEN** requested ID `fast` resolves to canonical ID `chat-small`
- **THEN** invocation identity SHALL preserve both public values without replacing them with a provider model ID

#### Scenario: Fallback identity is not overclaimed

- **WHEN** a resolved fallback model reports the provider and model ID of its first candidate
- **THEN** identity SHALL preserve those values as model-reported and SHALL NOT claim they identify the candidate that ultimately executed

#### Scenario: Post-resolution failure retains identity

- **WHEN** resolution succeeds and `DoGenerate` or `DoStream` subsequently fails
- **THEN** the runtime outcome SHALL retain requested, canonical, and resolved identity alongside the classified error

### Requirement: Gateway metadata reaches model middleware context

Before entering runtime-configured model middleware, the runtime SHALL enrich the invocation context with typed, read-only accessors for protocol, gateway request ID, requested public model ID, canonical public model ID, immutable host-authenticated attributes, and separately identified policy-derived metadata. Map-returning accessors SHALL return defensive copies. It MUST NOT expose caller-controlled headers as authenticated context.

Protocol adapters SHALL apply their own public identity semantics. Strict LanguageModelV4 SHALL omit `response.modelId` when backend identity is private because that field means the actual model used; it MUST NOT substitute a catalog alias. A future Chat adapter MAY use canonical public identity for its public `model` field.

#### Scenario: Middleware reads stable gateway identity

- **WHEN** a model middleware hook runs after resolution
- **THEN** typed runtime context accessors SHALL return protocol, request ID, requested model ID, canonical model ID, immutable host-authenticated attributes, and distinct policy metadata

#### Scenario: Strict V4 does not mislabel model identity

- **WHEN** backend model ID is private and differs from the canonical catalog ID
- **THEN** the strict V4 adapter SHALL omit `response.modelId` rather than inserting either private backend identity or the semantically incorrect catalog alias

### Requirement: Runtime-configured model middleware is attached once

For each resolved invocation the runtime SHALL attach its ordered `[]middleware.Middleware` chain once, preserve declared outer-to-inner order, and enter the selected generate or stream operation once. The runtime SHALL NOT claim to detect middleware already embedded in the catalog model and SHALL NOT prevent a middleware hook from deliberately calling an inner closure more than once.

#### Scenario: Generate enters middleware once

- **WHEN** one runtime generate call resolves a model and configured middleware counts entry
- **THEN** the runtime SHALL attach the chain once and enter its generate path once

#### Scenario: Stream enters middleware once

- **WHEN** one runtime stream call resolves a model and configured middleware counts entry
- **THEN** the runtime SHALL attach the chain once and enter its stream path once

#### Scenario: Middleware order is preserved

- **WHEN** the runtime is configured with middleware A, B, and C
- **THEN** parameter transformation and wrapping SHALL follow the existing middleware contract with A outermost and C closest to the resolved model

### Requirement: Unary invocation lifecycle

A runtime generate call SHALL apply policy, resolve once, invoke `DoGenerate` synchronously once through configured model middleware, and return an outcome containing invocation identity and an optional provider result. The configured total timeout SHALL begin after successful resolution and make the supplied context done when it expires. A nil result without an error SHALL become an internal failure. Provider and middleware errors SHALL be classified at the runtime boundary while preserving private causes and identity.

The provider remains responsible for context cooperation. The runtime MUST NOT spawn an unbounded goroutine to force `DoGenerate` to return.

#### Scenario: Successful generate invocation

- **WHEN** the resolved model returns a non-nil generate result
- **THEN** the runtime SHALL return that result with invocation identity and no error

#### Scenario: Cooperative generate timeout

- **WHEN** `DoGenerate` observes the configured timeout and returns
- **THEN** its context SHALL be canceled and the runtime failure classification SHALL have timeout kind and retryability derived at that boundary

#### Scenario: Provider blocks during generate

- **WHEN** a provider blocks synchronously inside `DoGenerate` and ignores its context
- **THEN** the deadline SHALL make the supplied context done, but the runtime call MAY remain blocked until the provider returns and SHALL NOT leak an additional runtime-owned invocation goroutine

### Requirement: Stream invocation remains minimal and ordered

A successful runtime stream call SHALL return an adapter-facing invocation containing immutable identity, a single-consumer receive-only `Parts` channel, `Wait() error`, and idempotent `Cancel(error)`. It SHALL invoke `DoStream` once through configured model middleware. Provider parts, including multiple `PartError` values, SHALL be forwarded unchanged and in order. `Wait` SHALL report runtime lifecycle termination only and MUST NOT report a provider-emitted `PartError` as terminal failure.

The adapter SHALL consume `Parts` once and call `Wait` after the channel closes. The public contract SHALL NOT require repeated or concurrent `Wait`, concurrent cancellation arbitration, protocol-specific stream state, or provider request/response metadata. Runtime internals SHALL still terminate their forwarding goroutine and timer after cancellation or completion.

#### Scenario: Provider error is ordinary data

- **WHEN** the provider emits `PartError`, then text, then `PartFinish`
- **THEN** the parts channel SHALL deliver all three in order and `Wait()` SHALL return nil after clean provider close

#### Scenario: Pre-stream call failure retains identity

- **WHEN** `DoStream` fails after successful resolution but before a valid stream exists
- **THEN** stream creation SHALL return the classified error and an outcome retaining identity but no usable stream invocation

#### Scenario: Adapter cancels after write failure

- **WHEN** a protocol adapter calls `Cancel(writeErr)` after a response write fails
- **THEN** the model context and internal forwarding SHALL terminate and the supplied cause SHALL remain inspectable

#### Scenario: Established provider stream ignores cancellation

- **WHEN** a provider leaves an established stream channel open after runtime context cancellation
- **THEN** runtime-owned forwarding SHALL terminate without waiting for that channel to close

#### Scenario: Provider blocks during stream setup

- **WHEN** a provider blocks synchronously inside `DoStream` and ignores its context
- **THEN** the deadline SHALL make the supplied context done, but the runtime call MAY remain blocked until the provider returns and SHALL NOT leak an additional setup goroutine

### Requirement: Runtime scope is provider LanguageModel execution

The runtime SHALL expose only normalized gateway control, catalog, middleware, failure, and provider LanguageModel-domain concepts. A façade MAY reuse it only for features representable without semantic loss by `provider.CallOptions`, `provider.GenerateResult`, and `provider.StreamPart`. A façade SHALL reject unsupported native fields before runtime invocation and MUST NOT silently drop them or hide them in opaque provider options. Native stateful/background Responses behavior, actual fallback-attempt identity, embeddings, and other non-LanguageModel operations remain outside this runtime contract.

The runtime SHALL NOT parse HTTP headers, choose content types, frame SSE, map HTTP status, project protocol error envelopes, generate terminal events, validate façade DTOs, or own Chat-specific choice/tool state. The actual Chat Completions implementation, not speculative mock DTOs, SHALL provide follow-up evidence for any additional shared runtime need.

#### Scenario: Runtime timeout reaches a committed adapter

- **WHEN** a runtime stream ends with timeout after a façade commits its response
- **THEN** the adapter SHALL decide how to represent the timeout and the runtime SHALL NOT fabricate a provider part or protocol terminal event

#### Scenario: Unsupported native feature is rejected by its façade

- **WHEN** a future protocol request includes a required feature with no provider LanguageModel representation
- **THEN** its adapter SHALL reject the request before runtime invocation rather than forcing it into gateway or provider options
