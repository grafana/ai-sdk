## MODIFIED Requirements

### Requirement: OpenAI Responses backend identity stays private
Authenticated discovery SHALL publish public model IDs, names, descriptions and alias rows with specification provider grafana, plus authorized configured provider-instance/provider/model facts through the gateway-configured-discovery projection. It SHALL NOT contain an OpenAI provider's API key, apiKeyEnv name, baseURL or unrelated configuration. Authorized provider-instance and configured model identifiers SHALL NOT be treated as credentials. Public runtime error responses, access logs and metrics SHALL retain their existing credential/backend-detail exclusions; this feature SHALL NOT expand those surfaces.

#### Scenario: Discovery lists OpenAI models
- **WHEN** an authorized authenticated client requests /api/v1/aisdk/config
- **THEN** the response SHALL list the OpenAI canonical and alias IDs with explicit ordered configured candidate facts
- **AND** it SHALL NOT contain provider credentials, secret references, baseURL or unrelated account/provider configuration

### Requirement: Compatible backend identity stays private
Authenticated discovery SHALL publish public model IDs, names, descriptions and alias rows with specification provider grafana, plus authorized configured provider-instance/effective-provider/model facts through the gateway-configured-discovery projection. The effective provider SHALL reflect the configured providerName or existing constructor default. It SHALL NOT contain API keys, apiKeyEnv names, baseURL, backend error bodies or unrelated configuration. Authorized provider-instance/providerName/model identifiers SHALL NOT be categorically concealed. Public runtime error responses, access logs and metrics SHALL retain their existing credential/backend-detail exclusions; discovery SHALL NOT enable additional operator capture or runtime diagnostic transport.

#### Scenario: Discovery lists compatible models
- **WHEN** an authorized authenticated client requests /api/v1/aisdk/config
- **THEN** the response SHALL list the compatible canonical and alias IDs with explicit ordered configured candidate facts
- **AND** it SHALL NOT contain credentials, secret references, baseURL, error bodies or unrelated account/provider configuration

#### Scenario: Backend failure carries a secret
- **WHEN** a compatible backend returns 502 with a secret marker in its error body
- **THEN** the client error, access logs and metrics SHALL retain their existing exclusions for that marker and private provider configuration
- **AND** discovery SHALL NOT reflect the runtime error or turn configured candidates into attempted/selected facts
