# Proposal

## Why

The frozen mature upstream reference adds the `interrupted` finish reason to LanguageModelV4, emits it from Anthropic streams cut off after output, and round-trips OpenAI shell execution controls. Moving pins without these changes fails existing conformance, the ProviderWire upstream-type witness and the conformance harness typecheck.

## What Changes

- Advance the coherent reference, consumer pins and lockfile using the repository-selected record.
- Add `FinishReasonInterrupted` and accept it in ProviderWire schemas and runtime, the Grafana client, Gateway logging/observability and metrics middleware.
- Report Anthropic streams that end after output without `message_stop` as `interrupted`, preserving the empty-stream and error paths.
- Stop tool execution, tool-choice enforcement and continuation for interrupted steps.
- Round-trip OpenAI shell `timeoutMs`/`maxOutputLength`.
- Adapt the conformance generator to the separate low-memory streaming function.
- Keep other assessed parity packages outside implementation scope.

## Capabilities

### Modified Capabilities

- `provider-v4-core-types`: interrupted unified finish reason.
- `anthropic-stream-finish-lifecycle`: end-of-stream interrupted finish.
- `stream-text-lifecycle`: interrupted steps end the invocation.
- `prometheus-middleware`: interrupted finish label.
- `openai-responses-provider`: shell execution controls.

## Impact

Root SDK, Anthropic/OpenAI/Grafana modules, middleware, Gateway schemas/runtime and TypeScript harnesses. Go module pins are unchanged; coordinated candidate-source checks establish integration, not independent publication readiness. Provider recordings and imported inputs remain unchanged.
