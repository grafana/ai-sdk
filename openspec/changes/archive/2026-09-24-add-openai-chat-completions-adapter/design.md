## Context

The Gateway exposes POST /v1/chat/completions as an independent AGPL protocol adapter over the canonical provider domain. Protocol authority is the pinned official OpenAI Go/JavaScript SDKs; provider semantics follow the registered upstream baseline.

## Goals / Non-Goals

Support bounded text, consumer-owned functions, structured output and reasoning settings through configured providers and fallback. Full OpenAI API coverage, BYOK, hosted execution and aggregate admission are outside this adapter.

## Decisions

The adapter uses explicit closed request schema validation, typed DTOs, bounded unary/SSE mapping and native candidate-local default translation. The catalog supplies existing authentication, routing, logical middleware and fallback. No model allowlist or additional fallback capability guard is retained.

Native identity is preserved before initial stream content and stable afterward, with missing fields filled once. Native warnings and late identity use the bounded optional Grafana extension, consumed through official SDK response/chunk access. Defaults preserve Chat storage and strictness semantics on each candidate. Functions remain consumer-owned and selected streams never replay.

Cancellation has one channel owner and asynchronous bounded cleanup. Request, frame, response, part-count, total and idle limits remain active; diagnostic-only traffic does not reset visible-progress idle time.

## Risks / Trade-offs

Synthetic HTTP proof does not establish live acceptance, deployment isolation or aggregate resource guarantees. SDK accumulators need not retain extension fields; callers collect them from chunks. Missing native identity uses documented fallbacks without fabricating selected-candidate attribution.

## Migration Plan

Additive route; existing ProviderWire callers remain unchanged. The review follow-up `harden-chat-completions-adapter` records the approved corrections and expanded validation. Rollback removes Chat composition without changing the shared foundations.
