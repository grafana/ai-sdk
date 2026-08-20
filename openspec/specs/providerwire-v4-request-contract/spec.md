# providerwire-v4-request-contract Specification

## Purpose

Define the exact strict ProviderWire V4 language-model request envelope and payload contract emitted by the registered Gateway client.

## Requirements

### Requirement: Exact ProviderWire V4 request baseline
The strict ProviderWire V4 request contract SHALL target exactly the package versions registered in `test/conformance/upstream.yaml`. Its claim SHALL be limited to requests emitted by the registered `@ai-sdk/gateway` LanguageModelV4 client and SHALL NOT claim compatibility with Vercel's private server, another Gateway version, or the existing tolerant Go ProviderWire dialect.

#### Scenario: Registered client defines the target
- **WHEN** the request contract and evidence are verified
- **THEN** the package versions used SHALL equal the registered baseline
- **AND** the result SHALL identify the exact Gateway and provider package versions covered

#### Scenario: Legacy transport remains a separate dialect
- **WHEN** the Phase 1 request contract is added
- **THEN** `gateway/providerwire` SHALL retain its existing tolerant behavior and canonical bytes
- **AND** no strict runtime SHALL be mounted or implemented by this change

### Requirement: Language-model HTTP request envelope
A supported ProviderWire V4 language-model call SHALL use `POST` to `/language-model` relative to the configured Gateway `baseURL`, with a final Fetch-emitted `Content-Type` of `application/json`, a single non-empty `ai-language-model-id`, a single `ai-language-model-specification-version` value of `4`, and a single `ai-language-model-streaming` value of `true` or `false` according to the requested call mode. Request sequence order SHALL be significant. The service's planned `baseURL` ends in `/api/v1/aisdk`, producing the final endpoint `/api/v1/aisdk/language-model`; the protocol route SHALL NOT embed that service-owned prefix. A configured, call-level, or observability header collision that changes a reserved final value SHALL be classified as emitted pinned-client behavior but unsupported by the strict V4 envelope.

#### Scenario: Unary request envelope
- **WHEN** the pinned client performs a unary language-model call
- **THEN** it SHALL emit one `POST {baseURL}/language-model` request with `ai-language-model-streaming: false`
- **AND** the request SHALL carry the required model ID, specification version, and JSON content type

#### Scenario: Streaming request envelope
- **WHEN** the pinned client performs a streaming language-model call
- **THEN** it SHALL emit one `POST {baseURL}/language-model` request with `ai-language-model-streaming: true`
- **AND** the request SHALL carry the required model ID, specification version, and JSON content type

#### Scenario: Service prefix is composed through baseURL
- **WHEN** the pinned client is configured with `baseURL` ending in `/api/v1/aisdk`
- **THEN** its language-model request path SHALL be `/api/v1/aisdk/language-model`
- **AND** neither the client nor protocol contract SHALL prepend the service prefix a second time

#### Scenario: Multi-step request sequence
- **WHEN** a client tool flow performs multiple model calls
- **THEN** every request SHALL use the same envelope contract
- **AND** captures SHALL preserve the number and order of requests

### Requirement: Outer HTTP header composition and collisions
The request contract SHALL cover every non-exempt outer HTTP header emitted from equivalent call options. The pinned Gateway client SHALL carry call-level `options.headers` both in the JSON body and on the outer HTTP request. Gateway configuration headers SHALL first be normalized to lower-case names. The serializer SHALL then compose plain JavaScript header objects with case-sensitive exact-key spread in this order: configured Gateway headers, call-level headers, model protocol headers, and observability headers; `postJsonToApi` SHALL prepend the default JSON content type. A later value SHALL replace an earlier value only for the same exact property key at this intermediate stage. Before Fetch, `normalizeHeaders` SHALL iterate those entries in insertion order, lowercase each key, and let the later value replace an earlier case variant. Fetch SHALL receive the resulting single value per normalized header name. Authentication, user-agent, and observability values MAY be normalized as explicitly client-owned differences; other emitted header values, exact-key replacement, normalized last-value outcomes, and reserved-collision outcomes SHALL remain compatibility evidence.

#### Scenario: Call header is emitted in both locations
- **WHEN** a supported scenario supplies a non-reserved, non-colliding call-level header through `options.headers`
- **THEN** its original key and value SHALL appear in the request body `headers` member
- **AND** Fetch SHALL receive its value under the normalized outer header name

