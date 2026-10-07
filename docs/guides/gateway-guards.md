# Enforce operator policy with Gateway guards

AI Gateway can evaluate text calls against an operator-owned Sigil policy before
and after inference. Guards are disabled by default. Enabling guards requires
both phases for unary and streaming calls; there is no preflight-only mode.

The policy tenant belongs to the Gateway operator, not the caller. Caller
credentials, tenant headers, provider options, and bypass headers cannot select
another policy or disable evaluation. Gateway authenticates and validates the
request, then resolves the public model before preflight.

## Understand content disclosure

If you enable guards, the operator's Sigil service receives the full supported
prompt, tool definitions, conversation history, and represented thinking.
Postflight sends the effective request again, together with the complete
supported output. Treat this as content disclosure to another service.

Gateway generation recording remains metadata-only. That recording setting does
not limit the content sent for policy evaluation. Guard credentials and the
policy endpoint are independent of the Agent Observability exporter. Review
Sigil access, retention, evaluators, and downstream judge destinations before
sending sensitive content.

Guard context identifies the model as provider `grafana` and the resolved public
model ID, including when the caller uses an alias. The fixed agent name is
`grafana-ai-gateway`. Rules must select that identity, not a backend provider or
fallback model. Gateway does not infer conversation identity from its request
correlation ID. Without genuine conversation correlation, Sigil does not persist
its conversation guard-decision row.

## Check the supported request boundary

Guarded calls support text, represented reasoning, and function-tool payloads.
Preflight includes system messages, history, tool descriptions, schemas,
arguments, and supported text or JSON results. Postflight includes completed
text, represented reasoning, and supported function-tool output.

Enabling guards deliberately reduces the unguarded Gateway request surface.
Gateway rejects unsupported input with HTTP 400 instead of passing content that
Sigil cannot inspect. Unsupported input includes:

- Media and files, including media nested in tool results or reasoning.
- Tool input examples and provider-defined tools.
- Stored conversation, response, or item references that substitute remote history.
- Hidden instructions and provider options that introduce uninspected input.
- Request options outside the strict guard allowlist, including schema-bearing
  response formats.
- Model-call headers other than `User-Agent` that remain after wire mapping,
  even if their values are empty.

The guarded allowlist preserves ordinary output-token, sampling, penalty, seed,
reasoning-effort, and function-tool-choice controls. Function definitions retain
strict mode. Response formats may select text or JSON without a schema, name,
or description. Stop sequences and all call-, message-, tool-, or result-level
provider options are rejected. Wire mapping strips `ai-language-model-*` and
`ai-o11y-*` headers before guard validation. Credential-bearing header overrides
remain rejected. Case-insensitive `User-Agent` is permitted for SDK identification
and is not copied to Sigil. Part-level options are rejected except an
Anthropic signature on nonempty represented reasoning; opaque redacted reasoning
is unsupported.

Tool schemas and function arguments must be JSON objects. Tool results may carry
text, JSON, execution-denied reasons, or text-only multipart content. Invalid
JSON and duplicate keys are rejected. Guard mapping preserves numeric values;
Sigil's JSON canonicalization can still affect what an evaluator reads.

Before enabling guards, check your application's exact options against the guard
allowlist. Unguarded support does not imply guarded support. Unsupported generated
content, including files, reasoning files, sources, and opaque reasoning payloads,
is also a closed failure. Guards do not silently remove it and approve a partial
answer. Redacted or encrypted reasoning remains unsupported even when visible
reasoning text accompanies it. If a call has tool-argument deltas, their
compacted JSON must match the final call. Object-key order and number spelling
must also match. Standalone calls and completed calls without argument deltas
are supported.

A supported preflight transform updates the actual provider request atomically.
Fallback candidates receive that effective request, never the original content
as a substitute. Gateway rejects unsafe transforms with HTTP 424 under either
failure policy. Invalid roles, changed tool identity, changed reasoning, and
numeric changes caused by JSON rounding are unsafe. Signed reasoning cannot
change while keeping its signature.

For an active operation, an explicit postflight deny returns fixed HTTP 403.
That applies even if the response includes `transformed_input` or malformed
optional diagnostics. If postflight explicitly allows the request but includes
`transformed_input`, Gateway returns fixed HTTP 424 under both failure policies,
even when the transform is unchanged. Neither error releases original output or
tool-argument deltas. Do not configure postflight redaction expecting rewritten
responses.

## Configure the guard destination

Run `grafana-ai-gateway --help` for the command's settings and validation limits.
Every `guards.*` flag has a `GRAFANA_AI_GATEWAY_GUARDS_*` environment binding;
replace dots and hyphens with underscores and use uppercase letters.

Set `--guards.enabled=true`, `--guards.endpoint`, and `--guards.tenant-id`.
The endpoint is an explicit base URL. Gateway preserves its path prefix when
joining `api/v1/hooks:evaluate`. For example, `https://sigil.example.com/sigil/`
produces `https://sigil.example.com/sigil/api/v1/hooks:evaluate`.
Do not supply the full hook path as the base URL.

Production requires HTTPS. URLs cannot contain credentials, a query, or a
fragment. Gateway refuses redirects and does not retry hook requests internally.
The operator must supply the final reachable ingress endpoint.

Select `--guards.auth-mode=basic` or `--guards.auth-mode=bearer` explicitly.
There is no authentication-mode default. Basic authentication also requires
`--guards.auth-username`. Bearer authentication rejects a username.
`--guards.auth-secret-env` names the environment variable containing the Basic
password or bearer token. Gateway resolves the secret before binding listeners.
Do not put the secret in a command argument, model YAML, or image layer.

