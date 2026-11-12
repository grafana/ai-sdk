## MODIFIED Requirements

### Requirement: Host-compatible configuration
The Gateway SHALL read settings, including the optional bounded strict model catalog document, as `GF_PLUGIN_<KEY>` variables that Grafana derives from its plugin configuration section, without arguments or file access. Documentation SHALL state how each host supplies them. Authentication, listener and Cloud-ingress settings SHALL NOT be accepted. Invalid configuration SHALL fail startup of a non-idle Gateway before any request, without exposing secrets.

#### Scenario: Legacy authentication blocks are present
- **WHEN** the model catalog contains an `auth` or `server` block
- **THEN** startup SHALL fail with a secret-safe unknown-field error

#### Scenario: Settings from the Grafana configuration section
- **WHEN** Grafana's configuration has a `[plugin.grafana-aigateway-app]` section with `access_mode` and `model_catalog`
- **THEN** the Gateway SHALL start with those settings, read from `GF_PLUGIN_ACCESS_MODE` and `GF_PLUGIN_MODEL_CATALOG`

#### Scenario: Catalog file through Grafana expansion
- **WHEN** a self-hosted operator sets `model_catalog = $__file{<path>}`
- **THEN** Grafana SHALL supply the file's contents as `GF_PLUGIN_MODEL_CATALOG`, and the Gateway SHALL NOT open the file itself

#### Scenario: Missing or oversized model catalog
- **WHEN** `GF_PLUGIN_MODEL_CATALOG` is unset, blank or larger than 64 KiB
- **THEN** an unset or blank catalog SHALL start the Gateway without an operator catalog and without resolving any operator provider key
- **AND** an oversized catalog SHALL fail startup with a reason that names the setting without its value

#### Scenario: Startup failure is logged with its reason
- **WHEN** configuration or construction fails at startup
- **THEN** the Gateway SHALL log the failure with its reason before exiting
- **AND** the reason SHALL name the failing setting or provider without the setting's whole value or credentials; it MAY name catalog keys and identifiers such as model IDs, provider names and `apiKeyEnv` variable names

#### Scenario: Environment-only startup
- **WHEN** the host starts the plugin with no arguments and a valid environment, with or without the model catalog
- **THEN** the Gateway SHALL construct its catalog, BYOK factories and telemetry, and serve requests

#### Scenario: Host without platform signing keys
- **WHEN** a host such as a Grafana Cloud stack's own Grafana starts the plugin without a platform signing-key source or any Gateway settings
- **THEN** the plugin SHALL keep running idle instead of exiting
