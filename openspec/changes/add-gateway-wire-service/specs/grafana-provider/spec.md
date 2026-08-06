## ADDED Requirements

### Requirement: Bounded remote response reads

The Grafana provider SHALL bound memory used to read remote provider-wire responses. It SHALL default to 16 MiB for a complete unary success body, 1 MiB for a complete non-2xx or invalid-content-type error body, and 8 MiB for one complete SSE event. It SHALL expose positive functional options for each limit, apply the configured limits to both cloud-auth and access-token providers, and reject zero or negative configured limits during construction.

This is a deliberate security correction: constructors and request bytes remain source and wire compatible, but a response accepted only because prior reads were unbounded MAY now fail unless the caller explicitly raises the applicable limit.

#### Scenario: Unary response at limit succeeds

- **WHEN** a valid generate response body is exactly the configured unary-success limit
- **THEN** the provider SHALL decode and return it normally

#### Scenario: Unary response exceeds limit

- **WHEN** a generate response body exceeds the configured unary-success limit by one byte
- **THEN** the provider SHALL return a non-retryable protocol `*provider.APICallError` without reading the unbounded remainder into memory

#### Scenario: Error body exceeds limit

- **WHEN** a non-2xx or invalid-content-type response body exceeds the configured error-body limit
- **THEN** the provider SHALL return a bounded synthesized `*provider.APICallError` and SHALL NOT retain the full response body

#### Scenario: Limits apply to both constructors

- **WHEN** providers are created through `NewWithCloudAuth` and `NewWithAccessToken` with the same limit options
- **THEN** models from both providers SHALL enforce the same response limits

#### Scenario: Invalid limit is rejected

- **WHEN** either constructor receives a zero or negative explicit response limit
- **THEN** it SHALL return an error and no provider

#### Scenario: Existing constructor calls remain source compatible

- **WHEN** an external consumer uses the existing `Option` type, calls either constructor with no new limit options, configures `WithHTTPClient`, or relies on the existing config-level HTTP client precedence
- **THEN** the code SHALL compile unchanged and preserve the pre-limit client-selection behavior

### Requirement: Bounded SSE event parsing

The Grafana provider SHALL parse SSE incrementally without unbounded `ReadString` or equivalent allocation. The configured SSE limit SHALL apply to the exact complete accumulated event bytes, including every field prefix, multiline `data:` fields, line endings, and terminating blank line. For a canonical strict-service event, both server and client SHALL count `data: ` plus canonical JSON plus `\n\n`. A valid event at the limit SHALL decode; limit-plus-one SHALL emit one final non-retryable protocol `PartError` and close the channel. Final bytes returned together with `io.EOF` SHALL still be processed before clean completion is decided.

#### Scenario: SSE event at limit succeeds

- **WHEN** one valid complete SSE event is exactly the configured event limit
- **THEN** the provider SHALL decode and forward its `provider.StreamPart`

#### Scenario: Unterminated event exceeds limit

- **WHEN** a server sends an unterminated SSE data line beyond the configured event limit
- **THEN** the provider SHALL stop buffering, emit one non-retryable protocol error part, cancel or close the response, and close the stream channel

#### Scenario: Multiline event uses aggregate limit

- **WHEN** individually small `data:` lines combine into an event larger than the configured limit
- **THEN** the aggregate event SHALL fail the same limit rather than allocating without bound

#### Scenario: Final-line EOF is preserved

- **WHEN** a valid final SSE data event has no trailing newline and the reader returns its bytes with `io.EOF`
- **THEN** the provider SHALL decode that event before subsequently treating EOF as clean completion

### Requirement: Response limits do not alter the provider-wire request contract

Adding response limits SHALL NOT change the Grafana provider's `/language-model` path, request headers, authentication headers, streaming selection, or default use of the legacy-tolerant `gateway/providerwire` codec.

#### Scenario: Existing canonical request remains unchanged

- **WHEN** a model call is made in default legacy mode with default or overridden response limits
- **THEN** its method, URL, headers, and request body SHALL equal the pre-limit canonical provider-wire request

### Requirement: Grafana exposes an explicit strict bidirectional codec mode

The Grafana provider SHALL define a typed provider-wire mode with legacy and strict values and a functional option selecting strict mode. Existing constructors without the option SHALL remain in legacy mode. In strict mode, request encoding plus unary and stream response decoding SHALL use only `gateway/providerwire/v4` conversion and MUST NOT call legacy `gateway/providerwire` codecs or provider custom JSON methods.

Strict mode SHALL accept canonical V4 results/events, reject legacy-only response shapes, apply the same configured read limits, and preserve the existing Grafana public model API. This mode is the migration seam toward strict V4 becoming canonical; flipping the default and removing legacy mode are follow-up changes.

#### Scenario: Existing constructors retain legacy mode

- **WHEN** callers use either existing constructor without the strict option
- **THEN** request and response codec behavior SHALL remain unchanged

#### Scenario: Strict mode is independent of legacy codecs

- **WHEN** a Grafana model in strict mode performs generate or stream against the strict service
- **THEN** all request/result/part conversion SHALL use the V4 package and unary/streaming calls SHALL complete through the existing Grafana public model API

#### Scenario: Strict mode rejects legacy response shape

- **WHEN** a strict-mode Grafana client receives a legacy-only generate result or stream event
- **THEN** it SHALL return a non-retryable protocol error rather than invoking legacy normalization

#### Scenario: Strict and server event limits agree

- **WHEN** the strict server emits an event exactly at or one byte above the configured framed-event limit
- **THEN** Grafana strict mode SHALL make the same at-limit decision using identical framing-byte accounting

#### Scenario: Strict unary service errors normalize safely

- **WHEN** the strict service returns a unary error in a supported safe category
- **THEN** the Grafana provider SHALL preserve status and retryability and SHALL automatically normalize recognized categories, including `forbidden` and `failed_dependency`, without exposing private provider data

#### Scenario: Strict stream categories remain recoverable

- **WHEN** the strict service emits a safe categorized stream error
- **THEN** the Grafana provider SHALL preserve it as `provider.StreamPart.APICallError` with status, retryability, and safe category data intact
- **AND** a consumer SHALL be able to recover the category by calling `NormalizeAPICallError` without changing the provider stream contract