Grafana Cloud documentation uses the instance ID as the Basic username and a
Cloud Access Policy token with `sigil:write` as the password. This does not prove
that a particular deployed hook ingress accepts those credentials. Confirm the
actual endpoint, tenant, and ingress authorization with the deployment owner.
Do not assume the generation-export credential authenticates hooks.

For a container already configured with models and Gateway authentication, pass
these additional environment variables:

```bash
export GRAFANA_AI_GATEWAY_GUARDS_ENABLED=true
export GRAFANA_AI_GATEWAY_GUARDS_ENDPOINT=https://sigil.example.com/sigil/
export GRAFANA_AI_GATEWAY_GUARDS_TENANT_ID=operator-policy-tenant
export GRAFANA_AI_GATEWAY_GUARDS_AUTH_MODE=basic
export GRAFANA_AI_GATEWAY_GUARDS_AUTH_USERNAME=operator-instance-id
export GRAFANA_AI_GATEWAY_GUARDS_AUTH_SECRET_ENV=GATEWAY_GUARD_TOKEN
# Supply GATEWAY_GUARD_TOKEN through your runtime secret mechanism.
```

Forward these variables and `GATEWAY_GUARD_TOKEN` to the container with `--env`.
The example uses placeholders; it does not identify a deployed Sigil endpoint.
See [Run AI Gateway in a container](ai-gateway-container.md) for the remaining
process configuration.

## Choose failure handling

`--guards.fail-open` defaults to `false`. A valid explicit deny always returns
HTTP 403 `forbidden`, including under fail-open. Closed guard failures return
HTTP 424 `failed_dependency`. Public errors use fixed messages and `param:null`;
Gateway does not expose Sigil reasons, rule IDs, or endpoint details.

With fail-open enabled, eligible guard-service failures can continue the
operation. Examples include transport failures, non-200 responses, and missing,
unknown, duplicate, or malformed verdicts. Gateway records the fixed `fail_open`
outcome. Allowing a request after a service failure does not mean a rule approved
that request.

Fail-open never overrides a deny, unsafe preflight transform, any postflight
transform, unsupported input, cancellation, or local resource exhaustion.
Admission exhaustion and guard buffer/body exhaustion return local HTTP 424.
Caller cancellation and the operation deadline stop work rather than permitting
buffered output release.

## Account for streaming latency and resources

Gateway withholds successful headers and content until output validation and
postflight finish. A guarded stream therefore delivers no tokens while the model
generates. After approval, Gateway releases the validated buffered frames.
Unary output is also withheld until postflight approves it.

A valid stream finish is required. End-of-file without a valid finish is not an
approved completion. A valid finish can proceed to postflight without waiting
for the provider channel to close. Postflight denial or failure does not start
another provider attempt. Preflight runs once outside logical fallback;
postflight checks the selected complete output once.

The default `--providerwire.model-duration=120s` bounds the whole guarded
operation, including both phases and inference. `--guards.timeout=5s` bounds
each hook phase within the remaining operation budget. Existing stream idle,
part, and frame limits still apply. Configure proxies and clients to tolerate
the full generation-plus-postflight delay, not only the normal first-token time.

The initial guard limits are starting settings, not measured capacity:

- `--guards.body-bytes=4194304` limits hook requests and decompressed responses
  to at most 4 MiB.
- `--guards.retained-bytes=8388608` budgets 8 MiB of additional retained guard
  data per call. Configuration permits at most 64 MiB.
- `--guards.max-concurrent=8` admits at most eight active guarded operations.
  Configuration permits at most 128.

Accounting includes projections, serialization, transform copies, and buffered
frames. Conservative charges can reject content below the nominal byte limits.
The retained budget is not a process heap cap. Providers, transports, and
observers consume additional memory. Metadata-only recording already retains
content before filtering export. Measure production memory and latency with
recording enabled before increasing concurrency or limits.

## Verify the policy before rollout

A strict client can validate a verdict; it cannot prove that a server-side rule
executed. Sigil can return allow when no rule matches, an evaluator or policy
service is disabled, or a server-side transform fails and evaluation continues.
Verify enablement and matching rules for both phases in the intended tenant.

Sending thinking or tool results does not prove every evaluator checks them.
Ordinary regex and heuristic flattened text excludes thinking and tool results.
Thinking is not redacted by the referenced Sigil transform implementation.
These server behaviors were source-checked at
[Sigil `de1631e89`](https://github.com/grafana/sigil/tree/de1631e8902b2c3e94202022ab3e88136607d3ec/sigil/internal/eval/hooks).
Check each evaluator's field coverage. Judges need an input target for preflight
rather than the default response target; verify the output target for postflight.
Judge content limits can truncate input below the HTTP body limit.

Keep guard judges on a direct provider route or a separately isolated route.
Routing a judge through the guarded Gateway can recurse. An untrusted bypass
header is not a recursion-prevention mechanism.

Postflight can prevent a consumer-owned tool loop from receiving a denied call.
It cannot undo provider-executed side effects or provider usage already incurred.
Local source-checked codecs and synthetic tests do not prove deployed ingress,
rule configuration, live Sigil compatibility, or production capacity. Arrange a
separately approved deployment check before activation.

---

Prev: [Run AI Gateway in a container](ai-gateway-container.md) | Up: [Docs index](../README.md) | Next: [Production checklist](../best-practices/production.md)
