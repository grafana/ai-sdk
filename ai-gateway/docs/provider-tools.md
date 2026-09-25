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
existing configured-backend and protected-field policy. Anthropic `mcpServers`
and MCP continuation/output metadata remain rejected rather than silently
reinterpreted as ordinary tools. The `gateway-anthropic-mcp` change separately
owns remote server options, route eligibility and name validation.

Approvals, sources, files and other media retain their explicit unsupported
failures. Image previews emitted before their tool call remain WP16 (#110),
not correlated provider-tool results. The existing byte, frame, part, duration,
writer/cancellation and cleanup limits continue to apply.

## Validation and rollout

Both registered clients are tested against the real handler and authenticated
Gateway command, including native Anthropic code-execution alias/continuation
requests. Fake native responses are deterministic transport evidence, not
provider recordings. No deployment activation or live-provider smoke is claimed.
Before rollout, use the reviewed image and corresponding-source notices; rollback
uses the prior image/module set and requires no persisted-state migration.
