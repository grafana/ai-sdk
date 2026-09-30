## Context

Issue #303 revises delivered Gateway contracts for customer-owned provider relationships without implementing BYOK provisioning. The command still uses startup-configured process-wide credentials. Useful authorized caller responses are not telemetry or an operator configuration dump; neither customer ownership nor internal-team use relaxes credential, tenant, execution or resource protections.

The registered reference remains commit `4e8c387622ee1bb0d55841664416d38754d5c9a3`: ai 7.0.109, Gateway 4.0.88, Anthropic 4.0.59, provider 4.0.17. Matching Git objects, not the older shared working tree, supply source/tests. `gateway-language-model.ts:69-107` preserves server unary warnings before client warnings but replaces typed request/response with Gateway transport. Its streaming transform (:145-185) forwards ordinary parts, converts timestamps and filters only raw parts. `create-gateway-error.ts`, `as-gateway-error.ts` and `gateway-error.ts` determine observable error fields/classes and HTTP retryability. #299 references a newer target; no upgrade or core termination change is approved here.

Current code: `response.go:20-61` drops unary warnings/identity; `stream.go:232-266,617-641` replaces warning prose/model identity; `sources.go:67-143` replaces source identity/display and projects numeric citation metadata. The command's `provider_options.go` omits Anthropic caller despite `providers/anthropic/convert_request.go:1072-1073,3005-3035` consuming it in ordinary assistant function history. `catalog.ResolvedModel` has no physical-adapter provenance, logical wrapping reports grafana, and fallback joins failures newest-first. These are evidence, not reasons to preserve every current filter or prescribe a new error framework.

## Goals / Non-Goals

**Goals:** define the revised caller-response contract and restriction dispositions; deliver independently useful non-metadata policy/runtime prerequisites; unblock #280 with request-policy and contract clarity; preserve exact-pinned client behavior, independently implemented Apache/AGPL ownership and independently green delivery.

**Non-Goals:** BYOK credential provisioning/storage/selection, native API adapters, provider-tool/MCP activation, effectful fallback, raw transport passthrough, public topology, new core error types/termination, baseline upgrades, automatic client replay or exactly-once generation.

## Decisions

### 1. #303 is a foundation, not a consumer of its successor

Nara's agreed order is **#303 → #280 → #238 → #239 → #240**. #303 must merge and complete its foundation without #280 response transport, #201's undelivered harness, or future tool helpers. Its implementation tasks are limited to:

- Revised policy/inventory and the normative changes for behavior actually delivered in this foundation.
- Unary/stream warning values, registered response identity, and source ID/display preservation without changing source-metadata transport.
- Anthropic caller request forwarding on already-supported assistant function-tool history, demonstrated from caller-supplied/built history through both clients into native fake-provider requests.
- Concrete request-policy corrections/refusal boundaries and focused security/lifecycle/telemetry tests.
- Minimal actionable provider errors through existing handler/error paths, with no new stream-lifecycle layer or public error API redesign. Nara explicitly approved this ownership boundary; invasive representation/transport work requires a separately registered Gateway follow-up coordinated with #299.

#280 alone owns all response metadata server/client/schema transport, unknown metadata and presence/budgets, metadata-dependent source integration, and continuation assembled from actual response output. #238 owns SDK/provider-tool/client readiness; #239 owns ordinary provider-tool runtime; #240 owns authorized hosted MCP. #201 owns the later authentic two-client matrix. `handoffs.md` maps the original issue's acceptance across these deliveries instead of silently deleting it.

Foundation completion is not proof of all original end-to-end #303 acceptance. Later response-metadata acceptance stays explicitly open in its owning work. Sync/archive only implemented foundation requirements; future transport requirements belong in the successor's deltas. This plan's active delta specs do not require #280 codecs as a foundation gate.

### 2. Audit restrictions by effect, not by their current existence

`restriction_inventory.md` gives concrete keep/remove/defer dispositions for current protected headers/fields, provider allowlist fields, namespace selection, option loss and fallback rules. Classification separates protocol correctness, capability support, credential/tenant authorization, telemetry privacy and obsolete shared-account concealment. A keep decision needs a present reason; supported consumed options are not silently removable merely because a typed struct/allowlist omitted them.

Keep credential overrides, reserved host controls, role/union/transport overrides and unsupported execution refusals. Remove caller-response concealment where represented and implemented. Remove the missing Anthropic caller restriction now. Retain irrelevant namespace omission only when every selected provider ignores it; heterogeneous zero/intersection filtering is not authorization to silently discard options a candidate consumes. Reject active consumed-but-unrouteable options before invocation rather than report a silently altered successful request. Scalar/continuation fallback support needs its own candidate-routing proof; it is deferred capability work, not inherently an effect or shared-account secret. Compatible unknown fields need endpoint-specific semantics/security review, not blanket permissive approval or secret-shaped-string censorship.

### 3. Narrow caller request prerequisite, independent of response metadata

