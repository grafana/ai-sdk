# Design

## Context

- Registered baseline: not parity-sensitive (see proposal). Platform references: grafana-app-sdk `v0.60.16` (pinned in `ai-gateway/go.mod` and `.config/app-sdk/generate-kinds.mjs`) and Grafana `v13.3.0`, which uses app-sdk `v0.61.0`. CI's end-to-end image is a pinned nightly that may differ from `v13.3.0`.
- The manifest declares one kind, `ModelCatalog`, with `folderScoped: false`, validation on `CREATE` and `UPDATE`, and the shared `roles` and `roleBindings` blocks. `managedKinds()` in `pkg/internal/gateway` returns its kind with `stackcatalog.Validator{}`; `Gateway.Provider()` uses it for running and idle Gateways.
- `gateway.New` validates extension definitions first (`extensionhost.ValidateDefinitions`), then returns an idle Gateway when no signing keys are configured. Definitions carry `Name` (DNS label), `Scope`, `ConfigSchema` (`schema.Schema`, zero accepts only `{}`) and `EnabledByDefault`. The public build passes `gateway.DefaultExtensions()`, which is empty.
- `extensionhost.NewPlan` validates operator entries with a private `validateConfig` that reports the deepest instance location and never the schema library's message.
- The protocol test host already serves every manifest kind generically: in-memory store, admission reviews only for declared operations, inline secure values when present, no pruning or schema validation. It decodes request bodies into `map[string]any`, so numbers pass through `float64`.
- app-sdk's plugin admission adapter decodes each reviewed object with the managed kind's codec into the kind's Go type before calling the validator, and returns a codec error as the refusal.
- The manifest is static: Grafana reads `app-sdk-manifest.json` from the plugin package at startup, and a malformed manifest drops every kind, `ModelCatalog` included.
- The end-to-end suite runs against the built plugin package (`mise run build-ai-gateway-plugin`, then `scripts/e2e.sh`). CI archives the same `dist/` as the candidate package, so the suite must exercise the shipped backend.

### What the platform does with an open object

A local reproduction, not evidence from a running Grafana. Grafana's kind store converts the manifest schema with app-sdk `VersionSchema.AsKubeOpenAPI`, builds an apiextensions structural schema, and prunes with `structuralpruning.Prune` on every write, before its own schema validation and before plugin admission.

- The app-sdk CUE generator cannot emit `x-kubernetes-preserve-unknown-fields`. `config?: {[string]: _}` generates `{type: object, additionalProperties: true}`.
- `AsKubeOpenAPI` maps both `additionalProperties: true` and `x-kubernetes-preserve-unknown-fields: true` to an `additionalProperties` that allows anything with no value schema, and drops the extension.
- Pruning then descends into each value with a nil schema. `{s: "v", list: ["a","b"], nested: {k: "v"}, objs: [{k: 1}]}` is stored as `{s: "v", list: ["a","b"], nested: {}, objs: [{}]}`.

Versions: grafana-app-sdk `v0.60.16` (generator and `AsKubeOpenAPI`), `k8s.io/apiextensions-apiserver v0.37.1`, Grafana `v13.3.0` `pkg/services/apiserver/kindstore`. So an open `config` cannot carry nested configuration, and no manifest edit fixes it.

### What typed members do

The same replay with typed optional members keeps nested configuration intact: every nested field is declared, so pruning keeps it, and Grafana validates JSON types, required fields and unknown fields before plugin admission. The generator puts `additionalProperties: false` on every struct; the kind store keeps unknown keys under it and then refuses them in validation rather than pruning them silently. Grafana cannot enforce a union: a member that doesn't match `type`, no member and two members are all stored. A CUE disjunction produces a schema that is not structural, which turns pruning off for the whole kind.

Generating candidate members with the pinned generator in a scratch copy (this design) confirmed:

