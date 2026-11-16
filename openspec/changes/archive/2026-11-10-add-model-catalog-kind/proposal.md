# Proposal

## Why

The Gateway serves one operator catalog, read at startup with keys from the plugin environment. A stack has no way to store its own providers, models, aliases, fallbacks and provider keys. The manifest declares no kinds, so there is nothing to store them in, and `config.File.Validate` requires `apiKeyEnv`, so it cannot validate a catalog whose keys live elsewhere. Later work builds stack-catalog models per request and serves catalog routes; both need a stored, validated, access-controlled catalog first.

## What Changes

- A `ModelCatalog` kind in `aigateway.ext.grafana.app/v1alpha1`: namespaced, not folder-scoped, with `secure: [{key: "*"}]` and validation admission on `CREATE` and `UPDATE`. `spec` mirrors the operator catalog document without `apiKeyEnv`; each provider's API key is a Grafana secure value whose key is the provider name.
- Generated Go and TypeScript types and the regenerated committed manifest.
- The app serves the kind as a managed kind with one validator, next to the existing route handler.
- Admission refuses an invalid catalog with a fixed `400` reason that never contains a secure value. On update it merges the stored secure keys with the update's `create`, `remove` and unchanged `name` entries before checking that keys and providers match one to one. It refuses `name` references to other secrets, provider types other than `anthropic` and `openai`, and the endpoint and compatibility fields `baseURL`, `providerName` and `supportsStructuredOutputs`. Those fields stay in the schema so schema pruning cannot drop them silently.
- Provider names follow the DNS label pattern, because they also become secure keys, discovery facts and log values.
- Two manifest roles, `aigateway:catalogreader` (viewer permission set, bound to Viewer) and `aigateway:catalogwriter` (editor permission set, bound to Admin), in one shared `roles` map and `roleBindings` block.
- Operator catalog validation is split into shape and key checks. Startup validation keeps its current behavior, messages and tests; the stack catalog reuses the shape check with its own policy and key checks. The spec-to-`config.File` conversion lives with the stack checks so per-request catalog work reuses both.
- The protocol test host serves manifest kinds from memory, with admission through the plugin protocol and Grafana's inline secure value semantics.
- End-to-end tests against Grafana prove the roles, folderless saves, secure storage, update and refusal behavior, secret-safe errors, and that the namespace `config` and `language-model` routes behave as before now that the manifest declares a kind.
- Documentation states the minimum self-hosted Grafana version and toggles for the kind, and that Grafana Cloud does not yet store secure values for plugin kinds.
- No catalog is served: stored catalogs have no runtime effect, and the operator catalog and BYOK are unchanged.

## Capabilities

### New Capabilities

- `gateway-model-catalog-kind`: the stored `ModelCatalog` resource: schema, provider keys as secure values, admission validation, catalog roles and platform requirements.

### Modified Capabilities

- `gateway-plugin-host`: the manifest now declares one kind besides the two routes; the app serves it with admission; the route table matches the manifest's custom routes; the test host serves manifest kinds; the namespace routes are unchanged by the kind.
- `gateway-caller-access`: the rule against Gateway-specific roles is scoped to the namespace routes, since catalog roles govern only `ModelCatalog` resources.

## Impact

- **Code:** `ai-gateway/kinds/` (new kind file, manifest roles); generated `ai-gateway/pkg/internal/modelcatalog/v1alpha1`, `ai-gateway/src/generated/`, `ai-gateway/src/app-sdk-manifest.json` and `ai-gateway/pkg/internal/manifestdata`; `ai-gateway/pkg/internal/config` (shape and key split); a new `ai-gateway/pkg/internal/stackcatalog` package (conversion, policy, key check, secure merge, validator); `ai-gateway/pkg/internal/gateway` (`ManagedKinds`); `ai-gateway/test/platformhost` and its command.
- **APIs:** a new Kubernetes-style resource in the Gateway's API group. No change to ProviderWire, the namespace routes or the public Go packages.
- **Tests:** unit tests for the three checks, the secure merge and the validator; manifest and app agreement; admission through the test host; Playwright tests under `ai-gateway/tests/`, run through `ai-gateway/docker-compose.yaml`.
- **Docs:** glossary entry; Gateway README requirements; architecture walkthrough if its manifest description drifts.
- **Not parity-sensitive:** the Vercel AI SDK has no tenant catalog, and no stream, chunk, provider or wire format changes.
