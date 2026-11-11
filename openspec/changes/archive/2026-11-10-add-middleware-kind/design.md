# Design

## Context

- Registered baseline: not parity-sensitive (see proposal). Platform references: grafana-app-sdk `v0.60.16` (pinned in `ai-gateway/go.mod` and `.config/app-sdk/generate-kinds.mjs`) and Grafana `v13.3.0`, which uses app-sdk `v0.61.0`. CI's end-to-end image is a pinned nightly that may differ from `v13.3.0`.
- The manifest declares one kind, `ModelCatalog`, with `folderScoped: false`, validation on `CREATE` and `UPDATE`, and the shared `roles` and `roleBindings` blocks. `managedKinds()` in `pkg/internal/gateway` returns its kind with `stackcatalog.Validator{}`; `Gateway.Provider()` uses it for running and idle Gateways.
- `gateway.New` validates extension definitions first (`extensionhost.ValidateDefinitions`), then returns an idle Gateway when no signing keys are configured. Definitions carry `Name` (DNS label), `Scope`, `ConfigSchema` (`schema.Schema`, zero accepts only `{}`) and `EnabledByDefault`. The public build passes `gateway.DefaultExtensions()`, which is empty.
- `extensionhost.NewPlan` validates operator entries with a private `validateConfig` that reports the deepest instance location and never the schema library's message.
- The protocol test host already serves every manifest kind generically: in-memory store, admission reviews only for declared operations, inline secure values when present, no pruning or schema validation. Gateway tests build it from `manifestdata.LocalManifest()`, and the `plugin-host` command reads `app-sdk-manifest.json`, so a new kind is picked up without host changes.
- The end-to-end suite runs against the built plugin package (`mise run build-ai-gateway-plugin`, then `scripts/e2e.sh`). CI archives the same `dist/` as the candidate package, so the suite must exercise the shipped backend.

### What the platform does with an open `config`

Generating a scratch kind with the pinned generator shows that app-sdk CUE cannot emit `x-kubernetes-preserve-unknown-fields`. `config?: {[string]: _}` and `config?: {...}` both generate `{"type": "object", "additionalProperties": true}`; `config?: _` generates `{"type": "object", "additionalProperties": {}}`.

Grafana's kind store converts the manifest schema with app-sdk `VersionSchema.AsKubeOpenAPI`, builds an apiextensions structural schema from it and prunes with `structuralpruning.Prune` before admission. `AsKubeOpenAPI` maps both `additionalProperties: true` and `x-kubernetes-preserve-unknown-fields: true` to `additionalProperties: true` and drops the extension. Pruning then descends into each `config` value with a nil schema. Replaying that path offline with the pinned libraries on `config: {s: "v", list: ["a","b"], nested: {k: "v"}, objs: [{k: 1}]}` stores `{s: "v", list: ["a","b"], nested: {}, objs: [{}]}`:

- top-level fields and arrays of scalars are kept;
- objects nested inside `config` are emptied, also inside arrays;
- hand-editing the manifest to declare `x-kubernetes-preserve-unknown-fields` would not change this.

This reproduction is not evidence about the pinned nightly image. Admission sees the pruned object, so what is stored is what admission validated.

### Deviation: no `x-kubernetes-preserve-unknown-fields`

The issue asks for `spec.config` to use `x-kubernetes-preserve-unknown-fields` so the host does not prune it. The pinned platform cannot honor that, so the kind declares an open object (`additionalProperties: true`) and the `gateway-extensions` spec limits tenant configuration to top-level fields with scalar or scalar-array values. Local reproduction, not evidence from a running Grafana:

