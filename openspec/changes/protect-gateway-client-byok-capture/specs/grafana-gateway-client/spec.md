## MODIFIED Requirements

### Requirement: Exact ProviderWire routes and protected headers
The configured base URL SHALL be treated as the ProviderWire API prefix and the client SHALL append exactly `/config` for discovery and `/language-model` for calls without discarding an existing path prefix. Every model call SHALL use `POST`, JSON content type, the requested model ID, specification version `4`, exact streaming value `true` or `false`, and the matching JSON or SSE accept type. Configured headers SHALL be applied before call-level headers, followed by client-owned content negotiation, selected authentication, acting-user, and protocol headers so client-owned values each have one effective value and cannot be overridden. Accepted call-level headers SHALL also remain in the JSON request body.

For every authentication flow, `Authorization`, `X-Access-Token`, `X-Grafana-Id`, `X-Scope-OrgID`, `X-Cloud-Org-ID`, and `X-Access-Policy-ID` SHALL be reserved case-insensitively. Supplying any reserved name in configured headers SHALL fail construction. Supplying any reserved name in call-level headers SHALL fail before request serialization and network I/O, including when its value is empty. JWT constructors SHALL own X-Access-Token and optional acting-user header composition; unrelated configured/call headers SHALL NOT add a competing Authorization credential or trusted Cloud assertion.

#### Scenario: Unary request is emitted
- **WHEN** `DoGenerate` is invoked for model `assistant`
- **THEN** the request SHALL be `POST <base-prefix>/language-model` with streaming `false`, model ID `assistant`, specification `4`, JSON content and accept types, and exactly one value for each client-owned header

#### Scenario: Streaming request is emitted
- **WHEN** `DoStream` is invoked
- **THEN** the request SHALL use the same method, route, model, and specification headers with streaming `true` and SSE accept type

#### Scenario: Header names collide
- **WHEN** configured or call-level headers attempt to set client-owned headers and are not rejected by the authentication reserved-header rule
- **THEN** the client-owned effective values SHALL win and no duplicate client-owned protocol value SHALL be emitted

#### Scenario: Authentication reserved headers are configured
- **WHEN** configured headers contain a reserved credential or identity header under any casing
- **THEN** construction SHALL fail without network I/O or echoing its value in the error

#### Scenario: Authentication reserved headers are supplied per call
- **WHEN** call-level headers contain a reserved credential or identity header under any casing
- **THEN** the call SHALL fail before serializing that value into request metadata or issuing an HTTP request

#### Scenario: Body-carried headers are supplied
- **WHEN** `CallOptions.Headers` contains a representable header entry accepted by the selected authentication flow
- **THEN** it SHALL participate in outer-header composition and remain present in the serialized `headers` body member for the server to accept or reject

## ADDED Requirements

### Requirement: BYOK request projection without catalog coupling
The Go client SHALL emit representable provider/model IDs and providerOptions.gateway.byok using the same outer model header and JSON provider-options namespace as the registered Vercel client. It SHALL preserve multiple-provider maps and ordered credential arrays for unary and streaming calls without client-side catalog lookup, automatic discovery, endpoint changes or native account selection. Gateway host controls SHALL remain ordinary representable client options, with service-owned capability validation.

#### Scenario: Ordered credentials are serialized
- **WHEN** CallOptions contains multiple provider entries and ordered credentials
- **THEN** unary and streaming request captures SHALL match equivalent pinned-client semantic bodies and preserve credential order

#### Scenario: BYOK model is not configured
- **WHEN** a caller constructs a model for a valid provider/model string absent from configured discovery
- **THEN** the client SHALL issue inference directly without requiring a catalog entry

#### Scenario: Request metadata is caller-owned
- **WHEN** either client returns metadata for a BYOK request
- **THEN** its submitted credential subtree SHALL remain part of the local request representation
- **AND** documentation and tests SHALL distinguish that representation from server reflection and redacted consumer capture

### Requirement: Caller-owned request metadata and capture guidance
Both Go and exact-pinned Vercel requests SHALL emit the standard BYOK subtree in unary and streaming bodies. Their returned caller-owned request metadata SHALL retain the submitted arguments under the existing client contract; the server SHALL NOT claim it can sanitize those local objects. Guidance SHALL separate credential-aware automatic capture from direct application inspection.

Default Go logger capture SHALL structurally protect known BYOK fields, and a tested TS example SHALL show redaction of options/request metadata before consumer logging or telemetry. Enrichment and Agent Observability capture paths SHALL be inspected and tested with dummy markers without enabling unsupported capture APIs or removing ordinary application content.

#### Scenario: Actual consumer capture
- **WHEN** Go logging enables provider-options and request-body capture for both unary and streaming BYOK calls
- **THEN** its real capture sink SHALL contain no dummy BYOK markers while retaining allowed non-secret request content

#### Scenario: Stock Vercel request metadata
- **WHEN** the registered Vercel client returns request.body after BYOK submission
- **THEN** evidence SHALL show the local BYOK subtree is present
- **AND** the documented redaction example SHALL remove it from captured copies without changing the HTTP request or caller result

### Requirement: Transport-safe model selectors
Model construction SHALL accept nonempty valid UTF-8 selectors up to 2,048 bytes
without whitespace or control characters, independently of configured-catalog
grammar. The server SHALL remain responsible for selector capability validation.

#### Scenario: Native suffix exceeds catalog grammar
- **WHEN** a valid selector contains native punctuation or exceeds the former 128-byte catalog limit
- **THEN** the client SHALL preserve it without discovery or rewriting

#### Scenario: Invalid transport selector
- **WHEN** a selector is empty, invalid UTF-8, over 2,048 bytes or contains whitespace/control characters
- **THEN** model construction SHALL fail before I/O
