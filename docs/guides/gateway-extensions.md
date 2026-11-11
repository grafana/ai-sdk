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
- Transforms run tenant extensions first, then operator extensions, each by
  ascending `Priority`, then by name. The Gateway refuses a transform that adds
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

- **Key ownership.** `operator` calls use the operator's catalog and keys;
  `customer` calls use the customer's keys, from request BYOK or a stack
  catalog. `Keys` defaults to both.
- **Sources.** `Sources` narrows further to `operator-catalog`,
  `request-byok` or `stack-catalog`.

An extension that doesn't apply to a call runs none of its phases and gets no
record.

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
  apiKeyEnv: GF_PLUGIN_QUOTA_API_KEY
business-hours:
  enabled: false
```

- `config` is validated against the extension's `ConfigSchema`. Without a
  schema, an extension takes no configuration.
- Each declared secret is named by `<secret>Env`. The Gateway reads it from the
  environment after every other setting is validated, and redacts it from the
  failure summaries it returns.
- An entry enables its extension unless it sets `enabled: false`. An extension
  without an entry uses `EnabledByDefault` and an empty configuration.
- The setting is at most 64 KiB. Unknown names, keys, aliases and invalid
  configuration fail startup with a reason that names the field, not its value.

A distribution can list operator extensions in `Options.Mandatory`. Each must
have an enabled entry with a valid configuration and its secrets, or startup
fails. The shipped Gateway registers no extensions and no mandatory ones.

Tenant extensions aren't configured by this setting; see
[Configure tenant extensions](#configure-tenant-extensions).

## Configure tenant extensions

Org admins store a tenant extension's configuration in their stack namespace
as a `Middleware` resource named after the extension:

```yaml
apiVersion: aigateway.ext.grafana.app/v1alpha1
kind: Middleware
metadata:
  name: default-instructions
spec:
  type: default-instructions
  enabled: true
  config:
    instructions: "Answer concisely."
```

Each call's tenant extensions follow its namespace's resources:

| Namespace stores | The extension |
|---|---|
| No resource | Uses its default: on with `{}` when `EnabledByDefault`, off otherwise |
| `enabled: false` | Is off |
| `enabled: true` with a valid `config` | Runs with that configuration |
| A `config` its schema no longer accepts, or a failing factory | Applies its failure policy, logged and counted |
| An enabled resource for a type this build doesn't register | Is skipped, logged once per namespace and type, and counted |

The rules for writing resources:

- `metadata.name` must equal `spec.type`, so a namespace holds one resource per
  extension. `enabled` is required, and `config` defaults to `{}`.
- The type must be a tenant extension the distribution registers. Operator
  extensions and unknown names are refused with the same reason.
- `config` must match the extension's `ConfigSchema` and is at most 64 KiB.
  Refusals name the field and the failing location, never a value.
- When the plugin has API access, an update that sets `enabled: false` and
  keeps `config` is always accepted, even after a rollback to a build without
  the extension. Any other change to
  a resource whose extension isn't registered is refused.
- Resources aren't in folders. Viewers and Editors can read them
  (`aigateway:middlewarereader`) and Admins can write them
  (`aigateway:middlewarewriter`).
- The Gateway validates every create and update, so writes fail while the
  plugin is unavailable. Deletes always work.
- Resources can't hold secure values.

Grafana prunes objects nested inside `config` before the Gateway sees it, so a
tenant extension's configuration is limited to top-level fields whose values
are scalars or arrays of scalars. Design `ConfigSchema` accordingly, and don't
use secret-bearing strings as keys: a schema that allows arbitrary keys can
name one in a refusal's location.

### How stored configuration takes effect

The Gateway reads a namespace's resources as the plugin itself, never as the
caller, so callers need no access to `Middleware` resources. It reads only for
calls that a tenant extension applies to, and caches each namespace:

- A namespace's first call waits for its read, which is bounded at 5 seconds.
  Concurrent first calls share one read.
- After one minute, calls keep using the cached configuration while one
  refresh runs in the background. A change therefore applies within about a
  minute of the namespace's next call.
- Each Gateway replica caches on its own, so replicas can disagree for up to
  that window.
- A failed read is never treated as "no resources". The last configuration
  read successfully keeps applying for up to 10 minutes, and failed reads are
  retried every 30 seconds. After that, or when the first read fails, every
  tenant extension in the namespace applies its failure policy: `skip`
  extensions are off and `fail-closed` ones refuse calls with the unavailable
  error. The Gateway never falls back to the more permissive default.

Prefer `skip` for tenant extensions. A `fail-closed` tenant extension turns an
outage of Grafana's API into refused calls in every namespace it applies to.
A gate that must always run is a mandatory operator extension instead.

The plugin needs API access from Grafana to read resources. On a self-hosted
Grafana that gives it none, tenant extensions run with their defaults in every
namespace, the Gateway logs this once at startup, and it refuses every
`Middleware` create and update with a `400` that says stored configuration
would not be applied. Deletes still work.

### Write tenant extensions

- A tenant extension's factory runs once for each namespace configuration that
  enables it, and again when that configuration changes. Keep it quick and free
  of I/O. Its instance is closed once its configuration is replaced or the
  namespace leaves the cache, and only after the calls using it finish.
- All instances of a definition share `Dependencies.Registerer`, and
  registering the same collector again succeeds. Create collectors once per
  definition, register them in `New`, and never unregister them in `Close`: a
  collector created anew in each `New` fails registration from the second
  namespace on.
- Tenant extensions see the same request information as operator extensions,
  including the caller's identity, and the provider instances and provider
  models of operator-catalog candidates and attempts.

## Observe extensions

Logs and model metrics record a denial as the outcome `denied`, not as an
error, so error rates exclude it. `grafana_ai_gateway_extension_events_total`
counts denials and failures by extension, phase and event, including phase
`configure` for a namespace configuration its extension refuses.
`grafana_ai_gateway_tenant_config_reads_total` counts namespace reads by
`result` (`success` or `failed`), and
`grafana_ai_gateway_tenant_config_unknown_types_total` counts enabled
resources of types the build doesn't register. Read failures are logged with
the namespace and a fixed class, such as `refused` or `timeout`. Extension instances
that implement `Close` are closed within the Gateway's final one-second flush,
after in-flight observations finish.

---

← [Authenticate to Grafana AI Gateway](gateway-authentication.md) · [Docs index](../README.md) · [How a request crosses AI Gateway →](../concepts/gateway-architecture.md)