#### Scenario: Exact-key call header replaces configured header
- **WHEN** configured Gateway headers and call-level headers use the same exact property key
- **THEN** object spread SHALL retain only the call-level value before final normalization
- **AND** Fetch SHALL receive the call-level value

#### Scenario: Case-variant custom header uses the later value
- **WHEN** configured Gateway headers and call-level headers use case variants of the same normalized custom header name
- **THEN** final normalization SHALL retain the later call-level value under the lower-case name
- **AND** Fetch SHALL NOT receive a combined value

#### Scenario: Exact-key protocol header replaces caller value
- **WHEN** a call-level header uses the same exact lower-case property key as an `ai-language-model-*` protocol header
- **THEN** object spread SHALL retain the later model-derived protocol value
- **AND** the request MAY satisfy the strict V4 envelope when the final value is otherwise valid

#### Scenario: Case-variant protocol header uses the model value
- **WHEN** a call-level header uses a case variant of an `ai-language-model-*` protocol header
- **THEN** final normalization SHALL retain the later model-derived value under the lower-case name
- **AND** the request MAY satisfy the strict V4 envelope when the final value is otherwise valid

#### Scenario: Exact or case-variant content-type collision is unsupported
- **WHEN** a configured, call-level, or observability header supplies a non-JSON value under the exact or case-variant `Content-Type` name after the prepended default
- **THEN** final normalization SHALL retain the later non-JSON value
- **AND** evidence SHALL classify the final request as unsupported by the strict V4 envelope

#### Scenario: Classified outer headers are captured
- **WHEN** a deterministic request scenario emits content-type, custom call, protocol, authentication, user-agent, or observability headers
- **THEN** the capture SHALL retain every non-exempt behavior-affecting value and collision outcome
- **AND** any normalized client-owned value SHALL be identified explicitly

### Requirement: Normative strict request JSON Schema
The repository SHALL provide a reviewable JSON Schema draft 2020-12 contract for the complete supported ProviderWire V4 request payload and necessary shared definitions. The schema SHALL be authoritative for payload shape, SHALL close finite objects against unknown members, SHALL encode requiredness and nullability explicitly, and SHALL reject unknown discriminators and properties belonging only to inactive union arms.

#### Scenario: Captured request validates
- **WHEN** a semantic request emitted by a supported pinned-client scenario is validated
- **THEN** it SHALL satisfy the normative request schema

#### Scenario: Unknown finite member is rejected
- **WHEN** a finite request object contains an unclassified property
- **THEN** schema validation SHALL fail

#### Scenario: Unknown or mixed union arm is rejected
- **WHEN** a tagged request union uses an unknown discriminator or includes a property valid only for another arm
- **THEN** schema validation SHALL fail

#### Scenario: Opaque provider option JSON remains opaque
- **WHEN** a provider option namespace contains nested JSON values, including nulls, arrays, and objects
- **THEN** the request schema SHALL preserve those values without applying another provider's or host's schema recursively

### Requirement: Presence and nullability semantics
The request contract SHALL preserve every registered-client distinction among absence, permitted null, empty string, empty array, empty object, zero, and false. Optional standardized fields SHALL reject typed null unless the exact registered contract permits null. Required fields SHALL remain present even when their valid value is empty.

#### Scenario: Optional scalar presence is significant
- **WHEN** a scalar supports an explicit zero, false, or empty value distinct from omission
- **THEN** the schema and semantic evidence SHALL preserve both states distinctly

#### Scenario: Empty collection is significant
- **WHEN** the pinned client emits an explicitly empty supported array or object
- **THEN** the semantic request SHALL retain that property and empty collection

#### Scenario: Invalid standardized null is rejected
- **WHEN** a standardized field is assigned null but the registered type does not permit null
- **THEN** schema validation SHALL fail

#### Scenario: Nested provider-option null is preserved
- **WHEN** opaque provider options contain a nested null
- **THEN** semantic capture and schema validation SHALL retain it unchanged

### Requirement: Prompt role and content union contract
The request schema SHALL represent the exact registered system, user, assistant, and tool message shapes and every classified request content discriminator. It SHALL preserve message and content order, role-specific required members, provider options, tool call/result/approval correlation identifiers, and valid file-data values without concatenation, omission, or reinterpretation.

#### Scenario: System message behavior
- **WHEN** a supported system message is serialized by the pinned Gateway client
- **THEN** its exact registered content shape and provider options SHALL be captured and schema-valid

#### Scenario: User file-data arm
- **WHEN** a supported user file part uses each classified file-data discriminator
- **THEN** each arm SHALL preserve its discriminator, required payload, media type, optional filename, and provider options

