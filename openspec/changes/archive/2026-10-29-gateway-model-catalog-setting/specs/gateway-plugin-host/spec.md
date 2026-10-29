# Spec Delta

## MODIFIED Requirements

### Requirement: Host-compatible configuration
The Gateway SHALL read settings, including the bounded strict model catalog document, as `GF_PLUGIN_<KEY>` variables, which Grafana derives from its plugin configuration section, without command-line arguments or file access. Operator documentation SHALL state how each host supplies them. Configuration SHALL NOT accept authentication, listener or Cloud-ingress settings, and invalid configuration SHALL fail plugin startup before any request is served, without exposing secret values.

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
- **THEN** startup SHALL fail with a reason that names the setting without its value

#### Scenario: Startup failure is logged with its reason
- **WHEN** configuration or construction fails at startup
- **THEN** the Gateway SHALL log the failure with its reason before exiting
- **AND** the reason SHALL name the failing setting or provider without its value, the model catalog contents or credentials

#### Scenario: Environment-only startup
- **WHEN** the host starts the plugin with no arguments and a valid environment, including the model catalog
- **THEN** the Gateway SHALL construct its catalog, BYOK factories and telemetry, and serve requests
