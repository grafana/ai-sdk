## MODIFIED Requirements

### Requirement: OpenAI Responses backend identity stays private
Discovery SHALL remain an authorized public route catalog with provider grafana and public IDs/names/descriptions/aliases, not an OpenAI configuration dump. Configured provider-instance keys, baseURL, API keys, apiKeyEnv names and fallback topology SHALL NOT be exposed through caller diagnostics or discovery. Access logs, metrics and metadata-only Agent Observability SHALL retain closed logical identity and SHALL NOT contain caller/provider response data or actionable error details.

Supported actual response identity, sources/warnings and ordinary metadata under #280 SHALL remain caller-visible at registered fields rather than be concealed as backend secrets. Configured backend model IDs SHALL NOT be synthesized as public configuration fields, but an actual supplied registered response model value SHALL survive even when it equals that ID. Typed unary identity SHALL remain overwritten by pinned clients and available only in bounded raw response body. Native transport/credentials SHALL remain excluded; public errors SHALL preserve only gateway-caller-response-policy's reviewed diagnostics, bound to the trusted physical adapter policy installed at construction before fallback/logical wrapping. Existing strict startup configuration, hardened transport, ambient-default rejection, SDK retry settings and execution guards SHALL remain unchanged.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authenticated client requests /api/v1/aisdk/config
- **THEN** public canonical/alias rows SHALL be listed with no private provider configuration or fallback topology

#### Scenario: Actual OpenAI identity is caller data
- **WHEN** a configured OpenAI model supplies registered response model/id values and a reviewed structured provider error
- **THEN** applicable success/error responses SHALL preserve those contracted values using that same candidate's trusted error policy
- **AND** configured endpoint/auth/instance/environment references SHALL remain protected independently from useful caller semantics

### Requirement: Compatible backend identity stays private
Discovery SHALL keep publishing provider grafana and only public route IDs/names/descriptions/aliases. Configured compatible provider-instance keys, providerName, baseURL, API keys, apiKeyEnv names and operator fallback topology SHALL NOT be copied into discovery or public diagnostics. Access logs, metrics and metadata-only Agent Observability SHALL retain closed logical identity and omit caller/provider response data and arbitrary provider prose. These restrictions protect configuration and telemetry independently of provider-account ownership; startup-configured credentials SHALL not imply implemented BYOK provisioning.

Supported actual response identity and ordinary metadata under #280 SHALL survive at registered fields even when provider-supplied values identify the model/provider; configured identities SHALL NOT be synthesized into an operator dump. Pinned typed unary overwrite/raw-body-only identity remains explicit. Reviewed actionable error message/status/code/type/param SHALL follow gateway-caller-response-policy's source-specific projection, selected by the trusted candidate policy before fallback/logical wrapping, never by body claims or configured logical providerName. Whole native error bodies, unknown siblings, transport/auth material and joined fallback prose SHALL remain excluded. Unknown/unreviewed compatible schemas SHALL use fixed safe fallback. Existing strict configuration, required endpoints/credentials, hardened shared transport, cancellation, usage request behavior, retry and effect guards SHALL remain effective.

#### Scenario: Discovery lists compatible models
- **WHEN** an authenticated client requests /api/v1/aisdk/config
- **THEN** public compatible canonical/alias rows SHALL be listed without private provider configuration

#### Scenario: Backend failure carries protected credential material
- **WHEN** a compatible backend returns 502 with protected transport/auth/request fields or unknown error-body siblings carrying credential markers beside a reviewed diagnostic message
- **THEN** those protected markers and whole native body SHALL not appear in public error/debug fields, access logs or metrics
- **AND** the reviewed bounded message/code/param SHALL survive for the caller only, without searching arbitrary application strings for secret-like patterns

#### Scenario: Provider auth prose echoes a credential
- **WHEN** a reviewed compatible auth failure can echo an API key in its native message/param
- **THEN** fixed actionable provider-account authorization prose SHALL replace those credential-bearing diagnostics while reviewed safe type/status/code follow the policy
- **AND** Gateway authentication guidance SHALL not replace a provider-account failure

#### Scenario: Ordinary caller value resembles a token
- **WHEN** supported metadata or a reviewed non-auth application diagnostic contains a harmless token-shaped value within its provenance and bounds
- **THEN** that value SHALL remain caller data, not be heuristically censored
- **AND** none of it SHALL enter logs, metric labels or metadata-only exports
