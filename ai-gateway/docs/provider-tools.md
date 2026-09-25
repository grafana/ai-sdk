# Provider-defined tools

Direct routes accept provider definitions with an ID, name and object args,
and return provider-executed calls and non-null JSON results. Function-only
fields are rejected on provider definitions. Provider-defined does not mean
provider-executed: callers use the returned ownership marker, not the definition,
to decide whether application execution is needed.

Unary and streaming calls share history-aware correlation. A provider-owned
call may finish without a result and receive its result in a later request when
the client supplies that unresolved assistant call in history. Streaming allows
multiple preliminary results followed by one final result; unfinished previews
fail safely. State is bounded and request-local, not a Gateway session or executor.

## Continuation and privacy

Tool-part options retain ordinary provider continuation values under the
Gateway's protected-field and configured-backend policy; host-owned controls
remain rejected. Public tool metadata uses an allowlist: Anthropic caller identity
and OpenAI/Azure item, namespace and caller correlation. Arbitrary metadata,
physical backend identity and credentials are not normalized public output.
Logical observation remains metadata-only; tool names, IDs, inputs and results
are not logs, metric labels or exported payloads. Fallback routes reject tools
and tool history before running any candidate.

## Support boundaries

Root, message and part provider options and call headers follow the Gateway's
configured-backend and protected-field policy. Direct Anthropic routes also
accept validated `providerOptions.anthropic.mcpServers` alongside ordinary
allowed options. Other routes reject MCP before invoking a backend, even without
tool definitions. Protected spelling variants and nested MCP options remain
rejected. A nonempty server list is effectful and cannot be retried across
fallback candidates.

## Anthropic-hosted MCP

The caller supplies distinct nonempty server names and bounded HTTPS URLs
without embedded credentials or fragments. Optional authorization tokens and
tool configuration are forwarded only in the selected native request. Absent
versus explicitly empty tokens and absent versus false enabled flags remain
distinct. Anthropic connects to the remote server; the Gateway does not. These
are Grafana policy checks, not evidence of Vercel's private hosted-service
policy or Anthropic's egress controls.

Tool-call/result continuation metadata may identify only a server configured in
the current request. Public MCP metadata retains type and server name, not the
URL, token, backend identity or unrelated caller fields. The native request and
the Go client's caller-owned request body necessarily contain the submitted
configuration; do not log either body. The Gateway's context deadline bounds
unary native attempts without reducing their configured token budget.

Approvals, sources, files and other media retain their explicit unsupported
failures. Image previews emitted before their tool call remain WP16 (#110),
not correlated provider-tool results. The existing byte, frame, part, duration,
writer/cancellation and cleanup limits continue to apply.

## Validation and rollout

Both registered clients are tested against the real handler and authenticated
Gateway command, including native Anthropic code-execution alias/continuation
requests. Fake native responses are deterministic transport evidence, not
provider recordings. No deployment activation or live MCP egress is claimed.
Before rollout, use the reviewed image and corresponding-source notices; rollback
uses the prior image/module set and requires no persisted-state migration.
