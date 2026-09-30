## Why

[#261](https://github.com/grafana/ai-sdk/issues/261) is still valid: Go has no core `ToolSearch`, deferred-loading flag, or generation-local discovery state, although registered upstream `ai@7.0.109` implements them. Large local tool registries therefore cannot opt into keyword discovery and next-step activation without application-owned orchestration.

## What Changes

- Add an opt-in `ToolSearch()` function tool and `Tool.DeferLoading`; search returns up to five ranked names/descriptions, never schemas, and activates matches only on the following model step.
- Keep discovery state local to each StreamText generation, including GenerateText and ToolLoopAgent entry points, with immutable step bindings and concurrent-search safety.
- Prepare discovery after effective active-tool filtering and before caller binding; reject unsupported search/deferred routes and prevent newly generated early calls from executing undiscovered tools.
- Add optional context-dependent description resolution while preserving `Tool.Description` and existing runtime context. Reuse one resolution contract for search, model definitions and local caller catalogs.
- Reuse the already implemented #211 `ToolRoutes`/`ToolCaller` API; preserve approvals, cancellation, historical approval-resume behavior and provider-executed/dynamic lifecycles.
- Establish exact-baseline unit, provider-independent UI/request and frontend evidence; document deterministic Go tie ordering and evidence boundaries.

## Capabilities

### New Capabilities

- `deferred-tool-discovery`: Opt-in core search, scoring, context-dependent descriptions, per-generation state and next-step activation, including approval/cancellation boundaries.

### Modified Capabilities

- `caller-aware-tool-routing`: Apply discovery eligibility before preparing caller execution bindings and catalogs; keep existing direct/local/provider routing behavior for ordinary tools.

## Impact

- Root orchestration: `tool.go`, a new `tool_search.go`, `streamtext.go`, `tool_caller.go`, and description preparation near `convert.go:446`. GenerateText collects StreamText; agents reuse that path but require explicit tests.
- New exported root API is additive for keyed tool literals; no provider interface, UI chunk discriminator or SSE framing changes. Ordinary tools without discovery controls retain their current path.
- #211 was delivered by #290 (`9571d905`); no red prerequisite stack or new caller/sandbox API is needed. Provider-defined/hosted search and #107 remain separate, not substitutes or blockers for this core feature.
- Reference is `test/conformance/upstream.yaml`: commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`, `ai@7.0.109`, `@ai-sdk/provider-utils@5.0.45`, `@ai-sdk/provider@4.0.17`, `@ai-sdk/react@4.0.112`. No baseline upgrade, dependency additions or live-provider parity claim is proposed.
- Tests cover the mixed core/UI layers in `test/conformance/PARITY.md`; implementation will add focused request capture, provider-independent UI fixtures and schema-parsed frontend integration. Narrative guidance belongs in `docs/guides/tools.md`; API details belong in godoc.
