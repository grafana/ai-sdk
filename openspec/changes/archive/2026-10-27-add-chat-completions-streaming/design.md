## Context

This streaming change is restacked on #555/#579/#581 on 2026-10-28. The former
private setup worker, visible-progress idle timer and drain loop are removed.
The registered upstream remains b7f6255acaae7724adc0fd37aac3dae3f3466578,
provider 4.0.29, OpenAI 4.0.96, Anthropic 4.0.82 and compatible 3.0.71.
Native provider parts are input; Chat SSE, not ProviderWire/UI SSE, is output.

## Goals / Non-Goals

Preserve incremental framing, stable public identity, represented function
arguments, warnings and explicit terminal outcomes through the shared runtime.
Do not implement execution workers, native credential input, hosted tools or
whole-stream buffering. Plugin registration and client-process proof are in a
separate change. #483 shared lifecycle validation is still not delivered.

## Decisions

### Runtime-owned setup, waits and cleanup

The handler calls Call.Stream, reads Stream.Next and defers Stream.Close.
Runtime wait categories determine termination/error classification; Chat only
maps them to its error document. There is no private setup handoff, local idle
timer, native failure classifier or drain consumer. Current synchronous setup
requires provider cooperation with cancellation; no bound on noncooperative
setup is claimed. #579 attempt capture/visibility stays in shared execution.

Idle now measures native-part activity in the runtime: metadata/reasoning parts
can keep the idle window active even when Chat emits no content. This is an
intentional correction to the October 27 visible-progress prototype and avoids
a second, conflicting policy. The host total deadline still bounds a request.

### Commitment and single native response

The first representable content or valid finish opens the assistant role chunk.
Text/tool deltas are encoded and flushed as they arrive. Native identity before
opening supplies id/model/created, and missing fields use unary fallbacks.
After opening these values freeze; late identity uses grafana.native_response
and stream-start warnings use grafana.warnings. Accumulator helpers may omit
extensions, so clients should inspect individual chunks.

Chat currently supports one native response segment. After its first validated
finish, one more runtime Next must return WaitClosed before the terminal Chat
finish/usage/[DONE] is emitted. Any next part, including metadata, a second
response start or finish, produces a safe error without a successful terminal
sequence. Earlier emitted content is not replayed. A draft #583 change to
ProviderWire sequential-segment semantics is not authority to truncate Chat at
its first native finish. #483 integration must preserve this explicit refusal
unless a separately designed Chat multi-response mapping is added.

### Protocol checks and pending lifecycle

Chat keeps function indices, fragment/final consistency, tool-choice and
parallel rules, JSON-schema validation, UTF-8 and representability. Shared
provider lifecycle validation is still pending in #483; current consume/finish
checks must not be described as that delivered validator. When it arrives,
remove duplicated generic validation only after ensuring the codec still
receives enough information to refuse unsupported sequential responses.

### Writer boundary

The feature codec flushes through http.ResponseController. Socket-deadline and
known Anthropic custom-metadata compatibility fixes remain a separate change.
Writer handling is HTTP framing/cancellation, not a replacement runtime timer.

## Risks / Trade-offs

JSON and function validation retain content in memory. Synthetic tests and
source inspection do not establish live-provider acceptance or deployed host
behavior. Runtime/host integration resolves part of #394; #483 remains a
readiness prerequisite. The single-segment refusal may reject a provider
feature that ProviderWire can represent; that is preferable to silent loss.

## Migration Plan

Retain wire tests while switching ownership to the shared runtime. Add route
and official-client evidence separately. Revalidate on #483 and inspect #583
interaction explicitly before declaring the adapter ready.
