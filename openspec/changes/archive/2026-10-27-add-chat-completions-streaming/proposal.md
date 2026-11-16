## Why

The second Chat adapter draft extends the unary protocol prototype with SSE
consumption by OpenAI clients. Separating framing, identity and function deltas
allows review of the codec separately from the shared execution contract.

The restack uses #555 execution runtime, #579 attempt visibility and #581 host
contract. The remaining #483 shared lifecycle validator is still a blocker to
readiness; #394 is not closed by this adapter. Earlier prototype execution,
drain and idle ownership is replaced by the runtime.

## What Changes

- Accept `stream:true` and emit Chat completion chunks, finish, optional usage
  and `[DONE]` on successful completion.
- Map function argument fragments and freeze public identity at first output;
  retain warnings and late native identity in the Grafana extension.
- Use runtime cancellation, native-part idle accounting and bounded cleanup, without restoring body/frame/part/total-duration limits.
- Add focused stream, cleanup and official OpenAI Go client tests.

## Non-goals

No plugin route, authentication, BYOK, TypeScript command proof, public runtime,
provider recordings, live-provider proof or completion of #394. The adapter
does not replay committed output, execute tools, expose reasoning text or add
hosted-tool/file representations.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-openai-chat-completions-adapter`: allow streaming beside unary and
  specify SSE framing, identity, representable progress and current cleanup.

## Impact

The AGPL Chat package and its tests change. Existing production Gateway routes
remain unchanged by this streaming cut. It depends on the runtime-based unary
cut; the shared lifecycle validator remains a later dependency.