The pinned Anthropic converter's `getAnthropicCaller` (:1460-1480) consumes `{type:"direct"}` and `{type:"code_execution_20250825"|"code_execution_20260120",toolId:nonempty}`; ordinary `tool_use` history (:902-910) carries that caller even when the SDK part is not provider-executed. The Go converter and focused native test `TestConvertAssistantContent_ProviderExecuted/non-provider-executed preserves caller metadata` implement the same consumed behavior. The Gateway currently removes the top-level `caller` option before that conversion.

#303 restores it at the existing assistant function-call part scope on direct Anthropic routes. Forward the reviewed provider option without adding a competing exact-key/direct-only validator from #293. Native conversion owns consumed caller variants, required toolId and ignored malformed/unknown forms; tests compare the registered consumed semantics and disclose any Go-representability difference. Nested caller.type is not a top-level role/type bypass, and caller history is not permission to execute a server tool.

Use supplied/built history, not the first Gateway response, in both clients' unary and streaming requests. Capture native Anthropic `tool_use.caller`, including tool_id conversion and missing/unknown/empty controls. No dependence on new response metadata, provider tool definitions, server execution or MCP. Keep providerExecuted/role/union/credential/MCP/effectful-fallback refusals and original option immutability. This proves request policy only. #280 subsequently obtains caller from actual unary/stream output and proves the next native request; manual-history evidence cannot establish that roundtrip.

### 4. Warning, source and response identity foundation

Preserve the registered warning union/order: unsupported/compatibility feature with optional details, deprecated setting/message, other message. Required empty strings survive; inactive fields do not. Document optional-empty Go details normalization. Unary emits a non-null warning array; streaming retains exactly one initial start and no finish warnings. Each warning string is at most 4096 UTF-8 bytes; cardinality/aggregate preflight and complete escaped unary/event bounds precede commitment. Invalid stream warnings yield the existing empty-start/fixed-terminal-safe path.

Preserve native source IDs and display values, including file_path title/filename, without source-N/hashing/Document substitution. Keep nonempty valid-UTF-8 IDs up to 1024 bytes, source unions, required empty document title, optional-empty normalization, Title-before-legacy-Text, order, no URL fetching, output-start/finish/fallback commitment and whole-document bounds. This is independently deliverable without metadata changes. The current numeric citation projection remains a temporary response-capability boundary until #280 replaces it; it is not the desired contract or a shared-account security justification. #303 must not broaden/decode/filter/reimplement source metadata. Full native metadata/source acceptance belongs to #280.

Keep logical requested/canonical routes and public discovery separate from actual provider response identity. Streaming response-metadata retains supplied id/modelId/timestamp; absent actual model is not fabricated from the canonical route. Unary emits registered response id/modelId/timestamp without native headers/body. Both pinned clients replace typed unary response with local Gateway transport, leaving native identity available only in bounded raw response.body; document this limitation instead of inventing another field/dialect. No configured provider instance, credential, endpoint or fallback topology is synthesized into public output. Telemetry remains canonical/low-cardinality and excludes actual response fields.

### 5. Provider diagnostics: observable contract first

The desired public distinction is actionable provider failure versus fixed-safe Gateway-internal/host/transport/adaptation failure. The registered envelope is message/type/code/param; TS directly exposes message/status/category and inspects code/param through cause/body, not typed public code/details. Supported provider diagnostics should preserve reviewed message, native status/code/type and bounded reviewed detail without arbitrary Error(), URL/header/body/cause serialization. Native credential failures must not become Gateway-key authentication guidance. Credential-bearing auth prose/parameters require safe actionable substitution; application content is not heuristically censored.

Define byte/UTF-8/JSON and complete-response/event bounds before parsing/encoding: 16384 source/error bytes, 4096 message/detail bytes and 256 type/string-code bytes, subject to containing server bounds. Client limits are independently configured; the server cannot promise a particular caller's limit. Preserve the existing string-valued GatewayError.Code API and bounded underlying APICallError envelope. Use existing registered param for reviewed native type/code/parameter details (including numeric/null native codes), accessible through both clients' cause/body rather than inventing typed properties. Keep the public top-level code as the Gateway's existing category code. This is a documented minimal diagnostic surface, not full native top-level-code equivalence; richer public error APIs require separate approval. Dynamic fields must not make permissive TS parsing the strict server oracle.

HTTP retry matches the pinned status rule (408/409/429/5xx); non-2xx provider retry overrides cannot be promised through a new flag or status fiction. Provider SSE error parts remain ordered/non-terminal at that boundary, preserving existing statusCode/retryable where reviewed, not automatically becoming HTTP GatewayError instances. Core termination remains unchanged. Retryable paid-generation adaptation failures may duplicate cost; preserve/report call counts and no exactly-once guarantee. No automatic client/fallback replay after commitment.

Nara approved minimal errors within the existing lifecycle/API. The direct-route policy seam below is the preferred foundation choice because it needs no error rewriting or extra stream reader; implementation must confirm it against actual composition. Alternatives remain subject to that same limit:

