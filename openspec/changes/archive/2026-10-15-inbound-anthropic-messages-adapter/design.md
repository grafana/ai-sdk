# Design

## Context

This adapter is a new Gateway capability, not an extension of ProviderWire and not a Claude Code compatibility claim. The feature was developed from `main` at `5081cfb2` (#413) and rebased onto fetched GitHub `main` at `cc7b77e0` on 2026-10-22. The registered AI SDK comparison baseline is `@ai-sdk/anthropic` 4.0.71 and `@ai-sdk/provider` 4.0.21 (upstream commit `eb77f09e3c06c28e860d92e0de941b143c2eecec`, verified 2026-10-07). The inbound protocol itself is specified by Anthropic's Messages API; official Anthropic client pins are `anthropic-sdk-go` v1.80.0 and `@anthropic-ai/sdk` v0.133.0 for tests.

PR #248 is open, draft, and changes-requested. Its useful pattern is routing protocol adapters through the shared service auth/telemetry and model catalog; its OpenAI-specific schema closure, request budgets, defaults, and error extensions are not carried over. Issues #394 and #395 remain open; they guide shared ownership but do not block this independently useful endpoint. Reuse the current outbound Anthropic resource policy. Do not introduce endpoint-local size or count limits.

## Goals / Non-Goals

**Goals:** Serve the documented Messages text and client-tool subset over existing Gateway execution. Prove the contract with pinned official clients against a local command.

**Non-Goals:** Claude Code compatibility, token counting, server tools, BYOK, and complete Anthropic content-block parity.

## Decisions

The adapter accepts the model selector, `max_tokens`, optional system text, ordered user/assistant text history, function tools with JSON schemas, tool choice, stop sequences, and supported sampling fields. Assistant tool-use blocks map to provider tool calls; subsequent user `tool_result` blocks map to results correlated by `tool_use_id`. `disable_parallel_tool_use`, strict function schemas, and tool input examples use existing canonical provider option/type fields. Inputs whose requested semantics cannot be mapped (including non-text content, thinking/signature replay, cache controls, beta-specific capabilities, and server tools) receive a fixed unsupported-request error instead of silent loss. Ordinary `encoding/json` struct decoding is used; no closed schema, duplicate-member rejection, key normalization, custom parser, or recursive opaque-value walk is added.

The endpoint is registered only on the private listener and requires configured-account access. Cloud BYOK and operational listeners do not expose it. Authentication reuses the existing Gateway JWT/static-key authenticator. `Authorization: Bearer` is accepted by Anthropic SDKs and normalized into the existing auth context; `x-api-key` may be accepted as a credential alias only through a route-specific header adapter that passes the supplied token into the same verifier. It never becomes an upstream provider credential. Conflicting or duplicate credential sources fail authentication.

Request mapping uses ordinary `encoding/json` typed decoding. The adapter maps system text, ordered text messages, function schemas, client tool use/results, stop sequences, sampling fields, and supported native Anthropic provider options into `provider.CallOptions`. Unsupported content blocks are explicit errors. The adapter does not validate arbitrary unknown additive fields or normalize option namespace keys.

The HTTP adapter resolves models through `catalog.ModelResolver` and executes the selected `provider.LanguageModel`. It never executes client tools. Anthropic credentials are adapted only for the shared Gateway verifier and are not passed to model options. Authentication observation remains in shared middleware.

Unary output maps canonical text/tool calls to Anthropic message content and returns the catalog's canonical public model ID, not the backend model ID. Provider stop reason `tool-calls` maps to `tool_use`, `length` maps to `max_tokens`, and successful stop maps to `end_turn`. Token values come only from provider usage; unavailable counts become zero. Native input tokens exclude cache reads/writes. Tool input JSON retains its raw numeric values, and native input examples are wrapped into the canonical example type. The canonical provider contract does not carry the matched text for a stop sequence, so that response field stays null.

Streaming maps provider text/tool input parts incrementally to Anthropic SSE events and flushes writes. Errors after commitment emit an Anthropic error event and do not emit `message_stop` or retry. Stream-start/raw markers do not force an early message start before response metadata. The final message delta carries cumulative input/output/cache usage. Request cancellation cancels provider execution.

Public errors use fixed type and message strings and do not expose provider or routing data.

## Risks / Trade-offs

- [Canonical provider types do not represent all Anthropic blocks] → Return an explicit unsupported error for requested unsupported blocks and document the subset.
- [A stream can fail after the HTTP response is committed] → Emit an Anthropic error event and omit successful terminal events; never retry after commitment.
- [SDK client auth modes vary] → Test official bearer/API-key client configuration against the actual shared verifier path.

## Migration Plan

No existing route changes. Deploying the Gateway adds `POST /v1/messages`; rollback removes the route by reverting the Gateway image.

## Evidence and gaps

Tests use local provider servers. Dedicated protocol conformance cases replay unchanged existing recorded/imported Anthropic fixtures through both direct official SDK consumption and the Gateway, comparing the supported message projection. Separate synthetic response cases explicitly identify their synthetic provenance. A real Gateway command is exercised by official Go and TypeScript clients for unary, tool continuation, errors, streaming, auth and routing. This proves client wire consumption and mapping only; it is not evidence of live Anthropic acceptance or Claude Code compatibility. No count_tokens endpoint is included: the initial acceptance case is SDK inference, and the provider contract exposes no token-count operation.

The Go provider returns canonical text, tool calls, reasoning, signatures in metadata, stop reason, and token usage. Reasoning signatures can round-trip at provider level but inbound replay is explicitly outside this adapter subset. The canonical model contract has no generic count-token call and does not preserve every Anthropic-specific content block. These remain concrete boundaries for follow-up client compatibility work.
