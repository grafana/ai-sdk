## RENAMED Requirements

- FROM: `### Requirement: OpenAI Responses backend identity stays private`
- TO: `### Requirement: OpenAI Responses configuration and operator identity boundaries`
- FROM: `### Requirement: Compatible backend identity stays private`
- TO: `### Requirement: Compatible configuration and operator identity boundaries`

## MODIFIED Requirements

### Requirement: OpenAI Responses configuration and operator identity boundaries

Discovery, public error responses, access logs and metadata-only operator metrics SHALL retain their existing exclusions for OpenAI instance keys, baseURL, API key, apiKeyEnv names and configured backend mappings/model IDs. Discovery SHALL continue publishing provider grafana and only its existing public model IDs/names/descriptions/aliases; this change SHALL NOT add configured visibility fields.

Those surface-specific restrictions SHALL NOT censor supported normal inference warnings/source ID/display or provider-reported response identity. Registered native id/modelId/timestamp SHALL survive in raw unary and stream output under their runtime contracts; typed unary response replacement SHALL remain explicitly documented. Native response modelId is distinct from configured discovery mappings and canonical operator identity. Configured secrets and another tenant's state SHALL remain protected. Attempt/failure evidence and configured discovery remain separately owned and SHALL NOT be introduced here.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authenticated caller requests config
- **THEN** discovery SHALL retain the canonical/alias public rows without private configuration or new backend mappings

#### Scenario: OpenAI returns native identity
- **WHEN** an OpenAI backend reports identity different from its requested/canonical route
- **THEN** registered normal response identity SHALL preserve it while discovery, public errors and operator metadata-only surfaces retain their existing exclusions

### Requirement: Compatible configuration and operator identity boundaries

Discovery, public error responses, access logs and metadata-only operator metrics SHALL retain their existing exclusions for compatible instance keys, providerName, baseURL, API key, apiKeyEnv names, configured backend model IDs/mappings and backend error bodies. Discovery SHALL continue publishing provider grafana and only its existing public model IDs/names/descriptions/aliases.

Supported normal inference warnings/source ID/display and provider-reported response identity SHALL remain caller-visible under their runtime contracts despite those surface-specific configuration/operator restrictions. Returned native modelId SHALL NOT be replaced with canonical route identity. Native bodies/headers, provider identity, attempt/failure evidence and configured discovery are not added by this change. Secrets and another tenant's state SHALL remain protected independently of ordinary scalar/display strings.

#### Scenario: Discovery lists compatible models
- **WHEN** an authenticated caller requests config
- **THEN** discovery SHALL retain canonical/alias rows without private configuration or new configured mappings

#### Scenario: Backend failure carries a secret
- **WHEN** a compatible backend returns a 502 with a secret marker in its error body
- **THEN** existing client error, access logs and metrics SHALL exclude the marker and private configuration

#### Scenario: Compatible backend returns native identity
- **WHEN** a successful compatible response supplies native response identity
- **THEN** raw unary/stream output SHALL retain its registered fields while canonical routing/operator identity and typed unary replacement stay unchanged
