# Inbound Anthropic Messages conformance

This Gateway-owned harness runs the real command built from candidate source.
Its protocol consumer/oracle is the pinned official Anthropic SDK, independently
of the Go adapter's mapping code:

- Go: `github.com/anthropics/anthropic-sdk-go` v1.80.0, pinned in the Gateway module.
- TypeScript: `@anthropic-ai/sdk` 0.133.0, pinned in this workspace.
- Canonical provider reference: the versions and commit in
  [`upstream.yaml`](../../../test/conformance/upstream.yaml).

## Run

```sh
mise run test-anthropic-messages-conformance
mise run test-ai-gateway-command
```

The second task runs the existing ProviderWire command tests, this suite, and
Messages SDK smoke tests. It is included in the required integration CI job via
`mise run test-integration`. TypeScript is typechecked as part of these tasks.

## Fixture replay contract

For each fixture, each official SDK consumes the same native response twice:
first directly from a local replay server, then through the real Gateway command
and its Anthropic provider. The assembled messages must agree for the supported
contract: message ID/type/role, ordered text and tool IDs/names/inputs, stop reason,
stop sequence, and uncached/output/cache token counts. Gateway output is also
checked for one message start/stop, contiguous content indices, correctly nested
content start/delta/stop events, and terminal event order.

| Input | Provenance | Contract exercised |
| --- | --- | --- |
| `recorded/simple-text/input.chunks.txt` | Existing provider recording | Text, ID, stop, usage |
| `recorded/tool-call/input-{1,2}.chunks.txt` | Existing provider recordings | Tool arguments and client continuation |
| `recorded/parallel-tool-calls/input-{1,2}.chunks.txt` | Existing provider recordings | Multiple tools, IDs, block order and continuation |
| `upstream/message-delta-input-tokens/input.chunks.txt` | Imported pinned upstream fixture | Input usage updated at completion |
| `upstream/tool-call-no-args/input.chunks.txt` | Imported pinned upstream fixture | Text followed by empty tool arguments |

Paths above are relative to
[`test/conformance/anthropic`](../../../test/conformance/anthropic).
Imported fixtures are registered in its `upstream/INDEX.yaml`. These files are
read in place and remain unchanged. The server adds SSE framing around each
original JSON line, including ping events. It does not truncate, splice, or
rewrite provider events. Client prompts and tool results in the replay harness
are synthetic; replay does not prove that a live provider accepts those requests.

The comparison intentionally projects the documented supported fields. Public
model IDs differ from backend model IDs and are checked separately in synthetic
unary cases. Absent cache counters normalize to zero, and absent stop-sequence
values normalize to null. Provider extras such as service tier, cache lifetime,
and direct-tool caller metadata are outside the initial response contract.
Transport pings and text-delta segmentation are not required to be identical.

## Synthetic boundaries

Separately labeled cases test unary `end_turn`, `max_tokens`, `stop_sequence`,
and `tool_use` replies with nonzero cache counts; fixed errors before streaming;
errors after content without successful completion; and delivery of a content
delta while the provider is deliberately held open behind a test-controlled gate.
The existing command smoke tests cover authentication, routing, unsupported
listener access, tool options, and cancellation.

Synthetic responses are generated in test code, never placed in `recorded/` or
`upstream/`. These checks establish the tested inbound protocol subset, not live
Anthropic acceptance, complete Anthropic API equivalence, or Claude Code support.
They do not replace the Vercel SDK provider/UI conformance suite.