- a generated `kinds/middleware_members.cue` struct unified into `spec` emits each member as a `$ref`-free nested object, with `required` for non-optional fields, `additionalProperties: false`, typed maps from `[string]: T`, and `description` from CUE doc comments;
- an empty struct (`{}`, `close({})`, a struct with only hidden fields) always emits `{type: object, additionalProperties: {}}`, an open object whose value schema has no type. That is not structural, so the generator cannot emit a member for a type without configuration.

## Goals / Non-Goals

**Goals:**

- Tenant configuration with nested objects that the platform stores and validates as written.
- One manifest for every build, public or private, with no manifest step in private pipelines.
- Private tenant implementations are possible without changing what the manifest describes.
- One validator and one spec decoder that #765 reuses at read time.
- Accept paths proven without shipping a test-only extension or type.

**Non-Goals:**

- Shipped tenant types and implementations (#766).
- Reading or applying stored resources, caching, admission warnings (#765).
- Tenant types whose schema must stay private (an opaque `custom` member, deferred).
- Secure values in `Middleware`.
- Schema validation, pruning or RBAC in the test host.

## Decisions

### Kind schema

```yaml
apiVersion: aigateway.ext.grafana.app/v1alpha1
kind: Middleware
metadata:
  name: model-access
spec:
  type: model-access
  enabled: true
  modelAccess:
    rules:
      - models: [balanced]
        teams: [platform]
```

`kinds/middleware.cue`, referenced from `v1alpha1.kinds` after `modelCatalogKind`:

```cue
middlewareKind: {
	kind:         "Middleware"
	folderScoped: false
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		spec: middlewareMembers & {
			type:    string
			enabled: bool
		}
	}
}
```

- `middlewareMembers` lives in the generated `kinds/middleware_members.cue`: one optional member per public tenant type with configuration, named `lowerCamel(type)`. It is `{}` while the registry is empty, so the shipped `spec` is `{type, enabled}`.
- `type` stays a plain string with no enum, members are optional and there is no `oneOf`. Any of these in the manifest would make Grafana refuse a stored object after a rollback to a build whose registry differs, and block the disable-only update and the delete paths that rely on Grafana accepting the write first.
- `enabled` is required and has no schema default, so Grafana and the test host store the same object.
- No `secure` block.
- Generated plural `middlewares`; Go package `pkg/internal/middleware/v1alpha1` (imported as `middlewarev1alpha1`).

Alternatives considered: an open `config` (nested data lost, see Context); `config` as a JSON string (no typed API, no platform validation); one kind per type (N list reads in #765, and a rolled-back kind makes its resources unreachable, so deletes stop working); a private build regenerating the manifest (a public manifest-building API, two manifest copies to keep in sync, and a bad manifest drops every kind).

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
- One role pair covers every tenant type.

### Public tenant type registry

New public package `ai-gateway/pkg/extension/tenanttype`, experimental v0 like `extension`. It imports only `extension` and `schema`, so private modules can bind to it.

```go
// Type is a public tenant extension type: the contract a Middleware
// resource configures. Implementations bind to it.
type Type struct {
	Name             string          // spec.type and metadata.name; a DNS label
	Description      string          // the member's description in the manifest
	ConfigSchema     schema.Schema   // zero: the type takes no configuration
	EnabledByDefault bool            // behavior when a namespace stores no resource
	Example          json.RawMessage // a valid configuration; nil without a schema
}

// Member returns the spec member that holds the type's configuration,
// lowerCamel(Name): "default-instructions" becomes "defaultInstructions".
func (t Type) Member() string

// All returns the public tenant types, in name order. It is empty in this
// version.
func All() []Type

// Implementation is how a build implements a type. Failure policy, key
// ownership, sources and priority belong to the implementation.
type Implementation struct {
	Keys          []extension.KeyOwnership
	Sources       []extension.Source
	Priority      int
	FailurePolicy extension.FailurePolicy
	New           func(config json.RawMessage, deps extension.Dependencies) (any, error)
}

// Bind returns the tenant-scope definition of impl for t, with t's name,
// schema and default.
func (t Type) Bind(impl Implementation) extension.Definition
```

- The registry is a package-level slice; #766 adds `default-instructions` and `default-settings` in their own files. `All` returns a copy.
- `extension.Definition` is unchanged. A tenant definition may still be written by hand, but startup checks it against the registry (next section).
- Tenant types have no secrets.

### Binding check at startup

The composition `Options` gain `TenantTypes []tenanttype.Type`. The public `gateway.Run` passes `tenanttype.All()`; tests pass their own types. The public `gateway.Options` are unchanged, so no distribution can add a type that the manifest doesn't describe.

`extensionhost.ValidateDefinitions(definitions, mandatory, types)` adds, before any setting is read and on idle hosts too:

- every type: name is a DNS label, not repeated; member is not `type` or `enabled` and not shared with another type; a schema compiles and has an `Example` that it accepts, and no schema means no `Example`;
- every tenant-scope definition: its name is a type's name, and its `ConfigSchema` bytes and `EnabledByDefault` equal the type's. Failures name the extension and the field: "tenant extension %q has no public type", "tenant extension %q differs from its public type in %s".

The structural and prune checks need apiextensions and run in the generator and in tests, not at startup (see Projection), so they stay out of the plugin binary.

Policy: private extensions are operator-scope by default. A private tenant implementation binds to a public type, which needs a public change for the type first. A tenant type whose schema can't be public is not supported; an opaque member can be added later as a new optional field.

### Projection and generator

`pkg/internal/tenantschema` projects a type's `ConfigSchema` into a value-free structural member schema and renders the members as CUE. It is imported by the generator and by tests only.

Projection rules, applied recursively from the member root:

- **Keep:** `type` (`string`, `integer`, `number`, `boolean`, `array`, `object`), `properties`, `items`, `additionalProperties: false`, a typed `additionalProperties` schema (a typed map), `required` within the member, `description`.
- **Drop:** `enum`, `const`, `pattern`, `format`, `minimum`/`maximum` and their exclusive forms, `multipleOf`, length and item and property count bounds, `uniqueItems`, `default`, `examples`, `title`. Admission checks them against the full schema. Dropping them keeps Grafana's own validation messages to field paths and JSON types, and dropping `default` keeps Grafana from writing values into stored objects.
- **Resolve:** local `$ref` into `$defs`/`definitions`; a recursive reference is refused.
- **Refuse:** a node without a single `type` (missing, an array of types, `null`); `oneOf`, `anyOf`, `allOf`, `not`, `if`, `patternProperties`, `dependentSchemas`; an object with neither properties nor a typed `additionalProperties`, including the member root; an object with properties whose `additionalProperties` is not explicitly `false`; `additionalProperties: true`; an array without `items`.
- **Never generated:** an enum on `spec.type`, a union across members, a required member.
- A type without a `ConfigSchema` has no member.

The member root must be an object. The generator refuses a type whose projection fails these rules, whose projection is not structural (`structuralschema.NewStructural` plus `ValidateStructural`), or whose `Example` does not survive a prune round-trip under the projection (`structuralpruning.Prune` of `{spec: {type, enabled, <member>: Example}}` returns it unchanged). Messages name the type and the schema location.

Rendering maps the projection to CUE: `string`, `int`, `number`, `bool`, `[...T]`, `{field: T, optional?: T}`, `{[string]: T}`, with descriptions as doc comments, and the member doc comment from `Type.Description`.

Flow and outputs:

```text
tenanttype.All() ──generator──▶ kinds/middleware_members.cue ──generate:kinds──▶ src/app-sdk-manifest.json
                                                                              pkg/internal/manifestdata
                                                                              pkg/internal/middleware/v1alpha1
                                                                              src/generated/middleware/
```

- Generator command: `pkg/internal/tenantschema/cmd/members`, writing `kinds/middleware_members.cue` with a generated-file header.
- `mise run generate-ai-gateway-kinds` runs the generator, then `pnpm --filter ./ai-gateway generate:kinds`. The scaffolded `generate-kinds.mjs` is not edited.
- Drift is checked without workflow changes: a `tenantschema` test renders `tenanttype.All()` and compares it with the committed `middleware_members.cue`, and CI's existing "Verify generated app manifest" step regenerates the manifest from CUE and fails on any difference. Together the committed manifest equals the projection of `All()`.

### Admission decoding

The Middleware managed kind uses a codec of `tenantconfig`, not the generated one, decoding into a `tenantconfig.Middleware` object that keeps the raw `spec` JSON and decodes only metadata. The validator then decodes the spec with `tenantconfig.DecodeSpec(raw) (Spec, error)`:

```go
type Spec struct {
	Type    string
	Enabled bool
	Members map[string]json.RawMessage // every key other than type and enabled
}
```

- Members stay raw JSON, so integers above 2^53 pass through exactly, and test-only types, which are not in the generated Go types, are seen.
- Spec decode errors never reach the caller: the validator refuses them with a fixed message. The codec reads exactly `apiVersion`, `kind`, `metadata` and `spec`, so a field that only case-folds to `spec` is ignored, and refuses a malformed object with a fixed `400`. The generated codec would decode typed members into Go fields and return messages such as `cannot unmarshal number 9223372036854775808 into ... int64`, which echo values.
- #765's reader uses `DecodeSpec` on the listed objects' raw `spec`.

### Where the validator gets its types and definitions

`gateway.New` builds the managed kinds once, after the binding check and before the idle check: `managedKinds(definitions)` returns the catalog kind with `stackcatalog.Validator{}` and `tenantconfig.Kind()` with `tenantconfig.NewValidator(definitions)`. Both idle and running Gateways keep them for `Provider()`. `NewValidator` keys the tenant-scope definitions by name; operator definitions are not kept. It needs no types: the member name derives from the type name, and the binding check guarantees each definition's schema and default equal its type's.

### Shared schema check

`extensionhost.CheckConfig(definition, configuration)` is exported from the operator plan's check: "is not accepted: the extension takes no configuration" for a schema-less extension given anything but `{}`, or "does not match the extension's schema at <location>", and nothing else from the schema library. `NewPlan` wraps it as before; the validator wraps it as `middleware: spec.<member> <reason>`; #765 reuses it at read time.

### Admission algorithm

`tenantconfig.Validator` implements `simple.KindValidator`:

1. Actions other than `CREATE` and `UPDATE` are allowed.
2. Assert the object types and decode the incoming spec; a failure is refused with a fixed message. A stored spec that can't be decoded counts as having no member, so it can still be disabled.
3. `metadata.name` must equal `spec.type`.
4. Let `m = lowerCamel(spec.type)`, derived from the name alone. The spec may hold at most one member, and only `m`: "middleware: spec may set only spec.<m>, the member of spec.type". The reason names only `m`.
5. **Disable-only update:** on update, `enabled: false`, and the member absent or equal to the stored one. Accept. Equality compares canonical encodings (sorted keys, numbers as written). This runs before every remaining check, so a rollback, a removed or rescoped implementation, a tightened schema or a lowered bound never blocks disabling, and "absent" covers a member the client removed because an older manifest no longer declares it.
6. The member's canonical encoding is at most 64 KiB, measured without HTML escaping.
7. A tenant definition for `spec.type` must exist: otherwise "middleware: spec.type %q is not a tenant extension of this Gateway", the same for unregistered, unimplemented and operator types; on update with "; an update may only set spec.enabled to false and keep or remove spec.<m>".
8. A type without configuration takes no member: "middleware: spec.type %q takes no configuration member".
9. With `enabled: true`, a type with configuration requires its member: "middleware: spec.<m> is required when spec.enabled is true". This catches a member pruned or omitted by a stale manifest, which would otherwise look like an empty configuration. With `enabled: false` and no member, accept: nothing is configured.
10. `CheckConfig(definition, member)`, refused as `middleware: spec.<m> <reason>`.
11. Every refusal is `apierrors.NewBadRequest`.

Reasons are built from fixed text, the type (equal to the already validated name), the derived member and schema locations, never from member values or the schema library's messages. A location keeps declared property names and array indexes and replaces every other segment, such as a typed map's key, with `*`, so it never names configuration data.

### Test host

- Request bodies are decoded with `UseNumber`, so numbers are stored and sent to admission as written. This is the only host change; it keeps integers above 2^53 exact for the protocol tests.
- Gateway tests get one harness option to serve a second Gateway against an existing host, so stored objects survive a "rollback" to a build with different types and definitions.
- Host behavior stays out of the platform evidence, as the existing requirement states.

### Generated-manifest evidence

A fixture under `pkg/internal/tenantschema/testdata` holds the Middleware schema the pinned app-sdk generator emits for a nested test type. A test converts it as Grafana does, checks that its `spec` equals the projection's structural schema exactly, and that the example survives pruning. Regenerate it with the generator and `generate:kinds` in a scratch copy when app-sdk is upgraded.

### Manifest test

- The Middleware `spec` requires exactly `type` and `enabled`, `type` is a string with no enum, there is no `oneOf`/`anyOf`, and the properties are exactly `type`, `enabled` and `Member(t)` for every type in `tenanttype.All()` with a schema. With the empty registry, that is `{type, enabled}`.
- For every such type, the committed manifest is converted as Grafana converts it (`AsKubeOpenAPI`, metadata removed, refs expanded), checked with `NewStructural` and `ValidateStructural`, and `{spec: {type, enabled, <member>: Example}}` must survive `Prune` unchanged. This is the real gate for #766's types: it checks the manifest the generator and app-sdk produced, not only the projection.
- The roles test pins all four roles, permission sets, bindings and `fixed:` IDs, with the catalog roles as literals.

### Testing the accept paths

The shipped registry and `DefaultExtensions()` are empty, so Grafana refuses every `Middleware` create until #766. Accept paths are proven in Go: tests pass test types through the composition `TenantTypes` and bound definitions through `Options.Extensions`, and drive admission through the test host and the real app-sdk adapter. Nothing test-only is compiled into the backend.

What CI end-to-end covers before #766: role bindings; Viewer and Editor list `200` and creates refused with nothing stored; Admin creates refused with `400` for an unregistered type and a name mismatch; an undeclared member with sentinel values refused by Grafana itself without echoing them; an Admin delete of a missing resource `404`; no sentinel in bodies or the plugin log; the catalog and route specs unchanged.

### Test plan

| Acceptance | Evidence |
|---|---|
| Invalid type, name or config refused, with no echo of config values | `tenantconfig` table tests: name mismatch, extra and mismatched members, unregistered, unimplemented and operator types, missing member when enabled, member on a type without configuration, schema mismatches by value, type and undeclared key, nested locations, oversized member, undecodable spec, each asserting the reason and the absence of sentinels. Protocol test with injected types and definitions: `400`, reasons, no sentinel in bodies, the app-sdk admission log or the Gateway log. Shipped plugin-process test: no sentinel in logs or stderr. **CI-e2e:** unregistered type and name mismatch refused with `400`, no sentinel in bodies or the plugin log. |
| Operator types refused | Unit and protocol tests with an operator definition. |
| Editors can't write; Viewers can read | Roles test. **CI-e2e:** permissions per role; Viewer and Editor list `200`, creates refused, nothing stored. |
| `DELETE` always works | Validation operations are `CREATE` and `UPDATE` only; other actions allowed; protocol test deletes a stored resource while the plugin no longer answers reviews. **CI-e2e:** authorization only (`404`, not `403`). |
| Nested `config` isn't pruned | Projection tests (rules, refusals, structural and prune round-trip of nested examples with typed maps and arrays of objects); manifest test per registry type; protocol test round-trips a nested member through the test host (not platform evidence). Platform round-trip moves to #766. |
| Members decoded exactly | `DecodeSpec` and validator tests with integers above 2^53; protocol disable-only test with such a member. |
| Tenant types are public | `ValidateDefinitions` tests: unknown tenant name, differing schema or default, invalid types, repeated and reserved members; `Bind` tests; startup through `gateway.New`. |
| Manifest equals the projection | Drift test over `tenanttype.All()`; CI's manifest regeneration step. |
| `ModelCatalog` roles and behavior unchanged | Roles test with literal catalog roles; catalog tests unchanged. **CI-e2e:** `catalog.spec.ts`, `routes.spec.ts`. |
| A stored resource has no runtime effect | Protocol test with counting tenant extensions, one off and one on by default. |
| Disable-only updates and rollback | Two Gateways, one store: disabling with the member kept or removed is accepted, enabling or changing it is refused; tightened schema and oversized stored member still disable. |
| Shipped build refuses every create | Protocol test with the shipped types and definitions; CI-e2e as above. |
| Idle Gateway | Provider test with both kinds, running and idle; idle process refuses `Middleware` reviews. |

### Documentation

- `docs/guides/gateway-extensions.md` "Configure tenant extensions": the resource and member, roles, admission rules, rollback (disable with the member kept or removed), writes failing while the plugin is unavailable, stored resources not applied yet, and how to add a tenant type: a public type in `tenanttype`, its projection rules, regenerating, then binding an implementation. Private extensions are operator-scope by default; private tenant implementations bind to public types. Type schemas evolve additively.
- `tenanttype` package documentation, and the `ScopeTenant` godoc pointing to it.
- Glossary: the Middleware kind entry describes typed members; a "tenant extension type" alias.
- Architecture walkthrough: the manifest's two kinds, and that the Middleware members come from the public type registry.

## Risks / Trade-offs

- [Tenant type names and schemas are public, including Cloud-only ones] → Policy (e); an opaque member is a later additive option.
- [Adding a tenant type needs a public change and release] → Accepted; private implementations still bind without public code.
- [Schema changes to a public type affect stored objects] → Grafana validates the whole object on every write without ratcheting, so a new required field or a narrower type would refuse updates of existing resources, including disable-only ones that keep the member. Type schemas evolve additively; a breaking change needs a new type name.
- [After a rollback to a manifest without a member, Grafana refuses writes that still carry it] → The disable-only rule accepts the member removed; documented.
- [The kind store keeps and refuses unknown keys under `additionalProperties: false` because it skips CRD schema checks] → If that changes, unknown keys would be pruned silently instead of refused; admission's full schema still validates what remains.
- [The generator path changes with app-sdk upgrades] → The manifest test converts the committed manifest as Grafana does and round-trips every example, so a generator change that breaks a member fails CI.
- [The schema package decodes instances as `float64`] → `CheckConfig` compiles the schema once per process for exact validation and decodes the instance with exact numbers, so bounds, enums and multiples above 2^53 are checked against the stored value; operator configuration gets the same.
- [Admission fails closed while the plugin is down or idle] → Creates and updates fail, deletes work; documented.
- [The accept paths have no Grafana evidence until #766] → Go evidence runs through the real adapter; CI-e2e proves wiring, roles and refusals.

## Risks for #765

- The reader decodes each listed object's raw `spec` with `tenantconfig.DecodeSpec` and passes the type's member, not `spec.config`, to factories; a missing member with `enabled: true` or another member makes the configuration invalid at read time.
- `ValidateDefinitions` takes the types, and #765 tests with tenant definitions need matching types; `NewValidator` keeps its signature.
- Its deltas on "Disable-only updates", "Tenant configuration admission" and the unknown-type rules must be reworded to members and to "a type this build doesn't implement".
- A registered type with no implementation is an unknown type at read time.
- Admission isn't a read-time guarantee, so #765 re-runs `CheckConfig` on enabled members.
- The plugin identity needs `list` on `middlewares`; the self-hosted admission warning needs an app-sdk change.

## Risks for #766

- Its two types go into `tenanttype` with descriptions and examples, implementations use `Bind`, and it runs `mise run generate-ai-gateway-kinds`.
- Their schemas must pass the projection: explicit `additionalProperties: false`, no empty objects, no type arrays or combinators.
- Member names are `defaultInstructions` and `defaultSettings`.

## Migration Plan

No migration: the kind starts empty. Rolling back to a build without the kind stops Grafana serving it and leaves stored resources unread. Rolling back to a build without a resource's type or implementation leaves it stored; it can be disabled, with its member kept or removed, or deleted.

## Recorded decisions

- **Typed union with a public type registry**, members at spec level named `lowerCamel(type)`, union rules in admission only.
- **Policy:** private extensions are operator-scope by default; private tenant implementations bind to public types.
- **Registry ships empty;** #766 adds the two types.
- **Failure policy and key ownership** stay with the implementation.
- **Disable-only updates:** accepted for every stored type with the member equal or absent.
- **Operator and unimplemented types:** the unregistered-type reason.
- **`enabled`:** required; a member is required with `enabled: true`.
- **Size bound:** 64 KiB on the member.
- **End-to-end accept paths:** Go only; no test code in the backend.
- **Types without configuration have no member (deliberate deviation from the brief's "`{}` for a schema-less type").** The generator cannot emit a closed empty object, and an open one is not structural, which turns pruning off for the whole kind. Such a type takes no member, `enabled: true` needs none, and any member is refused.
- **Disabled without a member:** accepted without `CheckConfig`.
- **Structural and prune checks:** in the generator and tests only; startup keeps the cheap type and binding checks. The drift check guarantees the committed manifest equals the projection.
- **Objects without an explicit `additionalProperties: false`:** refused, so the full schema and the projection agree.
- **`TenantTypes`:** explicit in the composition options; nil means no types.
- **Additive evolution:** Grafana validates every write against the current schema with no ratcheting, so a public type's schema changes only additively and an incompatible change takes a new type name. Stated in the spec, the guide and the `tenanttype` package doc.

## Follow-ups

- **#765:**
  - decode members with `DecodeSpec` and re-run `CheckConfig` on enabled members at read time;
  - the self-hosted admission warning needs an app-sdk change, because `v0.60.16` validators return only an error;
  - the plugin identity needs `list` on `middlewares`.
- **#766 end-to-end checks**, which need a shipped tenant type:
  - an Admin creates a valid resource without a folder annotation (`201`), and one with `grafana.app/folder` is refused;
  - a member with nested fields reads back unchanged after Grafana's pruning and validation;
  - Grafana's own validation messages for wrong JSON types, missing required fields and unknown keys carry no sentinel values;
  - a valid update (`200`), and a schema-failing update refused with `400` and the location, with no sentinel in the body or plugin log;
  - a disable-only update (`200`);
  - a stored resource is deleted (`200`) and is gone on read;
  - Viewer and Editor `get` of a stored resource (`200`), and `403` on its update and delete;
  - the runtime effect that replaces "Stored tenant configuration has no runtime effect".
- **Opaque member:** add only when a tenant type's schema truly can't be public.
- **End-to-end plugin-log sentinel checks can pass vacuously.** `scripts/e2e.sh` greps the Grafana log for `logger=plugin.grafana-aigateway-app` lines and then for sentinels; if the filter matches no line, both sentinel checks pass without inspecting anything. The first change that touches the end-to-end suite after this one should add a positive check before the sentinel checks.
