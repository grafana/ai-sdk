# Extend Grafana AI Gateway

A Gateway **distribution** is a plugin backend whose `main` calls
`gateway.Run`. A distribution can add **extensions**: Go code that runs around
every model call to decide eligibility, enforce quota, add default settings,
check input and output, or record usage and audit events.

Extensions use typed phases. The Gateway owns control flow, so an extension
never receives the model, never wraps it, and cannot change the caller's
identity, the selected model or its credentials. The package API is v0 and
experimental: it can change in any minor release.

## Define an extension

An extension is a `Definition` and an instance that implements one or more
phase interfaces. This one denies calls outside business hours and counts
usage:

```go
type hours struct{ clock func() time.Time }

func (h hours) Admit(ctx context.Context, call *extension.Call) error {
	if hour := h.clock().UTC().Hour(); hour < 6 || hour >= 22 {
		return extension.ErrDenied
	}
	return nil
}

func (hours) Observe(ctx context.Context, call *extension.Call, record extension.Record) error {
	return publishUsage(ctx, call.Namespace, record.Outcome, record.Usage)
}

var hoursDefinition = extension.Definition{
	Name:          "business-hours",
	Scope:         extension.ScopeOperator,
	Keys:          []extension.KeyOwnership{extension.KeysOperator},
	FailurePolicy: extension.FailClosed,
	New: func(config json.RawMessage, deps extension.Dependencies) (any, error) {
		return hours{clock: deps.Clock}, nil
	},
}

func main() {
	options := gateway.Options{Extensions: append(gateway.DefaultExtensions(), hoursDefinition)}
	if gateway.Run(options) != nil {
		os.Exit(1)
	}
}
```