| Seam | Benefit | Cost / required proof |
|---|---|---|
| Server-owned optional diagnostic policy attached during trusted direct-route construction and passed with resolved route into existing unary/stream projection; zero/unreviewed/fallback routes fixed-safe | No new stream reader, core error type or public error API; catalog policy is AGPL-only and not discovery | Select by configured adapter, never logical grafana identity/body claims. Because only a single direct candidate can execute, the same route's policy binds its failure. Generic/fallback routes have no authority for native detail. Existing PartError processing projects in place without rewriting input channels/errors. |
| Projection at an existing trusted candidate/composition boundary with existing transport/error evidence | Can cover selected physical failures without inventing public topology | Must preserve errors.As/Is/native retry/context-window and source pairing; inspect actual lifecycle before adding wrappers. |
| Producer/core representation enrichment coordinated with #299, followed by Gateway adoption | Can support typed/native stream diagnostics absent today | Separate owner/target/publication decision; cannot silently adopt #299's newer reference or make #303 depend on it. |

Deliver reviewed structured HTTP/direct-stream diagnostics available now for configured Anthropic/OpenAI/compatible direct routes; preserve native non-2xx status and pinned retry/category mappings while keeping host/internal errors fixed. Source-specific extractors read only reviewed Data/ResponseBody fields: OpenAI/compatible message/type/code/param and Anthropic error type/message. Auth failures use fixed provider-account authorization prose with credential-bearing native prose/param excluded. Unsupported message-only events, unknown compatible schemas, unconfigured resolvers and ambiguous fallback aggregates retain safe diagnostics and explicit gaps. Existing stream projection handles available direct PartError evidence without a new producer/error/channel representation.

Do not durably prescribe sentinels, two-child errors.Join trees, structural error parsing, cloned APICallErrors, another forwarding goroutine or a breaking GatewayError.Code type. Do not mutate shared errors or combine a policy from one joined branch with data from another. If richer stream/aggregate diagnostics require new lifetime/API/error machinery, stop that expansion and register the concrete Gateway follow-up in handoffs.md; it is not part of #303 foundation or automatically #299-owned.

### 6. Debugging, security and delivery proof

Support client-owned Gateway request body and bounded Gateway HTTP response headers/body, actual registered response identity with unary caveat, and reviewed diagnostics only where delivered. Native request/response/debug transport can contain credentials, signed URLs, endpoint/private configuration and SDK dumps and is overwritten by the pinned unary client anyway. Do not tunnel it through metadata or add a debug endpoint. Known credential sources/owned transport are excluded; arbitrary application strings are not scanned for token patterns.

Test caller output separately from logs, metric labels and metadata-only Agent Observability, including internal-team contexts. No content, warning prose, sources, metadata, actual identity or actionable error prose enters telemetry. Scoped fixture contexts establish non-disclosure boundaries, not deployed BYOK credential storage.

Each foundation behavior PR updates its actual normative specs/DTO/schema/client/docs/tests together, independently green against this reference. Apache client never imports AGPL DTOs/implementation. Candidate-source Gateway image uses go.gateway.work; public producer changes require releases before consumer pins and GOWORK=off readonly evidence. Do not introduce a public API break without its own reviewed migration need.

Use current registered `test-providerwire-v4`, `test-ai-gateway-command`, `test-ai-gateway-source-integration`, direct conformance/parity, affected modules and candidate image checks. #201 is an open draft; no full-matrix harness/tasks exist on this main worktree. #303 can complete on current focused proof, with authentic full replay handed to #201 rather than recreated, guessed or weakened. Synthetic failures/native requests remain focused tests, not recorded/upstream provider evidence. UI behavior changes require deterministic schema-parsed integration evidence. Published adoption, private-service parity and live provider acceptance are distinct claims.

## Risks / Trade-offs

- Foundation completion is narrower than original end-to-end acceptance; handoffs must stay explicit and unresolved acceptance must not disappear.
- Source metadata remains a temporary supported-boundary gap until #280; no new metadata implementation belongs to #303.
- Manual caller history proves the request path only; actual response-derived continuation requires #280.
- Minimal direct-route diagnostics intentionally leave ambiguous aggregate/message-only/richer-code API gaps; the separately registered Gateway follow-up must justify any producer/transport/API complexity.
- Typed unary identity and HTTP retry overrides are pinned-client limitations.
- #201 full replay, public-module adoption and BYOK lifecycle are not established by current focused tests.

## Migration Plan

Land #303 foundation, then #280, then rebase/readiness/runtime/MCP work #238 → #239 → #240. Split/sync/archive only delivered requirements in each owner; future target contracts remain handoffs, not an omnibus unshipped normative upgrade. Release changed public producers before consumer adoption; deploy same-revision Gateway image separately. Existing recorded inputs/direct expectations and historical milestone criteria remain unchanged. No credential migration is included.

## Open Questions

The sequencing and error ownership decisions are resolved by nara: #303 completes its independently green foundation first and owns minimal actionable provider errors without a new stream-lifecycle layer/public error API redesign. Full original acceptance is not claimed from foundation evidence. Required invasive diagnostic representation/transport work must be separately registered as a Gateway follow-up coordinated with #299. The registration candidate and exact proof boundaries are in handoffs.md; no GitHub mutation is authorized in this PLAN. Implementation must confirm source/route coverage and name any additional capability gap, not expand the approved foundation to hide it.
