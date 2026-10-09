## Context

Use main including #326/#328/#332/#238 and the registered ai 7.0.118 / Gateway 4.0.96 baseline (5d12eaa6). Public Chat bytes use official OpenAI Go v3.66.0 and JS 6.27.0; provider request semantics use the pinned Vercel sources/tests inspected in the initial research. Latest evidence/BYOK/MCP stacks are separate capabilities, not implicit adapter dependencies.

## Goals / Non-Goals

Goals: remove temporary model/fallback restrictions; preserve native defaults, identity and warnings with bounded processing and official SDK evidence.
Non-goals: BYOK, hosted tools, new routing, whole-API OpenAI parity or an execution-evidence collector.

## Decisions

- Replace host RequestPolicy with a candidate-local defaults wrapper, activated only by a private Chat request context. The host supplies backend kind and compatible namespace before identity wrappers. Each invocation copies native options and translates Chat defaults only for that candidate; this also handles a compatible provider named openai without colliding with Responses. No intersection, model allowlist, mutation of shared options or candidate inspection in the codec.
- Compile a private explicit request schema once, then decode typed DTOs and shallow unions. Standard encoding/json duplicate-last and Unicode replacement semantics replace reflective exactFields/uniqueJSON. Keep existing bounded user-schema subset and lifecycle/resource protection.
- Expose a Grafana-owned optional `grafana` object on unary responses and SSE chunks. Preserve warning type/fields/order in `warnings`; capture response identity learned after the first emitted chunk as `native_response`. Only represented diagnostics are exposed, not raw request/response bodies or headers. Budget and UTF-8 checks fail rather than truncate.
- Delay the role chunk until first representable content/finish. Accumulate native identity beforehand, synthesize missing fields once, then freeze id/model/created. Preserve late native identity in the extension. No extra stream consumer, buffering of complete output or change to first-provider-part fallback commitment.
- Warnings do not turn successful generation into 502. Representation/lifecycle errors remain failures. Function and explicitly strict structured results retain output validation.

## Risks / Trade-offs

SDK helper accumulation may discard extensions: prove access through ordinary typed responses and streamed chunks, document that helper behavior separately. Missing native identity needs a generated ID/time and a documented route-model fallback; the extension does not claim unavailable attribution. Native unsupported-setting warnings may accompany changed provider behavior and remain caller-visible. Synthetic HTTP tests prove mapping, not live acceptance.