Pass definitions explicitly. Importing a package registers nothing, and two
definitions with the same name fail startup. See the
[extension package reference](https://pkg.go.dev/github.com/grafana/ai-sdk/ai-gateway/pkg/extension)
for every field and interface.

## Understand the phases

For each call, the Gateway runs the phases in this order:

| Phase | Sees | Can |
|---|---|---|
| Admit | The [request information](#know-who-called-and-what-the-call-may-try). No prompt. | Deny |
| Transform request | The request information and the call parameters | Change the parameters |
| Inspect request | The parameters the model will receive | Deny |
| *Model call* | One call to the selected model, with its fallback chain | — |
| Inspect response | Each normalized output part: text, reasoning, a complete tool call, finish | Abort |
| Observe | One record per call: outcome, the denying extension and phase, usage, physical attempts and timing | Nothing |

- Admit and both inspectors run in name order and stop at the first
  `ErrDenied`. The caller gets the Gateway's fixed permission error; your error
  text never reaches it.
- Transforms run tenant extensions first, then operator extensions, each in
  name order. The Gateway refuses a transform that adds
  a credential-bearing header or the `gateway` provider-option namespace.
- Response parts are the same for unary and streaming calls. A streamed part
  carries its delta; a unary part carries the complete text. A unary abort
  fails the call; a streaming abort ends the stream with one error event.
- Observe runs after the response is returned or the stream closes, off the
  response path, under a context without the call's cancellation and with a
  one-second bound. It runs for denied, failed and cancelled calls too.
- Each extension gets its own `Call` per call. Use `Call.State` to share data
  between its phases, such as a reservation made in admit and committed in
  observe.

Extension outcomes sit above fallback, so a denial is never retried on another
candidate. A call refused before the model is called makes no provider request
and carries no `gateway.execution` overview.

## Know who called and what the call may try

Every phase receives the same `Call` for the call. It tells an extension who
called: the route namespace, the caller type (`user`, `service-account` or
`internal-service`) and the caller's identity as `user:<uid>` or
`service-account:<uid>`, which is also set when an internal service calls on
behalf of a user or service account. It tells what was asked for: the model ID
as sent, which can be an alias or a BYOK `provider/model` selector, and its
canonical ID, which BYOK calls don't have. And it lists the candidates the call
may try, in order, each with the model or `models` entry it belongs to, its
provider, provider instance and provider model. See the
[`Call` reference](https://pkg.go.dev/github.com/grafana/ai-sdk/ai-gateway/pkg/extension#Call)
for every field.

The `Record` that observers get adds `Attempts`: the attempts the call
actually made, in the order of `Candidates`, each with its provider, provider
instance, provider model, outcome (`selected`, `failed` or `canceled`) and
HTTP status when known. A call with one candidate reports one attempt; a call
refused before the model was called reports none; a stream reports its
attempts when it ends. Attempts come from the Gateway's own record of the call,
the same record behind the `gateway.execution` overview, and never carry error
text.

Every extension that applies, operator or tenant, sees the same information,
including the caller's identity and the operator's provider instances. It
never includes credentials or other user details, such as names or email
addresses. The Gateway also writes the caller's identity to its logs and Agent
Observability exports; it never becomes a metric label or reaches a provider.

## Choose where an extension applies

Declare where an extension applies with `Keys`, the key ownership of the
calls it handles. `operator` calls use the operator's catalog and keys;
`customer` calls use the customer's keys, from request BYOK or a stack catalog.
`Keys` defaults to both.

An extension that doesn't apply to a call runs none of its phases and gets no
record. `Call.Source` names where the call's models and keys came from, such as
`operator-catalog`, `request-byok` or `stack-catalog`. It is informational:
the set may grow, and a new source keeps its key ownership, so don't use it to
decide whether an extension applies.

## Handle failures

A phase fails when it returns an error other than `ErrDenied`, panics, or
returns refused parameters. With `FailClosed`, the call fails with the
Gateway's fixed unavailable error (HTTP 503, retryable). With `FailSkip`, the
phase's effect is discarded and the call continues. The Gateway counts and logs
every failure by extension, phase and class, never with the error's text.

Your extension's own logs, through `Dependencies.Logger`, are not filtered by
the Gateway. Keep prompts, outputs, credentials and customer data out of them.

## Configure operator extensions

Operator extensions read the `extensions` plugin setting, a YAML mapping from
extension name to its entry:

```ini
[plugin.grafana-aigateway-app]
extensions = $__file{/etc/grafana/aigateway/extensions.yaml}
```

```yaml
quota:
  config:
    limit: 1000
  secrets:
    apiKey: GF_PLUGIN_QUOTA_API_KEY
business-hours:
  enabled: false
```

- `config` is validated against the extension's `ConfigSchema`. Without a
  schema, an extension takes no configuration.
- `secrets` maps each declared secret to the environment variable holding it.
  The Gateway reads the variable after every other setting is validated, and
  redacts its value from the failure summaries it returns.
- An entry enables its extension unless it sets `enabled: false`. An extension
  without an entry uses `EnabledByDefault` and an empty configuration.
- The setting is at most 64 KiB. Unknown names, keys, secrets, aliases and
  invalid configuration fail startup with a reason that never includes values.

A distribution can list operator extensions in `Options.Mandatory`. Each must
have an enabled entry with a valid configuration and its secrets, or startup
fails. The shipped Gateway registers no extensions and no mandatory ones.

Tenant extensions aren't configured by this setting; see
[Configure tenant extensions](#configure-tenant-extensions).

## Configure tenant extensions

Org admins store a tenant extension's configuration in their stack namespace
as a `Middleware` resource named after the extension. The configuration goes
in the spec member named after the type, in lowerCamel case:

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

The Gateway doesn't apply stored resources yet. Until it does, a tenant
extension runs everywhere it applies when it is enabled by default, with an
empty configuration, whatever its namespace stores.

- `metadata.name` must equal `spec.type`, so a namespace holds one resource per
  extension. `enabled` is required.
- The spec may set only its type's member. With `enabled: true` the member is
  required, unless the type takes no configuration, in which case it has no
  member at all.
- The type must be a tenant extension this build implements. Operator
  extensions, unknown names and public types the build doesn't implement are
  refused with the same reason.
- Grafana checks the member's structure: JSON types, required fields and
  unknown fields. The Gateway then checks it against the type's full schema,
  including enums, patterns and bounds, and limits it to 64 KiB. Refusals name
  the field and the failing location, never a value.
- An update that sets `enabled: false` and keeps or removes the member is
  always accepted, even after a rollback to a build without the extension.
  After a rollback to a release whose manifest no longer declares the member,
  Grafana refuses writes that still carry it, so remove the member when you
  disable the resource. Any other change to a resource whose extension isn't
  implemented is refused.
- Resources aren't in folders. Viewers and Editors can read them
  (`aigateway:middlewarereader`) and Admins can write them
  (`aigateway:middlewarewriter`).
- The Gateway validates every create and update, so writes fail while the
  plugin is unavailable. Deletes always work.
- Resources can't hold secure values.

### Add a tenant extension type

A tenant extension's type is public: its name, configuration schema, default
and an example are declared in
[`tenanttype`](../../ai-gateway/pkg/extension/tenanttype/doc.go), and the
Gateway's manifest declares one member per type. Its implementation may be
private: bind it to the public type, and pass the definition with your other
extensions.

```go
definition := modelAccessType.Bind(tenanttype.Implementation{
	FailurePolicy: extension.FailSkip,
	New:           newModelAccess,
})
```

Startup fails when a tenant-scope definition has no public type, or differs
from its type in schema or default. To add a type:

1. Add it to the `tenanttype` registry with a description and an example.
2. Run `mise run generate-ai-gateway-kinds`, which projects the schema into the
   manifest and regenerates the kind. The projection keeps JSON types,
   properties, items, required fields and descriptions, and drops value
   constraints and defaults.
3. The generator refuses schemas the platform would prune or can't express:
   every node needs exactly one JSON type, every object either declares
   properties with `additionalProperties: false` or declares a typed
   `additionalProperties`, and combinators such as `oneOf` aren't supported.
   It also refuses a type whose example doesn't survive Grafana's pruning.

Grafana validates every write against the current schema, including fields
the write doesn't change. A type's schema can therefore only grow: add
optional fields, and never add a required field or narrow a type. An
incompatible change needs a new type name.

Prefer operator scope for private extensions. Use a tenant type only when
tenants need to configure the extension, and remember that its schema becomes
part of the public manifest.

## Observe extensions

Logs and model metrics record a denial as the outcome `denied`, not as an
error, so error rates exclude it. `grafana_ai_gateway_extension_events_total`
counts denials and failures by extension, phase and event. Extension instances
that implement `Close` are closed within the Gateway's final one-second flush,
after in-flight observations finish.

---

← [Authenticate to Grafana AI Gateway](gateway-authentication.md) · [Docs index](../README.md) · [How a request crosses AI Gateway →](../concepts/gateway-architecture.md)
