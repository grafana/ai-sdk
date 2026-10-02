## Context

Direct provider calls, core streaming, and the standalone Gateway client currently expose different subsets of provider-tool behavior. Incompatible tool fields can reach a backend, marker defaults can erase an explicit input-start value, and the client cannot safely read provider-owned results. At the same time, the service must not accept new request families merely because its SDK dependency can decode them.

Behavior follows the registered reference `4e8c387622ee1bb0d55841664416d38754d5c9a3` (provider 4.0.17, ai 7.0.109, Anthropic 4.0.59, Gateway 4.0.88), particularly the provider tool/result types, stream-language-model-call and UI chunk conversion, Anthropic request options, and Gateway client serialization.

## Goals / Non-Goals

**Goals:** Give direct callers and the Go Gateway client a validated, bounded provider-tool contract; retain meaningful marker and MCP option presence; keep unary Anthropic requests with caller deadlines compatible with their model token budgets.

**Non-goals:** Enable provider tools or MCP on the Gateway service, change service fallback or egress policy, persist correlation state, support unrelated tool families, or alter authentic provider fixtures.

## Decisions

- Validate the existing flat Go `Tool` at each direct entry point rather than imposing the HTTP schema on Go callers. This catches incompatible populated fields before I/O. Nil Go provider args become `{}` at conversion, matching upstream defaults; HTTP callers still must supply an object.
- Retain the parent's presence-aware unary and streaming marker fields. Absent and false preliminary markers both mean final; input-start explicitly false overrides definition-based text-stream inference. UI conversion independently classifies known application tools as the registered frontend does; tests cover both outputs rather than assuming they agree.
- Extend the client's closed unary/SSE output union with provider calls/results while reusing its independent opaque metadata decoder rather than sharing server DTOs or introducing metadata inventories. Preserve byte/event bounds, cancellation, client-owned response fields, and valid warning order. Do not require a current-response call for deferred results or add an independent server lifecycle validator.
- Use pointers for Anthropic MCP token/enabled options so omission differs from empty/false. For unary Anthropic calls, supply a future context deadline as the default SDK timeout before its large-token check; an explicit request timeout still wins. Neither streaming nor the no-deadline SDK guard changes.
- Keep Apache SDK modules independent of Gateway code. The Gateway service consumes published immutable module versions, migrates its existing field uses, and retains rejection of capabilities it does not yet validate or route.

## Risks / Trade-offs

- Changed Go field types can break consumers → migrate compiled callers and test isolated modules without workspace replacements.
- A client can decode responses the service does not yet emit → keep service rejection tests and describe SDK decoding separately from service support.
- Accepting new tool variants prematurely could enable effects → reject unsupported service inputs before invocation.
- Synthetic transport tests cannot prove provider behavior → retain fixture provenance and report live-provider gaps separately.

## Migration Plan

Validate compiled consumers against candidate source while retaining merged-main module pins. Leave existing native option forwarding, reusable fallback and opaque metadata transport unchanged; client decoding alone does not activate service-side provider tools or MCP. Verify direct provider calls, both stream projections, client decoding, isolated module resolution, and Gateway rejection. Roll back the corresponding module set if needed; no persistent state is introduced.

## Open Questions

None for SDK contract support. Remote MCP egress and service activation require separate service-side decisions.
