# Design

## Context

The Gateway is a Grafana app plugin. The platform authenticates callers and
forwards routes to the plugin, which composes one route table: each entry has a
method, path, telemetry label, handler and host-error writer, and gets the shared
access decision and telemetry (`gateway-plugin-host`). Adapters hold no
selection, fallback, attempt or stream-validation logic; they take an
`execution.Runtime` call, map the request into provider call options, and map
the result back. `gateway-openai-chat-completions-adapter` is the existing
example. The first Messages draft (#488) predates this and carried its own
listener, authentication shim and size budgets.

The Anthropic provider already converts a system message that follows other
messages into Anthropic's mid-conversation system block, so the adapter does not
need to reorder or merge system content. It also handles thinking and cache
control through provider options, which the adapter does not set yet.

There is no upstream (Vercel AI SDK) counterpart for an inbound Anthropic
server, so the references are the Anthropic Messages API and the Claude Code
gateway protocol guide linked from #427. Provider conversion parity is
unchanged because this change touches no provider code.

See `proposal.md` for the motivation and the list of deferred behavior.

## Goals / Non-Goals

**Goals:**
- A working `v1/messages` route that Claude Code can drive through a multi-turn
  tool session against a configured model.
- Match the host contract exactly: one route-table entry, no private selector,
  dispatcher, drain or limit.
- State what is and is not applied, so nobody infers compatibility from a
  passing text test.

**Non-Goals:**
- Thinking and signature replay, cache-control preservation, `count_tokens`,
  compaction, model discovery. These need provider-option work or a second
  route and have their own follow-ups under #427.
- Credential flows other than the platform's bearer token.

## Decisions

**Rebuild on the route table instead of rebasing #488.** Nearly every #488 file
targeted a removed command, and its authentication and limits are now host
concerns. The request and stream mapping, tests and OpenSpec content are reused
after review; the wiring is new. Alternative considered: rebase and edit. It
would keep a history built on a deleted architecture and produce conflicts in
files that no longer exist.

**One package, `ai-gateway/anthropic/messages`, mirroring
`openai/chatcompletions`.** It exports `New(runtime)`, `Path`, `WriteError` and a
host-error writer, so the route table registers it the way it registers Chat.
Alternative: a subpackage of `providerwire`. Rejected because Messages is a
different public protocol with its own errors.

**Accept wire-only fields; do not apply them yet.** (Refusing them was
considered for the first release and rejected: Claude Code sends all of them on
every request.) Claude Code sends
`cache_control`, `metadata`, `thinking`, `context_management` and `output_config`
on every request. Chat uses a closed request schema and refuses unknown fields;
doing that here would make Claude Code fail on its first call. The adapter
decodes with ordinary JSON, ignores what it does not consume, and refuses only
content it cannot represent (images, documents, server tools). The cost is that
a client can ask for something and not get it. The spec and client guide state
each case, and the follow-ups apply them through provider options so the same
field set stops being ignored. Alternative: refuse until each is supported.
That blocks the first end-to-end evidence this change exists to produce.

**Bearer authentication only.** The platform does not accept `x-api-key`; the
adapter cannot change that and does not parse credentials. The guide tells
Anthropic clients to use their bearer option (`ANTHROPIC_AUTH_TOKEN` for Claude
Code). This also keeps the incoming credential out of provider calls, because
the plugin never sees it.

**Mid-conversation `system` role becomes a provider system message.** Claude Code
sends its reminders this way. Mapping it to `RoleSystem` at its position lets the
Anthropic provider emit the supported mid-conversation form. Merging it into the
initial system prompt would change where the model reads it. Dropping it would
silently remove instructions.

**Stream through `execution.Stream`.** The runtime validates part lifecycle, owns
idle and drain times, and reports terminal outcomes; the adapter only turns
parts into Anthropic events and ends with `error` instead of `message_stop` on
any failure after the first byte. Alternative: read the provider channel
directly as #488 did. That would repeat validation and timing the runtime owns.

**Fixed error documents.** Statuses come from the runtime's failure categories,
as in Chat. Cancellation uses 499 internally for classification and writes
nothing when the caller is already gone.

**Evidence in three tiers.** Handler tests with the official Go client run in CI
without a key. A command-level scenario against the Gateway with the end-to-end
stand-in backend runs in CI without a key and can drive Claude Code with a
scripted tool call. A live-model Claude Code session is a manual check that
needs an Anthropic API key and records the client version. The captured Claude
Code request used while developing this change is not committed verbatim, since
it contains the client's full system prompt; fixtures keep the structure
(block types, cache-control placement, tool shapes) with placeholder text.
These are client request fixtures, not recorded provider events, so the
provider-fixture provenance rules do not apply to them.

## Risks / Trade-offs

- [A client asks for thinking, caching or parallel-tool control and silently
  gets none] → The spec lists each field; the guide repeats the list; follow-ups
  under #427 apply them. Do not describe the route as Claude Code compatible
  until the compatibility pass lands.
- [`disable_parallel_tool_use` depends on each target honoring its parallel-call
  option] → Applied through a per-target translator, as Chat does for
  `parallel_tool_calls`; a target that ignores its option can still return
  several calls.
- [Dropping thinking blocks from history may change quality or break signed
  history on models that require it] → Observed fine on a short tool session
  only; the compatibility pass must exercise adaptive thinking and
  interleaved thinking.
- [Claude Code calls an endpoint the Gateway does not serve, such as
  `count_tokens`] → Not observed in the first sessions; unknown for longer ones.
  Add the endpoint only if a pinned client needs it.
- [Claude Code's model names do not match catalog aliases] → Operators set
  model aliases in the catalog, or clients set the model environment
  variables. The guide documents both.
- [Client drift: Claude Code changes the fields it sends] → Evidence names the
  client version; a version bump re-runs the smoke scenario.

## Open Questions

- Whether `count_tokens` and model discovery belong in the first supported
  scope. #427 says to decide separately and not to block inference on them.
- Where the internal instructions for obtaining an Anthropic API key live. They
  are not repository content.
