# Tasks

## 1. Start idle without signing keys

- [x] 1.1 Resolve the platform signing keys before any other setting in `gateway.New`; without them, return an idle Gateway with no handler, authenticator or sinks, unless insecure authentication skipping is requested, which still fails startup. Log a `plugin_idle` lifecycle event instead of `plugin_started`. Verify with `go test ./pkg/...` from `ai-gateway/`.
- [x] 1.2 Cover idle startup with valid, missing and invalid settings without reading secrets, and launch the built plugin without signing keys through the test platform host: health is OK, route requests are rejected by the plugin host and startup does not fail. Verify with `go test ./pkg/internal/gateway/ ./test/...` from `ai-gateway/`.

## 2. Docs

- [x] 2.1 Describe idle startup in `ai-gateway/README.md`. Verify with `mise run lint-docs`.
