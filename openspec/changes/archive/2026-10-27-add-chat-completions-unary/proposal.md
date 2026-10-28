## Why

OpenAI Chat Completions callers need a protocol adapter over the Gateway's
provider domain. The first change uses the shared execution runtime for request translation and unary
responses so they can be reviewed independently of streaming and App Platform
route composition. It replaces the standalone-server assumptions of PR #248
with the resource ownership already established by #428, #518 and #532.

## What Changes

- Add an AGPL unary handler, closed request schema, typed request mapping and
  fixed OpenAI error envelopes under `ai-gateway/openai/chatcompletions`.
- Represent text, client-owned function calls and continuations, structured
  output and reasoning settings without adding server-side tool execution.
- Preserve available native response identity, successful warnings and optional
  usage; reject unsupported output safely.
- Supply per-attempt Chat translation without changing calls from other
  protocols, and inherit request duration and body limits from the host.
- Verify the handler with unit tests and the official OpenAI Go SDK.

## Non-goals

This change does not register a plugin route, authenticate callers, compose the
production catalog, support streaming or BYOK, add SDK-wide OpenAI Chat provider
support, or establish TypeScript/plugin-process/live-provider compatibility.
`stream:true` is rejected before resolution. Those integration and streaming
capabilities are separate changes in the stack.

## Capabilities

### New Capabilities

- `gateway-openai-chat-completions-adapter`: independent unary Chat request and
  response contract, with host-owned resources and native defaults.

### Modified Capabilities

None. Existing Gateway routes and ProviderWire behavior remain unchanged.

## Impact

Only the AGPL Gateway module gains a package. No production route, deployment
configuration, provider credentials, Apache SDK dependency or public discovery
surface changes. The official Go client test consumes an HTTP test handler;
it does not launch the Gateway plugin.

## Restack on 2026-10-28

The constructor now accepts execution.Runtime. Runtime.Call/Call.Generate,
WithTranslator and shared classification replace the original resolver, local
worker and context-marker wrapper. This records the rebased architecture, not
a claim that these interfaces existed in the October 27 prototype. #555, #579
and #581 provide runtime/attempt visibility/host composition; the common
lifecycle validator in #483 remains pending before readiness.
