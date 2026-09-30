## Context

The original design baseline was `ai@7.0.109` / `@ai-sdk/provider-utils@5.0.45` at [4e8c387622ee1bb0d55841664416d38754d5c9a3](https://github.com/vercel/ai/tree/4e8c387622ee1bb0d55841664416d38754d5c9a3). Relevant upstream paths at that exact commit:

- `packages/ai/src/tool-search/tool-search.ts:15-58`: marked function tool, fixed schemas/description and unbound-execution error.
- `packages/ai/src/tool-search/prepare-tool-search.ts:16-138` and its tests: generation-owned discovery, route validation, active snapshot, contextual descriptions and scoring.
- `packages/ai/src/tool-search/tool-search.test.ts:80-320`: direct/nested next-step activation and all four core/agent entry points.
- `packages/ai/src/generate-text/stream-text.ts:1393,2040-2130,2351-2364` and `generate-text.ts:580,920-935`: state creation, original-registry approval resume, then active filtering → search preparation → caller preparation.
- `packages/provider-utils/src/types/tool.ts:55-72,185-203`: `deferLoading` and context-dependent description functions.

Merging `origin/main` advances the registered baseline to `ai@7.0.116` / `@ai-sdk/provider-utils@5.0.49` at [ee3169b3c4880e2abe4d0d7c781243bb81822ec4](https://github.com/vercel/ai/tree/ee3169b3c4880e2abe4d0d7c781243bb81822ec4). Discovery source and tests remain behaviorally aligned; regenerating the paired core fixtures with these package versions produces byte-identical scenarios, requests and UI chunks.

Go currently has static `Tool.Description`, caller metadata and no discovery controls (`tool.go:111-203`). `streamtext.go:615-624` prepares callers and provider definitions directly, with a no-routes active filter applied only to provider definitions; `stepCfg.tools` controls parsing/execution (`674-690`). `tool_caller.go:55-121` separates execution/model sets, late binding and announcements. GenerateText collects StreamText (`generatetext.go:1-75`). #211's routing is already implemented by #290, so it is not an outstanding delivery prerequisite.

## Goals / Non-Goals

**Goals:** opt-in core registry search; no undiscovered definitions, schemas or newly generated execution access; next-step activation; active/route filtering; contextual descriptions; isolated concurrent generation state; existing tool lifecycle and frontend wire compatibility.

**Non-Goals:** provider-hosted search, sandbox/code-mode implementation, a new caller authorization/provenance engine, per-tool context schemas or sandbox sessions, semantic/vector search, configurable limits, persistence of discovery, changes to provider adapters or completion of #107. “Local” denotes core-owned search of the supplied registry: filtering is type-agnostic like upstream, not permission to execute provider-defined tools locally.

## Decisions

### 1. Add small opt-in root API, preserving tool value copies

Proposed symbols:

- `Tool.DeferLoading bool` (zero false).
- `ToolSearch() Tool`, returning a `UserToolFunction` with the exact baseline description, input/output schemas and ordinary raw-JSON executor contract. Its input is `{query:string}` with minLength 1, required query and no additional properties. Its output is `{tools:[{name:string,description?:string}]}` with no schemas or extra properties. Unbound execution returns an `aisdk:` error through existing execution error handling.
- Private search marker on `Tool`, preserved by ordinary struct assignment/copy even if a user changes its description or approval configuration. Only `ToolSearch()` creates this marker; detection does not depend on the registration name or matching descriptions.
- `ToolDescriptionOptions` containing `Context any`, `ToolDescriptionFunc func(ToolDescriptionOptions) string`, and optional `Tool.DescriptionFunc ToolDescriptionFunc`. A non-nil callback overrides the existing string, including an explicitly empty result; otherwise a nonempty static description is present and an empty static description is absent for search output. This adapts upstream's per-tool description context to existing Go `PrepareStepResult.Context` and agent `WithToolLoopAgentRuntimeContext` / `WithAgentRuntimeContext`, without inventing a context map or sandbox API.

A shared internal resolver retains description presence as well as text. Resolve eligible tool copies before caller catalog preparation so `PrepareModelMessage` sees contextual descriptions; resolve returned model definitions as needed near `convert.go:446`. Search closes over original candidate descriptions and resolves them at execution against that step's captured runtime context. Clear the callback only on resolved copies to avoid a second evaluation of the same prepared definition; never clear it on the registry. A caller-bound replacement can supply its own description callback. Description callbacks should be deterministic for their context and safe under concurrent requests; no cross-generation cache is introduced.

Replacing `Description string` with an interface would break existing consumers; static-only search would fail #261 acceptance. Adding options to every constructor is unnecessary: a `TypedTool` result is a `Tool` and can be configured before registration. `FingerprintTools` keeps its documented static-definition scope; do not claim it fingerprints callback outputs or use discovery as authorization.

### 2. Own discovery once per generation, not on a shared tool

Introduce internal search state in `tool_search.go`, initialized once in StreamText's generation before the step loop. Validate the original registry and routes before provider requests, even if the offending tool is inactive. For every deferred or marked search entry, each named caller must be a local `ToolCaller` with `PrepareModelMessage`; provider callers and local callers without an announcement callback are invalid. A marked search cannot itself defer. Missing routes imply direct access; explicit `ToolRoute{}` implies no allowed caller and is valid but matches nothing. Ordinary non-search/non-deferred caller routes are unchanged.

The state owns a mutex-protected discovered-name set. Each step captures eligibility into fresh maps/slices before tool execution. Searches union their matches into generation state without changing that step's tools, nested caller bindings or catalog. Do not hold the mutex across description callbacks. Shared ToolSet instances and ToolSearch values remain untouched; approved history does not reconstruct state. Merely marking tools deferred without registering a search keeps them unavailable for new step calls.

Alternatives rejected: mutating `cfg.tools`, storing state on the tool/agent, or dynamically widening a caller's live binding would leak requests or permit same-step execution.

### 3. Prepare effective active tools → discovery → callers → provider definitions

After `PrepareStep` determines effective active tools and runtime context:

1. When the registry contains deferred/search entries, filter it using `activeToolsSet` and the effective names. Omitted active selection means all configured tools; explicit empty selection means none, including the search. Unknown active names add nothing. Snapshot candidates from this active registry before removing undiscovered entries.
2. Remove all undiscovered deferred entries from the step execution/model input. Bind each marked search to candidates that are deferred, not marked searches, and share an allowed caller with the search. Direct counts as a shared caller; a named caller counts only when active. Candidate membership is evaluated before caller binding and remains fixed for the step; previously discovered candidates remain searchable.
3. Resolve descriptions for the eligible catalog copies; call `prepareToolsForCallers` with the prepared set and existing routes. Preserve local-only visibility, stable original caller model definitions and deduplicated user announcements. Direct search itself injects no catalog messages.
4. Convert model tools to sorted provider definitions and assign the effective execution set to `stepCfg.tools`. Thus parse/input callbacks, approvals and execution cannot fall back to the original registry for new step calls. An early static call emits the existing tool input/output error lifecycle, without invoking the deferred executor. Preserve the original registry separately for streamed-input and accepted/rejected-call UI static/dynamic classification: a known but undiscovered static tool remains a static-tool error on the wire, as upstream's `toUIMessageChunk` consults original tools. This registry is never an execution or validation fallback; unknown dynamic/provider-driven behavior is unchanged.

Newly discovered tools first appear in the next preparation, subject again to active selection and routing. Deactivation hides an already discovered tool; reactivation within the same generation restores eligibility without another search. Stop conditions still control whether a next step occurs; do not automatically increase the default one-step limit.

Keep the existing no-discovery preparation path unchanged, including its current no-routes active filtering behavior. This deliberately avoids widening #261 into ordinary active-tool execution hardening. The new opt-in path must not retain that provider-only filter loophole. Provider-defined entries use the same defer eligibility rules, but retain their existing type, arguments, warnings and provider-execution lifecycle: discovery is not conversion to a local function.

### 4. Preserve scoring, with explicit deterministic Go tie adaptation

Match the baseline tokenizer: split ASCII lowercase-letter/digit followed by ASCII uppercase-letter, lowercase, then collect Unicode letter/number runs. Deduplicate query terms. For each unique term, add two points for membership in name tokens and one for description tokens; repeated tokens do not multiply a term's score. Resolve each candidate's description with the captured step context, keep positive scores, sort descending, return at most five and record exactly those matches. Whitespace/punctuation-only nonempty queries return `{tools:[]}`; empty query is a schema error. Repeated searches can return previously discovered tools.

Upstream stable ties follow JavaScript object insertion order; Go ToolSet is a map. Iterate sorted tool names, preserving that order on score ties, consistent with Go's sorted provider definitions and caller preparation. This is an explicit deterministic Go adaptation, not byte-identical insertion-order parity. Add a tied-limit test and record this durable evidence boundary in PARITY.md when implemented, rather than weakening fixture comparisons. No fuzzy search or configurable ranking is added.

### 5. Use existing approval, cancellation and continuation rules

Bound search is an ordinary local tool. Pending/denied search approval does not run its executor or discover anything; automatic approval runs it and activates matches on the next step. Once eligible, deferred callees retain existing input validation, output conversion and approval rules. Local caller callbacks own nested invocation; orchestration does not introduce a second nested approval engine.

Keep historical approval resume before model-step binding (`streamtext.go:476,2029-2103`), as exact upstream `stream-text.ts:2040-2130` executes approvals against original tools. A resumed marked search remains unbound and produces the ordinary tool-output error without discovering. An already-approved historical deferred callee can execute via existing resume rules without seeding discovery; it stays unadvertised in new step definitions until searched again. The undiscovered-execution guarantee applies to newly generated calls/bindings, not approved historical execution. Preserve signature verification, policy revalidation, provider-executed handling and result ordering. No history-derived discovery, rebinding before resume or serialized state is added.

Use existing context cancellation and timeout mechanisms. A canceled generation does not continue steps or leak its discarded state into another request; no rollback of a search that already completed is promised. Race tests must cover sibling searches and independent generations. Approval-resume behavior is a baseline boundary, not a seamless cross-request search guarantee.

### 6. Prove core behavior independently of provider-hosted search

- Unit/lifecycle tests exercise direct and local-caller discovery, early calls, invalid routes, nil/empty active sets, context changes, scoring, state isolation, approvals/resume and cancellation. Explicitly cover StreamText, GenerateText, Agent.Stream and Agent.Generate rather than inferring agent proof from delegation.
- Add a deterministic multi-step mock model that captures provider `CallOptions` and asserts exact tools/prompts at every step (schema absent before discovery, present only later, stable search/caller definition, no unrelated schema leakage). Compare provider-neutral request projections with exact-baseline TypeScript MockLanguageModelV4 runs. Do not require a live adapter request to prove this core contract.
- Follow `test/conformance/ui_conformance_test.go` for provider-independent `ui/deferred-tool-discovery` UI chunk snapshots. Existing UI fixtures replay provider StreamParts; this feature needs a configured multi-step core driver with ToolSearch and deferred tools, not just a precomputed tool result. Add a focused core mock driver/test for the fixture; retain provenance linking the corresponding pinned upstream mock scenario and captured expectations. UI evidence alone does not prove registry filtering; pair it with request capture. Do not fabricate `recorded/` or `upstream/` provider inputs.
- Add `test/integration/testserver/scenario_deferred_tool_discovery.go` and a matching Vitest scenario. Parse SSE with `parseJsonEventStream` / `uiMessageChunkSchema`, assert search output, early rejection, subsequent tool output, step order and assembled UI messages using the pinned frontend. No new chunk types or framing are introduced.

Core orchestration/tools and UI/SSE are currently `mixed` in PARITY.md. Mock requests/chunks establish core behavior and frontend assembly, not live provider acceptance. Existing provider-hosted tool-search fixtures are not this proof. Record stable support/proof boundaries only when implementation lands; do not add dated issue catalogs or change baseline pins.

## Risks / Trade-offs

- [Shared-state races or same-step visibility] → Generation-owned locked union and immutable eligibility snapshots; race tests check parallel searches/requests and early nested calls.
- [Resolving descriptions too early, twice, or against stale context] → One shared resolver, original candidates captured for execution, resolved catalog copies and tests for per-step context overrides, empty callback output and stable caller descriptions.
- [Approval resume is surprising] → Document/test the exact upstream boundary; users needing discovery after resume must run a newly bound search in the new generation.
- [Go map tie ordering differs from upstream] → Sorted-name ties, explicit classification and snapshots; do not imply insertion-order parity.
- [Active filtering accidentally changes ordinary tools] → Preserve default path and add compatibility tests without discovery controls; isolate the stricter execution snapshot to the opt-in path.
- [Synthetic fixture overstated as provider parity] → Provenance notes and separate core-request, UI and real-provider evidence; no invented provider payloads.

## Migration Plan

Deliver one independently green root-module feature with tests, fixture provenance and docs against the unchanged baseline. No provider-module publication prerequisite exists: #211 APIs are already on main and provider contracts do not change. Publish the root module through normal release policy after merge; applications opt in by registering `ToolSearch()` and marking deferred Tool entries. No automatic migration or increased step count. Rollback is removal of those opt-ins; ordinary tools/provider-hosted search remain usable. This proposal adds no source implementation or baseline metadata changes.

## Open Questions

No unresolved product/scope prerequisites. Proposed exported names/signatures and the explicit Go tie/context adaptations require normal design review before implementation. Live provider acceptance remains an acknowledged proof gap, not a blocker to provider-independent core evidence or permission to synthesize recordings.