- Versions: grafana-app-sdk `v0.60.16` (generator and `AsKubeOpenAPI`; Grafana `v13.3.0` pins `v0.61.0`, whose `oapi3SchemaToKubeSchema` handles `additionalProperties` and the extension identically), `k8s.io/apiextensions-apiserver v0.37.1`, Grafana `v13.3.0` `pkg/services/apiserver/kindstore` (`structural.go`, `schema.go`, `openapi.go`).
- Step 1: copy `kinds/` and `.config/app-sdk/` to a scratch directory with the module's `go.mod`, add a kind whose `spec` has `config?: {[string]: _}`, `open?: {...}` and `anyv?: _`, and run `node .config/app-sdk/generate-kinds.mjs`. The manifest schema has `additionalProperties: true` for the first two and `additionalProperties: {}` for the third, and no `x-kubernetes-preserve-unknown-fields`.
- Step 2: in a scratch Go module, unmarshal two version schemas into `app.VersionSchema`, one with `config: {type: object, additionalProperties: true}` and one with `config: {type: object, x-kubernetes-preserve-unknown-fields: true}`. Convert each with `AsKubeOpenAPI`, drop `metadata` and expand refs as `kindstore.expandSchemaRefs` does. Then marshal to `apiextensionsv1.JSONSchemaProps`, convert to internal props, call `structuralschema.NewStructural` and `ValidateStructural` (no errors), and run `structuralpruning.Prune(obj, structural, true)`.
- Result for both schemas: `config` `{s: "v", list: ["a","b"], nested: {k: "v"}, objs: [{k: 1}]}` becomes `{s: "v", list: ["a","b"], nested: {}, objs: [{}]}`. The converted kube schema for both is `{type: object, additionalProperties: true}`.

## Goals / Non-Goals

**Goals:**

- One validator that later work reuses: #765 resolves the same definitions and re-applies the same schema check at read time.
- Accept paths proven without shipping a test-only extension.

**Non-Goals:**

