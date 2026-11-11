# Proposal

## Why

Tenant-scope extensions can be registered, but a stack has nowhere to store its configuration for them. Until namespace configuration exists, a tenant extension runs everywhere it applies when it is enabled by default, with an empty configuration. Before the Gateway can apply per-namespace configuration, org admins need a stored, validated and access-controlled resource for it.

## What Changes

- A `Middleware` kind in `aigateway.ext.grafana.app/v1alpha1`: namespaced, not folder-scoped, no secure values, validation admission on `CREATE` and `UPDATE` only. `spec` holds `type`, a required `enabled` and an optional open `config` object. `metadata.name` must equal `spec.type`, so a namespace holds one resource per extension.
- Generated Go and TypeScript types and the regenerated committed manifest.
- Two roles in the existing `roles` map: `aigateway:middlewarereader` (viewer permission set, bound to Viewer) and `aigateway:middlewarewriter` (editor permission set, bound to Admin). The catalog roles are unchanged.
- A Middleware validator next to the catalog validator, built from the distribution's extension definitions for running and idle Gateways. It accepts a write only when the type is a registered tenant extension and `config` matches its schema. Operator and unregistered types are refused with the same reason. An update that only disables a resource is accepted whatever the stored type, so a rollback never strands stored resources. `config` is bounded to 64 KiB.
- Refusals are `400` with field paths and schema locations only; no configuration value or schema-library message reaches a response, log or metric. The operator extension setting and admission share one location-only schema check.
- Stored resources have no runtime effect. Tenant extensions keep running as they do today.
- Accept paths are proven in Go through the protocol test host with test-only definitions passed through the existing options. The end-to-end suite against Grafana proves roles, refusals and secret-safe errors with the shipped build, which registers no tenant extension; accept paths against Grafana arrive with the first shipped tenant extensions.
- Documentation: the extension guide gains tenant configuration (resource, roles, admission rules, writes failing closed while the plugin is unavailable, which `config` shapes the platform preserves); glossary entry; architecture walkthrough's kind sentence.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: tenant configuration resources, their admission, disable-only updates, secret-safe refusals, admission scope, roles, size bound, no runtime effect yet, documentation and evidence.
- `gateway-plugin-host`: the manifest declares two kinds; the app serves the Middleware validator with the distribution's definitions; the namespace routes stay unchanged with both kinds.
- `gateway-caller-access`: "No Gateway-specific role" names every Gateway kind role, not only the catalog roles.

## Impact

- **Code:** `ai-gateway/kinds/` (new `middleware.cue`, roles); generated `ai-gateway/pkg/internal/middleware/v1alpha1`, `ai-gateway/src/generated/middleware/`, `ai-gateway/src/app-sdk-manifest.json`, `ai-gateway/pkg/internal/manifestdata`; new `ai-gateway/pkg/internal/tenantconfig` (validator); `ai-gateway/pkg/internal/extensionhost` (shared schema check); `ai-gateway/pkg/internal/gateway` (managed kinds from definitions); `ai-gateway/pkg/extension` (tenant scope godoc).
- **APIs:** a new Kubernetes-style resource in the Gateway's API group. No change to ProviderWire, the namespace routes, `gateway.Options` or the extension contract's types.
- **Tests:** validator unit tests; manifest and app agreement; admission through the test host with injected definitions, including a rollback across two Gateways sharing one store; Playwright specs under `ai-gateway/tests/` and the plugin-log sentinel check in `scripts/e2e.sh`.
- **Docs:** `docs/guides/gateway-extensions.md`, the Gateway glossary and architecture walkthrough.
- **Not parity-sensitive:** the Vercel AI SDK has no tenant configuration resource, and no stream, chunk, provider or wire format changes.
