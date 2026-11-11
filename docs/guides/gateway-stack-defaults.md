# Set stack defaults for AI Gateway calls

Org admins can give every model call in their stack default system
instructions and default sampling settings. Each is a `Middleware` resource in
the stack's namespace, named after its type:

- `default-instructions` adds system messages to calls that have none.
- `default-settings` fills sampling and limit settings that a call leaves
  unset.

Both are off until a resource enables them. They apply to every call in the
stack: calls to the stack's configured models and calls that bring their own
provider keys.

## Before you start

- Admins create, update and delete `Middleware` resources
  (`aigateway:middlewarewriter`). Viewers and Editors can read them
  (`aigateway:middlewarereader`).
- Resources aren't in folders and can't hold secure values.
- The Gateway reads resources through the API access Grafana gives the plugin.
  On a self-hosted Grafana that gives it none, every create and update is
  refused with a `400` that says stored configuration would not be applied,
  and the defaults stay off. Deletes always work.

The examples use Grafana's API with a service account token of an Admin in
`$GRAFANA_TOKEN`, and the stack namespace in `$NAMESPACE` (`default` for the
first org of a self-hosted Grafana):

```bash
MIDDLEWARES="$GRAFANA_URL/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/$NAMESPACE/middlewares"
```

## Set default instructions

A single message is the common case:

```yaml
apiVersion: aigateway.ext.grafana.app/v1alpha1
kind: Middleware
metadata:
  name: default-instructions
spec:
  type: default-instructions
  enabled: true
  defaultInstructions:
    messages:
      - content: You are an assistant for this Grafana stack. Answer concisely.
```

```bash
curl -fsS -X POST "$MIDDLEWARES" \
  -H "Authorization: Bearer $GRAFANA_TOKEN" -H "Content-Type: application/json" \
  -d '{"apiVersion":"aigateway.ext.grafana.app/v1alpha1","kind":"Middleware",
       "metadata":{"name":"default-instructions"},
       "spec":{"type":"default-instructions","enabled":true,
               "defaultInstructions":{"messages":[{"content":"Answer concisely."}]}}}'
```

List more messages to send several system messages, in order.

- The messages are added only to a call whose prompt has no system message.
  A call that sets its own instructions, or carries a system message anywhere
  in its history, keeps its prompt unchanged.
- Each message has only `content`, a non-empty string. Provider options per
  message aren't accepted.
- The messages are sent with every call they apply to, so they add input
  tokens to each one.

## Set default settings

```yaml
apiVersion: aigateway.ext.grafana.app/v1alpha1
kind: Middleware
metadata:
  name: default-settings
spec:
  type: default-settings
  enabled: true
  defaultSettings:
    temperature: 0.2
    maxOutputTokens: 2048
    stopSequences: ["END"]
```

| Setting | JSON type | Accepted values |
| --- | --- | --- |
| `temperature`, `topP` | number | any |
| `presencePenalty`, `frequencyPenalty` | number | any |
| `topK`, `seed` | integer | signed 64-bit |
| `maxOutputTokens` | integer | at least 1 |
| `stopSequences` | list of strings | any |

Set at least one. Headers, provider options, tools, tool choice, response
format, reasoning and any other key are refused, so a stack's defaults can't
change how a call reaches its provider beyond these settings.

How defaults combine with a call:

- A default applies only to a setting the call leaves unset. A call's own
  value always wins, including `0` and an empty list of stop sequences.
- Settings combine one by one, not as a set. A default `topP` and a call's own
  `temperature` reach the provider together, and some models refuse requests
  that set both. For known Anthropic models the Gateway drops `topP` with a
  warning instead. Prefer defaults for settings your callers don't set
  themselves.
- Some models ignore sampling settings. Recent Anthropic models, such as
  Claude Sonnet 5 and Claude Opus 4.7 and later, don't accept `temperature`,
  `topP` or `topK`, and neither does any Anthropic model with extended thinking
  on. The Gateway drops them with a warning, so those defaults have no effect
  on such calls.
- Providers check values against their own limits. A default the provider
  doesn't accept, such as a `maxOutputTokens` above a model's limit, fails
  every call it applies to. Settings a provider doesn't support, such as a
  seed for Anthropic models, are dropped with a warning.

## Change, disable and delete

- To change a resource, read it, edit its spec and write it back with its
  `metadata.resourceVersion` (`PUT $MIDDLEWARES/<name>`).
- To disable it, set `enabled: false`. You may keep or remove the member. A
  disable-only update is always accepted while the plugin has API access, even
  after a rollback to a release without the type. After a rollback to a
  release whose manifest no longer declares the member, remove the member when
  you disable.
- To remove it, `DELETE $MIDDLEWARES/<name>`. The stack then uses the
  default, which is off for both types.

A refused write names the failing field and its location in the member, never
the value you sent.

## How changes take effect

The Gateway reads the `Middleware` resources of every namespace it serves
calls for, as the plugin and never as the caller, and caches them:

- The first call in a namespace, on each Gateway replica, waits for that read,
  for at most 5 seconds. Later calls use the cache.
- A change applies within about a minute of the namespace's next call, on each
  replica independently, so replicas can disagree briefly.
- If a read fails, the last configuration read successfully keeps applying for
  up to 10 minutes. After that, or when the first read fails, both defaults
  are off until a read succeeds.

The cache's TTL, staleness bound, read timeout and namespace bound are fixed in
this version. The [extension guide](gateway-extensions.md#how-stored-configuration-takes-effect)
describes them and their metrics.

---

← [Extend Grafana AI Gateway](gateway-extensions.md) · [Docs index](../README.md) · [How a request crosses AI Gateway →](../concepts/gateway-architecture.md)