- Reading or applying stored resources, caching, admission warnings (#765).
- Shipped tenant extensions and accept paths against Grafana (#766).
- Secure values in `Middleware`.
- Schema validation, pruning or RBAC in the test host.

## Decisions

### Kind schema

`kinds/middleware.cue`, referenced from `v1alpha1.kinds` after `modelCatalogKind`:

```cue
middlewareKind: {
	kind:         "Middleware"
	folderScoped: false
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		spec: {
			type:    string
			enabled: bool
			config?: {[string]: _}
		}
	}
}
```

- Generated plural `middlewares`, Go package `pkg/internal/middleware/v1alpha1` (imported as `middlewarev1alpha1`, because the root module has a `middleware` package), `Spec.Config map[string]interface{}`.
- No `secure` block, so the kind schema has no `secure` property.
- `type` stays a plain string, as `ModelCatalog`'s provider `type` does, so admission owns every type message.
- `enabled` is required: no schema default, so Grafana and the test host store the same object, and a disable-only update is unambiguous.
- `config` is optional and open. Absent means `{}`. It is the only shape the generator emits; the "Tenant configuration shape" requirement states its pruning limit (see the deviation above).

### Manifest and roles

```cue
roles: {
	"aigateway:catalogreader": {...} // unchanged
	"aigateway:catalogwriter": {...} // unchanged
	"aigateway:middlewarereader": {
		title:       "Middleware reader"
		description: "Read AI Gateway tenant extension configuration"
		kinds: [{kind: "Middleware", permissionSet: "viewer"}]
	}
	"aigateway:middlewarewriter": {
		title:       "Middleware writer"
		description: "Create, update and delete AI Gateway tenant extension configuration"
		kinds: [{kind: "Middleware", permissionSet: "editor"}]
	}
}
roleBindings: {
	viewer: ["aigateway:catalogreader", "aigateway:middlewarereader"]
	admin: ["aigateway:catalogwriter", "aigateway:middlewarewriter"]
}
```

- Names have no hyphens, so they match the SDK's `^aigateway:[a-z0-9]+$` and stay validated against `#Role`.
- Grafana registers `fixed:aigateway.ext.grafana.app:aigateway:middlewarereader|middlewarewriter`. Basic roles inherit, so Editors and Admins also read.
- The manifest test pins all four roles, their permission sets, the bindings and the rendered `fixed:` IDs, with the catalog roles as literals that must not change.

### Where the validator gets the definitions

`gateway.New` already receives the distribution's definitions in `Options.Extensions` and validates them before the idle check. Right after that check it builds the managed kinds once and keeps them on the `Gateway`:

- `managedKinds(definitions)` returns the catalog kind with `stackcatalog.Validator{}` and the Middleware kind with `tenantconfig.NewValidator(definitions)`.
- Both the idle and the running `Gateway` store the list, and `Provider()` passes it to `simple.NewApp`. An idle Gateway still refuses every review in the plugin runtime because it has no authenticator.
- `NewValidator` keeps a name-to-definition map of tenant-scope definitions only. Operator definitions are not kept, so an operator type behaves exactly like an unregistered one.
- The composition `Options` and the public `gateway.Options` are unchanged: test definitions reach the validator through the same field the shipped build uses.

New package `pkg/internal/tenantconfig` (tenant extension configuration stored as `Middleware` resources; #765 adds the reader there). Alternative: put the validator in `extensionhost`. Rejected: `extensionhost` would import generated kind types and app-sdk admission types, and the catalog precedent keeps kind validation in its own package.

### Shared schema check

`extensionhost` exports the check it applies to operator entries:

- `CheckConfig(definition extension.Definition, configuration json.RawMessage) error` returns "is not accepted: the extension takes no configuration" when the schema is zero and the configuration is not `{}`, or "does not match the extension's schema at <location>" with the deepest instance location, and nothing else from the schema library.
- `NewPlan` wraps it as `config: <setting>.<name>.config <reason>`, so every operator message and test stays unchanged.
- The Middleware validator wraps it as `middleware: spec.config <reason>`. #765 reuses it at read time.

### Admission algorithm

`tenantconfig.Validator` implements `simple.KindValidator`:

1. Any action other than `CREATE` or `UPDATE` is allowed. The manifest never sends one; this keeps delete independent of the validator if the manifest changes.
2. Type-assert `req.Object` (and `req.OldObject` on update) to `*middlewarev1alpha1.Middleware`; otherwise refuse with a fixed message.
3. `metadata.name` must equal `spec.type`; otherwise refuse with "middleware: metadata.name must equal spec.type".
4. Encode `spec.config` (nil as `{}`) with `encoding/json`, which sorts map keys, so equal configurations encode to equal bytes.
5. On update, if `spec.enabled` is `false` and the encoding equals the stored one's, allow. This disable-only path runs before the type, size and schema checks, so a rollback, a removed or re-scoped extension, a tightened schema or a lowered bound never prevents disabling.
6. Refuse an encoding over `MaxConfigBytes` (64 KiB): "middleware: spec.config exceeds 65536 bytes".
7. Look up the tenant definition. If it is missing, refuse with "middleware: spec.type %q is not a tenant extension of this Gateway", and on update append "; an update may only set spec.enabled to false and keep spec.config".
8. `extensionhost.CheckConfig(definition, encoding)`; refuse with its wrapped reason.
9. Every refusal is `apierrors.NewBadRequest(reason)`, so Grafana answers `400` and RBAC refusals (`403`) stay distinguishable.

The type in step 7 equals `metadata.name`, which the API server has already validated as a resource name, so quoting it is safe. Name mismatches are reported without either value.

### Value-free refusals

- Reasons are built only from fixed text, the type (step 7) and the schema's instance location. The schema library's `Error()` and the configuration are never formatted.
- Instance locations name the failing field and array index, never a value. For an undeclared key under `additionalProperties: false`, the location is the parent object, so the key is not named. A schema that permits arbitrary keys with a value schema can surface such a key in the location; keys are configuration structure, and the guide tells authors not to design keys that carry secrets.
- app-sdk logs the returned error; the reason is the only content it gets.
- A sentinel test refuses creates and updates carrying sentinels as values and as undeclared keys, and checks the response bodies and the plugin log handler output.

### Test host

No change to `test/platformhost`: `Middleware` is stored and reviewed like any declared kind, has no secure values, and undeclared deletes are not reviewed. Gateway tests get one harness option to serve a second Gateway against an existing host, so stored objects survive a "rollback" to a build with different definitions. The `plugin-host` command needs no change. Host behavior stays out of the platform evidence, as the existing requirement states.

### Testing the accept paths

The shipped build registers no tenant extension, so Grafana refuses every `Middleware` create until #766 ships `default-instructions` and `default-settings`.

Options:

- **A. Test-only definitions through `Options.Extensions`, Go only (chosen).** Gateway tests construct `gateway.New` with a test tenant definition (schema with a string with `maxLength`, an array of strings and `additionalProperties: false`), a schema-less tenant definition and an operator definition, and drive admission through the test host and the real app-sdk admission adapter. Nothing test-only is compiled into the backend.
- **B. A `gatewaytest`-tagged file in `pkg/` that adds a test tenant definition to `main`**, as `native_proxy_trust.go` does for a test CA, plus an `e2e.sh` phase that builds a tagged backend into a copy of `dist/` and restarts Grafana with it. It would prove the accept paths against Grafana now, but it adds a second Grafana boot to every run, a `main` hook for tags, and a second plugin package next to the archived candidate. The hook and phase become redundant once #766 ships real tenant extensions.
- **C. Accept specs that skip without a tenant type.** Rejected: skipped tests aren't evidence.

What CI end-to-end honestly covers before #766, with the shipped build:

- role registration and bindings, through `/api/access-control/user/permissions`;
- Viewer and Editor list and get (`200`), create, update and delete refused (`403`);
- an Admin create reaching admission and being refused (`400`) for an unregistered type, and for a name that differs from the type;
- an Admin delete of a missing resource returning `404`, not `403`;
- no sentinel in any refusal body or the plugin log;
- `catalog.spec.ts` and `routes.spec.ts` unchanged.

Moving to #766's end-to-end suite: folderless save, the flat `config` round-trip against Grafana's pruning, updates and disable, an unreviewed delete of a stored resource, and then the runtime effect that replaces "no runtime effect". Nested-object pruning stays unobservable there, because neither shipped schema accepts nested objects.

### Test plan

| Acceptance | Evidence |
|---|---|
| Invalid type, name or config refused, with no echo of config values | `tenantconfig` table tests: name mismatch, unregistered type, schema mismatch by value, type and undeclared key, non-empty config without schema, oversized config, wrong object types, each asserting the reason and the absence of sentinels. Gateway test through the test host with injected definitions: `400`, reasons and no sentinel in bodies or the plugin log buffer. **CI-e2e:** unregistered type and name mismatch refused with `400`, no sentinel in bodies or the plugin log (`e2e.sh`). |
| Operator types refused | `tenantconfig` test: an operator definition gets the unregistered-type reason. Gateway test with an injected operator definition. Not CI-e2e: the shipped build registers no operator extension. |
| Editors can't write; Viewers can read | Manifest test pins roles, permission sets, bindings and `fixed:` IDs. **CI-e2e:** permissions endpoint per role, `200` list and get for Viewer and Editor, `403` create, update and delete. |
| `DELETE` always works | Manifest test: validation operations are `CREATE` and `UPDATE` only. `tenantconfig` test: other actions are allowed. Gateway test: a resource stored by one Gateway is deleted, with no admission review, while a Gateway that no longer registers its type has stopped answering reviews (an idle Gateway cannot be served in process, and a stopped plugin is the stronger case). CI-e2e only for authorization (`404`, not `403`); a stored-resource delete moves to #766. |
| `config` isn't pruned | Manifest test: `spec.config` is `{type: object, additionalProperties: true}` and `spec` keeps `additionalProperties: false`. Gateway test: the test host returns the stored `config` unchanged (not platform evidence). Platform behavior is documented above; the Grafana round-trip of flat configs moves to #766. |
| `ModelCatalog` roles and behavior unchanged | Manifest test with literal catalog roles; existing `stackcatalog` and catalog admission tests unchanged. **CI-e2e:** `catalog.spec.ts` and `routes.spec.ts` unchanged. |
| A stored resource has no runtime effect | Gateway test: with a counting tenant extension that is off by default and one that is on by default, storing `enabled: true` and `enabled: false` respectively leaves the phases each runs on a language-model call unchanged. |
| Disable-only updates and rollback | Gateway test: one host, a Gateway with the test definition stores an enabled resource; a Gateway without it accepts the disable-only update and refuses enable and config changes. `tenantconfig` test: a tightened schema still allows disabling. |
| Shipped build refuses every create | Gateway test with no definitions; CI-e2e as above. |
| Idle Gateway | `TestProvider_ManagesTheManifestKinds` expects both kinds with validators, running and idle; the existing idle admission test adds a `Middleware` review. |

### Documentation

- `docs/guides/gateway-extensions.md`: a "Configure tenant extensions" section with the resource example, the roles, the admission rules (one resource per type, schema, disable-only updates, size bound), that creates and updates fail while the plugin is unavailable and deletes always work, that stored resources are not applied yet, and that only top-level fields and arrays of scalars survive the platform today. The existing paragraph on tenant extensions without namespace configuration links to it.
- `ai-gateway/pkg/extension`: `ScopeTenant` godoc mentions `Middleware` resources, not applied yet, and the `config` shape limit.
- Glossary: "Middleware kind; `Middleware`; tenant extension configuration", next to the model catalog kind entry.
- Architecture walkthrough: "The manifest also declares one kind" becomes two kinds, with one sentence on `Middleware`.
- README: no new requirement, since the catalog section already lists the version and toggles for plugin kinds.

## Risks / Trade-offs

- [Grafana empties objects nested in `config`] → A spec contract and the guide limit tenant configuration to flat shapes; both #766 schemas are flat. A schema that requires a nested field gets a refusal at admission; one with optional nested fields loses them silently. See Follow-ups for the registration check and the upstream issue.
- [The accept paths have no Grafana evidence until #766] → Go evidence runs through the real admission adapter; CI-e2e proves wiring, roles and refusals; #766 owns the rest, listed above.
- [Admission fails closed while the plugin is down or idle] → Creates and updates fail, deletes work; documented.
- [Decode failures before the validator] → app-sdk's codec reports JSON type errors, which name types and fields, not values.
- [Instance locations can name tenant-chosen keys under permissive schemas] → Keys, not values; documented for authors.

## Risks for #765

- Admission isn't a read-time guarantee: disable-only updates skip the schema, schemas tighten, builds roll back, and self-hosted hosts may lack validation. #765 must re-run `CheckConfig` on every enabled resource and apply the failure policy.
- Integer precision: the generated `map[string]interface{}` decodes numbers as `float64`. #765 should read `spec.config` as raw JSON, for example through an unstructured client, before passing it to factories.
- Admission warnings: the plugin protocol has `warnings`, but app-sdk `v0.60.16` validators return only an error, so #765's self-hosted warning needs an app-sdk change or another channel.
- One resource per type lets #765 list a namespace once. The plugin's own identity needs `list` on `middlewares`; verify that in #765's development deployment check.
- Disabled resources of unknown types stay stored. #765 decides whether its "unknown type" count includes disabled ones.
- The 64 KiB bound, the number of tenant extensions and the LRU size bound the cache's memory.
- Recorded #762 follow-ups still apply: idempotent collector registration, close on eviction, the configuration revision in the list key.

## Migration Plan

No migration: the kind starts empty. Rolling back to a build without the kind stops Grafana serving it and leaves stored resources unread. Rolling back to a build without a resource's extension leaves it stored; it can be disabled or deleted.

## Recorded decisions

- **Nested configuration:** documented as a contract, not enforced at registration. See the `gateway-extensions` "Tenant configuration shape" requirement and the follow-ups.
- **Disable-only updates:** accepted for every stored type, including a configuration a tightened schema no longer accepts.
- **Operator types:** refused with the unregistered-type reason.
- **`enabled`:** required.
- **Size bound:** 64 KiB on the encoded `config`.
- **End-to-end accept paths:** Go only, through the protocol test host with definitions passed through `Options.Extensions`; no test code in the backend.

## Follow-ups

- **Registration-time shape check.** Once a tenant extension needs nested configuration, either the platform preserves it or `ValidateDefinitions` refuses tenant schemas that allow nested objects (including through `$ref`, combinators and `additionalProperties` schemas).
- **Upstream app-sdk issue.** To be opened by a maintainer; draft below.
- **#765:**
  - re-run `extensionhost.CheckConfig` on every enabled resource at read time and apply the failure policy, because admission is not a read-time guarantee;
  - decode `spec.config` as raw JSON, not through the generated `map[string]interface{}`, so integers keep their precision;
  - the self-hosted admission warning needs an app-sdk change, because `v0.60.16` validators return only an error and the adapter does not expose the protocol's `warnings`;
  - the plugin identity needs `list` on `middlewares`; verify it in the development deployment check.
- **#766 end-to-end checks** that move from this change because the shipped build registers no tenant extension before #766:
  - an Admin creates a valid resource without a folder annotation (`201`), and one with `grafana.app/folder` is refused;
  - a flat `config` (string, number, boolean, string array) reads back unchanged after Grafana's pruning;
  - a valid update (`200`), and a schema-failing update refused with `400` and the location, with no sentinel in the body or plugin log;
  - a disable-only update (`200`);
  - a stored resource is deleted (`200`) and is gone on read;
  - Viewer and Editor `get` of a stored resource (`200`), and `403` on its update and delete;
  - the runtime effect that replaces "Stored tenant configuration has no runtime effect".

- **End-to-end plugin-log sentinel checks can pass vacuously.** `scripts/e2e.sh` greps the Grafana log for `logger=plugin.grafana-aigateway-app` lines and then for sentinels; if the filter matches no line, both sentinel checks pass without inspecting anything. The first change that touches the end-to-end suite after this one should add a positive check, for example that the filtered log contains `validation failed`, before the sentinel checks.

### Draft upstream issue

**Title:** `AsKubeOpenAPI` drops `x-kubernetes-preserve-unknown-fields`, so pruning empties nested objects in open fields

**Body:**

> A kind with an open object field, for example `config?: {[string]: _}` in CUE, generates a manifest schema of `{"type": "object", "additionalProperties": true}`. The CUE kind definitions offer no way to emit `x-kubernetes-preserve-unknown-fields`.
>
> When a host builds a structural schema from that manifest, `VersionSchema.AsKubeOpenAPI` (`oapi3SchemaToKubeSchema`) turns both `additionalProperties: true` and `x-kubernetes-preserve-unknown-fields: true` into `AdditionalProperties{Allows: true}` with no schema, and does not carry the extension over. apiextensions pruning then prunes each value of the field against a nil schema: top-level scalars and arrays of scalars survive, but nested objects are emptied, also inside arrays.
>
> Reproduction (grafana-app-sdk v0.60.16 and v0.61.0, k8s.io/apiextensions-apiserver v0.37.1): convert a version schema with `config: {type: object, x-kubernetes-preserve-unknown-fields: true}` with `AsKubeOpenAPI`, build a structural schema with `structuralschema.NewStructural`, and run `structuralpruning.Prune` on `{"spec": {"config": {"s": "v", "nested": {"k": "v"}, "objs": [{"k": 1}]}}}`. The result is `{"s": "v", "nested": {}, "objs": [{}]}`.
>
> Expected: an open object field keeps arbitrary nested content, as a CRD with `x-kubernetes-preserve-unknown-fields: true` does. Possible fixes: emit `x-kubernetes-preserve-unknown-fields: true` (as a vendor extension on the kube schema) for objects with `additionalProperties: true` and no properties, and let CUE kinds declare open fields that generate it.