#### Scenario: Assistant tool call and tool result content
- **WHEN** assistant content carries a classified tool call or tool result
- **THEN** the request SHALL preserve all required identifiers, input or output values, flags, and provider options in order

#### Scenario: Tool result and approval response content
- **WHEN** tool-role content carries a classified tool result or approval response
- **THEN** the request SHALL preserve the selected output arm, correlation identifiers, decision fields, flags, and provider options

#### Scenario: Role-incompatible content is rejected
- **WHEN** a content discriminator is placed under a role that does not permit it
- **THEN** schema validation SHALL fail

### Requirement: Tool and tool-choice union contract
The request schema SHALL represent every classified function-tool, provider-tool, and tool-choice discriminator from the registered contract. Each arm SHALL require its own members, preserve optional false and empty values where valid, and reject inactive-arm fields.

#### Scenario: Function tool is preserved
- **WHEN** a function tool includes its classified name, description, input schema, examples, strict value, and provider options
- **THEN** the selected values and their presence SHALL be retained in the semantic request

#### Scenario: Provider tool is preserved
- **WHEN** a provider-defined tool includes its classified type, identifier, name, and arguments
- **THEN** exactly those members and their values SHALL be retained in the semantic request
- **AND** a `providerOptions` member SHALL be rejected for the provider-tool arm

#### Scenario: Every tool-choice discriminator is valid
- **WHEN** each classified tool-choice arm is emitted by a supported scenario
- **THEN** the semantic request SHALL select the correct schema arm and preserve its required data

#### Scenario: Mixed tool arm is rejected
- **WHEN** a tool combines fields that are valid only in different discriminated arms
- **THEN** schema validation SHALL fail

### Requirement: Call settings and structured output contract
The request schema SHALL cover every classified LanguageModelV4 call setting and response-format arm, including all presence-sensitive scalar and collection values. JSON schema values carried for structured output SHALL remain semantic JSON, and array order SHALL remain significant.

#### Scenario: Classified scalar settings validate
- **WHEN** a scenario emits the classified token, sampling, penalty, seed, reasoning, raw-chunk, or related scalar settings
- **THEN** their values and presence SHALL validate against the request schema

#### Scenario: Structured output validates
- **WHEN** a classified structured response format includes its schema and optional descriptive members
- **THEN** those members SHALL be preserved as emitted and SHALL validate against the selected response-format arm

#### Scenario: Array order is preserved
- **WHEN** stop sequences, input examples, prompt parts, tools, or another ordered request array is captured
- **THEN** semantic comparison SHALL treat a reordered array as different

### Requirement: Body-carried headers and provider options
The request body contract SHALL represent the registered call-level `headers` and `providerOptions` members exactly as serialized by the pinned Gateway client. These body members SHALL remain request evidence only; Phase 1 SHALL NOT define host policy, backend forwarding, authentication, or reserved namespaces.

#### Scenario: Body-carried headers are captured
- **WHEN** a supported scenario supplies call-level headers
- **THEN** the body member SHALL preserve their names, values, and empty-map presence behavior as emitted
- **AND** non-empty call-level headers SHALL also be asserted on the outer HTTP request under the outer-header precedence contract

#### Scenario: Provider options are captured
- **WHEN** a supported scenario supplies multiple opaque provider option namespaces
- **THEN** each namespace and semantic JSON value SHALL be preserved without normalization into Go provider types

#### Scenario: Host policy remains undefined
- **WHEN** a body carries a Gateway-named, Grafana-named, or provider-named option namespace
- **THEN** the Phase 1 request contract SHALL validate only the registered payload shape
- **AND** it SHALL NOT specify whether a future host accepts, removes, rejects, or forwards that namespace

### Requirement: Request contract scope
This capability SHALL define only the ProviderWire V4 request envelope and payload. Response-arm validity, public error taxonomy, stream lifecycle, privacy policy, host request policy, model resolution, and provider invocation behavior SHALL be owned by separate capabilities.

#### Scenario: Response smoke evidence is non-normative
- **WHEN** pinned-client response-consumption probes pass
- **THEN** they SHALL NOT be treated as exhaustive response-contract or server-runtime evidence

#### Scenario: Runtime behavior is specified separately
- **WHEN** a production ProviderWire V4 runtime is introduced in a later change
- **THEN** its decoding, response, error, lifecycle, policy, resolution, and invocation behavior SHALL be defined outside this request-contract capability
