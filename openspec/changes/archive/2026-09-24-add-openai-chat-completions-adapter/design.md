## Context

The Gateway already owns authentication, a canonical model catalog, observability, outbound clients and shutdown. ProviderWire is a separate protocol, not a Chat Completions adapter. OpenAI Responses defaults differ from Chat defaults and require explicit translation.

## Goals / Non-Goals

Goals: one-choice text and client-owned functions, certified structured output and reasoning, OpenAI errors/SSE, explicit limits, official SDK verification. Non-goals: developer messages, files/images/audio, hosted tools, persistence, moderation/refusal mapping, logprobs, prediction, service tiers, metadata, organization/project headers, streaming obfuscation, Responses fallback, or aggregate admission control.

## Decisions

- Independent `ai-gateway/openai/chatcompletions` package consumes only provider-domain types and catalog resolution; it never calls core GenerateText/StreamText orchestration.
- Host builds immutable canonical-ID policies from provider type and exact backend model ID before middleware hides backend identity. Unknown IDs fail closed on adapter requests.
- Adapter Bearer authentication delegates to existing access-token verification; X-Access-Token, X-Grafana-Id and X-Scope-* are rejected in access-token mode. Cloud mode reuses existing credential-rejection and trusted assertion verification unchanged.
- Responses and certified compatible requests always carry store false. Every function maps omission/null strict to false; Anthropic omits the unsupported explicit setting. Fallback is restricted to certified Anthropic candidates because storage overrides cannot cross the shared fallback guard. Explicit strict JSON schema validation is local only after successful stop.
- Duration fields are actual `time.Duration` values copied from existing validated host limits. Limits are per request; no claim of total process memory/concurrency bounds.
- SSE validates text and function lifecycles, matches complete function calls to accumulated fragments, drops private reasoning, emits usage separately when requested, and never fabricates DONE after failure/EOF.
- Setup uses one unbuffered ownership handoff. Cancellation cancels the provider then transfers a claimed stream to one asynchronous bounded drain owner; failure response completion never waits for the drain interval. Late setup retains its existing sole cleanup owner. A non-cooperative synchronous provider may outlive the handler; no generic Go mechanism can kill it safely.

## Risks / Trade-offs

Exact model allowlists intentionally narrow compatibility. Custom compatible endpoints still must satisfy the documented contract; matching model names is not provider attestation. Provider warning/default behavior requires deterministic backend tests. Response writers must support write deadlines for streaming. Slow-client total write deadlines bound socket backpressure, not aggregate admission. Local schema and stream buffers must remain bounded; malformed output fails closed.

## Migration Plan

Additive route. Existing configuration and ProviderWire clients stay valid. The adapter route reuses the configured limits; rollout documentation lists exact unsupported fields and model profiles. Rollback removes adapter composition without modifying the shared foundations.

## Open Questions

Broader certified model IDs, signed reasoning tool replay, global admission controls and wider JSON Schema vocabulary are follow-ups, not implied support.
