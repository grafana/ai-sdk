# Tasks

## 1. Read the model catalog from a setting

- [x] 1.1 Replace `LoadFile` with `ParseModelCatalog(document)` (64 KiB bound, same strict validation) and `Settings.ConfigFile` with `Settings.ModelCatalog` read from `GF_PLUGIN_MODEL_CATALOG`; remove file access from the plugin backend. Verify with `go test ./pkg/...` from `ai-gateway/` and `git grep -n 'os.Open\|os.Stat' ai-gateway/pkg`.
- [x] 1.2 Pass the catalog through the environment in the config, gateway and command integration tests, including missing, invalid and oversized catalogs. Verify with `go test ./pkg/...` and `mise run test-ai-gateway-command`.

## 2. Configuration and docs

- [x] 2.1 Switch `ai-gateway/provisioning/grafana.ini` to `model_catalog = $__file{...}` and update `ai-gateway/README.md` (install, settings, limits, local development). Verify with `mise run lint-docs` and the plugin end-to-end tests in CI.
