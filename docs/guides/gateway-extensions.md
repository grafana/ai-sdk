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
| Admit | Namespace, key ownership, source, caller type, correlation ID and model. No prompt. | Deny |
| Transform request | The request information and the call parameters | Change the parameters |
| Inspect request | The parameters the model will receive | Deny |
| *Model call* | One call to the selected model, with its fallback chain | — |
| Inspect response | Each normalized output part: text, reasoning, a complete tool call, finish | Abort |
| Observe | One record per call: outcome, the denying extension and phase, usage and timing | Nothing |

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

Tenant extensions aren't configured by this setting. Until per-namespace
configuration exists, a tenant extension runs everywhere it applies when it is
enabled by default.

## Observe extensions

Logs and model metrics record a denial as the outcome `denied`, not as an
error, so error rates exclude it. `grafana_ai_gateway_extension_events_total`
counts denials and failures by extension, phase and event. Extension instances
that implement `Close` are closed within the Gateway's final one-second flush,
after in-flight observations finish.

---

← [Authenticate to Grafana AI Gateway](gateway-authentication.md) · [Docs index](../README.md) · [How a request crosses AI Gateway →](../concepts/gateway-architecture.md)
