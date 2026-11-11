# Proposal

## Why

Tenant-scope extensions can be registered, but a stack has nowhere to store its configuration for them. Until namespace configuration exists, a tenant extension runs everywhere it applies when it is enabled by default, with an empty configuration. Before the Gateway can apply per-namespace configuration, org admins need a stored, validated and access-controlled resource for it. The platform prunes objects nested inside an open field, so the resource needs a typed schema per tenant extension, and private builds need a way to implement tenant extensions without changing that schema.

## What Changes

- A `Middleware` kind in `aigateway.ext.grafana.app/v1alpha1`: namespaced, not folder-scoped, no secure values, validation admission on `CREATE` and `UPDATE` only. `spec` holds `type`, a required `enabled` and at most one typed member, named `lowerCamel(type)`, that holds the extension's configuration. `metadata.name` must equal `spec.type`, so a namespace holds one resource per extension.
- A public, experimental tenant type registry, `ai-gateway/pkg/extension/tenanttype`: each type has a name, description, configuration schema, default and example, and `Bind` turns an implementation into an extension definition. It ships empty.
- Startup refuses a tenant definition that has no public type or whose schema or default differs from it. Test-only types reach the Gateway only through internal composition options.
- A generator projects every type's schema into a structural, value-free member and writes the CUE the kind imports; `mise run generate-ai-gateway-kinds` runs it and the app-sdk generator. A test fails when the committed members drift from the registry. The generator refuses schemas whose projection isn't structural or doesn't keep the type's example through a prune round-trip.
- Generated Go and TypeScript types and the regenerated committed manifest.
- Two roles in the existing `roles` map: `aigateway:middlewarereader` (viewer permission set, bound to Viewer) and `aigateway:middlewarewriter` (editor permission set, bound to Admin). The catalog roles are unchanged.
- A Middleware validator next to the catalog validator, built from the distribution's extension definitions for running and idle Gateways. It accepts a write only when the spec sets no member but the type's, the type is implemented as a tenant extension, the member is present when enabled and matches the type's full schema. Operator, unregistered and unimplemented types are refused with the same reason. An update that only disables a resource, with its member kept or removed, is accepted whatever the stored type, so a rollback never strands stored resources. The member is bounded to 64 KiB and decoded from raw JSON, so large integers stay exact.
- Refusals are `400` with field paths and schema locations only; no configuration value or schema-library message reaches a response, log or metric. The operator extension setting and admission share one location-only schema check.
- Stored resources have no runtime effect. Tenant extensions keep running as they do today.
- Accept paths are proven in Go through the protocol test host with test-only types and definitions passed through internal options. The end-to-end suite against Grafana proves roles, refusals and secret-safe errors with the shipped build, which registers no tenant extension; accept paths against Grafana arrive with the first shipped tenant extensions.
- Documentation: the extension guide gains tenant configuration (resource and member, roles, admission rules, rollbacks, writes failing closed while the plugin is unavailable, adding a public tenant type, and the policy that private extensions are operator-scope by default while private tenant implementations bind to public types); the `tenanttype` package doc; glossary entry; architecture walkthrough's kind sentence.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: tenant configuration resources and members, public tenant types and their binding, the member projection, admission, disable-only updates, secret-safe refusals, admission scope, roles, size bound, no runtime effect yet, documentation and evidence.
- `gateway-plugin-host`: the manifest declares two kinds; the app serves the Middleware validator with the public tenant types and the distribution's definitions; the namespace routes stay unchanged with both kinds.
- `gateway-caller-access`: "No Gateway-specific role" names every Gateway kind role, not only the catalog roles.

## Impact

- **Code:** `ai-gateway/kinds/` (new `middleware.cue`, generated `middleware_members.cue`, roles); new `ai-gateway/pkg/extension/tenanttype`; new `ai-gateway/pkg/internal/tenantschema` (projection and generator); generated `ai-gateway/pkg/internal/middleware/v1alpha1`, `ai-gateway/src/generated/middleware/`, `ai-gateway/src/app-sdk-manifest.json`, `ai-gateway/pkg/internal/manifestdata`; new `ai-gateway/pkg/internal/tenantconfig` (spec decoding and validator); `ai-gateway/pkg/internal/extensionhost` (shared schema check, binding check); `ai-gateway/pkg/internal/gateway` (tenant types option, managed kinds); `ai-gateway/pkg/extension` (tenant scope godoc); `ai-gateway/test/platformhost` (numbers kept as written); `mise.toml` (generator task).
- **APIs:** a new Kubernetes-style resource in the Gateway's API group, and a new public v0 package, `tenanttype`. No change to ProviderWire, the namespace routes, `gateway.Options` or `extension.Definition`.
- **Tests:** projection and generator tests, including structural and prune round-trips; the drift test; binding tests; spec decoding and validator unit tests; manifest and app agreement; admission through the test host with injected definitions, including a rollback across two Gateways sharing one store; Playwright specs under `ai-gateway/tests/` and the plugin-log sentinel check in `scripts/e2e.sh`.
- **Docs:** `docs/guides/gateway-extensions.md`, the Gateway glossary and architecture walkthrough.
- **Not parity-sensitive:** the Vercel AI SDK has no tenant configuration resource, and no stream, chunk, provider or wire format changes.
