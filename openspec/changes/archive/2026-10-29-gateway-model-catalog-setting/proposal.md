# Proposal

## Why

Grafana's shared plugin validator refuses to publish the Gateway because the plugin backend reads its model catalog from a file (`config_file`): "It is not permitted to access the file system." The dev plugin host also supplies plugin settings only as environment variables and secrets, so a file path cannot work there either (#612).

## What Changes

- **BREAKING:** replace the `config_file` setting (`GF_PLUGIN_CONFIG_FILE`) with `model_catalog` (`GF_PLUGIN_MODEL_CATALOG`), whose value is the catalog YAML itself. The plugin backend no longer opens any file.
- Keep the same strict, single-document YAML validation and secret-safe errors; bound the setting at 64 KiB instead of the former 1 MiB file limit, well below the 128 KiB a single Linux environment variable can hold.
- Self-hosted Grafana keeps a catalog file through Grafana's `$__file{}` expansion: `model_catalog = $__file{/etc/grafana/aigateway/models.yaml}`. Environment-only hosts set `GF_PLUGIN_MODEL_CATALOG` directly.
- A leftover `config_file` setting is ignored; startup then fails because `model_catalog` is missing.
- Update the local development configuration, tests and operator documentation. The end-to-end discovery test uses the largest catalog that fits the new bound; a configured catalog can no longer produce a discovery response above 1 MiB, and the discovery handler unit test keeps that no-cap evidence.

Catalog resources managed through the App Platform API are a separate, larger design and out of scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-plugin-host`: host-compatible configuration reads the model catalog from a setting instead of a file.

## Impact

- **Code:** `ai-gateway/pkg/internal/config` (`ParseModelCatalog` replaces `LoadFile`; `Settings.ModelCatalog` replaces `ConfigFile`) and `ai-gateway/pkg/internal/gateway`.
- **Tests:** config and gateway unit tests, and the command integration test, pass the catalog through the environment.
- **Configuration:** `ai-gateway/provisioning/grafana.ini`; self-hosted operators must rename `config_file = <path>` to `model_catalog = $__file{<path>}`.
- **Docs:** `ai-gateway/README.md` (install, settings, limits, local development).
