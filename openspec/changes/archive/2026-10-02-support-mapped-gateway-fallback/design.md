## Context

Issue #319 and work package 32 of `/home/nara/ai-sdk-ai-gateway-plan.md` replace only the fallback-eligibility portion of stopped #303. This is a fresh plan against the current service, not a continuation of #303/#309. The existing `nrbrd/gw-fallback` branch already contains the authorized #318/PR #326 prerequisite through `02c59bc2` (ancestry verified); implementation and review use this stack without branch restructuring. Fresh-from-main intent forbids stopped-code extraction, not the explicit prerequisite stack.

The current service constructs each configured native candidate once in `ai-gateway/cmd/grafana-ai-gateway/internal/service/catalog.go`. Consumption-backed `nativeOptionsModel` protections wrap individual adapters; configured chains use `fallback.New` and the existing private attempt observer. An additional `fallbackTextModel` in `fallback_route.go` rejects otherwise supported tools/history/choices, file/reasoning parts, call/part options and headers, accepting message options only when namespace objects are empty. The factory then applies one canonical logical middleware chain outside this composition. No remaining catalog option inventory/intersection was found in this checkout after #318; implementation must recheck rather than introduce replacement policy plumbing.

The strict mapper in `ai-gateway/providerwire/v4/request.go` already maps function tools and supported results, ordinary file arms and filename presence, reasoning text/files, reasoning controls, ordinary headers and opaque scoped options. Unsupported codecs and reserved/protected controls fail through their owning boundaries. Removing the service guard does not expand those codecs.

### Reference and evidence boundary

The registered baseline is `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`: `ai` 7.0.116, Gateway 4.0.94, Provider 4.0.18, Anthropic 4.0.65, OpenAI 4.0.78 and compatible 3.0.57. Source was resolved from that commit in `/home/nara/src/ai`, whose checkout HEAD differs; no HEAD behavior was substituted. Relevant pinned references:

- `packages/gateway/src/gateway-language-model.ts` and its tests: complete call-option projection, native option pass-through, file/reasoning-file/result-file encoding and permissive unary/stream consumption.
- `packages/provider/src/language-model/v4/language-model-v4-call-options.ts`: request union and supported scalar/options scopes.
- `packages/anthropic/src/convert-to-anthropic-prompt.ts`: native scoped Anthropic consumption.
- `packages/openai/src/responses/openai-responses-language-model.ts`: native OpenAI/Azure namespace precedence.
- `packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts`: native namespace merging/extensions, tool conversion, reasoning and call headers.

The registered public client is a projection/consumption oracle, not a private Vercel fallback-service oracle. Configured ordering and first-provider-part commitment are governed by the existing reusable Go fallback contract (`fallback/fallback.go`, `openspec/specs/fallback-stream-error/spec.md`) and its regression tests. This is a Go extension with explicit semantics, not a claim that Vercel's private selection algorithm is reproduced.

`test/conformance/PARITY.md` classifies the work as Gateway runtime/host composition and provider request-boundary evidence. Current direct native-option evidence explicitly does not establish expanded fallback eligibility. Focused synthetic transports and both-client command tests close that selected boundary; they do not establish live provider acceptance or full output-derived continuation.

## Goals / Non-Goals

**Goals:**

- Every capability already mapped for the invocation mode remains eligible on configured fallback routes without an extra text/emptiness dialect.
- Each attempted adapter receives the same request semantics and reads its native namespaces, with unchanged configuration order, failure eligibility and cancellation.
- Preserve existing commitment and cleanup ownership; local tools execute only in consumer applications.
- Prove both clients, native consumption, presence, isolation and one logical observation per request with deterministic regression evidence.

**Non-Goals:**

- New request routing, BYOK, candidate discovery, public attempts/failures or native transport diagnostics.
- Missing provider-executed/provider-tool/approval/raw/custom/generated/structured codecs, or MCP/tool activation.
- Response metadata transport or actual output-derived native continuation (#280); supplied history remains request-path evidence.
- Changing fallback deciders, provider retries, start/error commitment, buffering, read-ahead, channel ownership or native adapter semantics.
- Exactly-once provider generation/effects or universal live acceptance by heterogeneous backends.

## Decisions

### 1. Delete the extra route dialect; do not replace it

Set the configured lower model to the reusable ordered fallback directly and remove `fallbackTextModel`, `fallbackTextRequest` and their semantic-emptiness helper. Keep per-adapter consumption-backed protections delivered by #318, the private attempt observer, canonical identity and the logical factory placement unchanged. Audit for candidate-policy intersections on the implementation base and remove only genuine extra eligibility/filtering machinery.

Alternatives rejected: broadening an allowlist leaves a second Gateway dialect; forwarding an intersection loses native semantics; refusing every active option replaces silent loss with blanket refusal; silently choosing only the primary changes configured execution. Existing strict mapping and concrete native bypass protections remain the correct owners.

### 2. Treat native interpretation as candidate-specific, not route-global

Forward supported `provider.CallOptions` unchanged in semantic value and scope, including meaningful empty/nested JSON, file-arm selection and optional filename presence. No cross-provider translation, option promotion, candidate-aware filtering or shared mutable request cache is added. The native adapter may consume, ignore, warn or reject according to its existing supported surface and protections; a mapped request does not assert every backend accepts it. Such failures use the existing decider, not a new capability-selection algorithm.

Tests distinguish model-boundary retention from actual HTTP consumption. A heterogeneous Anthropic/OpenAI/compatible chain carries multiple ordinary namespaces: captured native bodies prove each attempted adapter consumes its own namespace/precedence and does not consume another adapter's options. Include compatible named/camel namespace behavior and Anthropic/OpenAI request controls where already supported.

Alternative rejected: precomputing a route-wide option/capability intersection would both invent policy and bypass native behavior. Defensive request cloning is not added by default; test existing adapters for immutability and concurrent isolation. If a reproducible native mutation requires an unrelated producer fix, stop and scope that fix rather than transplant stopped code or silently broaden this change.

### 3. Keep the existing commitment and ownership boundary

Unary successful results select the candidate. Streaming setup errors, invalid nil results/channels and empty pre-part EOF remain pre-commit failures; a first received provider part irrevocably selects the candidate, including stream-start, error, response metadata, reasoning, source and tool input/call/result. A post-selection codec failure, missing finish or later error cannot re-enter fallback. Do not peek past stream-start, suppress errors to seek success or replay a committed stream.

Keep native retry classification and the reusable default/custom decider behavior unchanged. Preserve cancellation before setup, during first-part wait, at decision races and after selection. Preserve result-plus-error and late-setup cleanup behavior, including the existing single handoff/relay owner, cancellation-aware sends/receives and bounded asynchronous drains. The current reusable cleanup constants (100 ms and 256 parts) are regression facts, not new Gateway knobs.

Consumer applications run local functions after receiving selected calls and supply continuation in a new independent request. Both-client deterministic loops count each expected local execution once and verify native continuation requests. Each new request starts at the configured primary; a previous winner is not sticky. No Gateway executor or workflow state is introduced. Observable returned calls/parts establish commitment; hidden native work before a setup failure cannot be ruled out, so no exactly-once generation/effect guarantee is made.

Alternative rejected: tool-specific replay/idempotency or a second stream executor would change reusable semantics and is outside #319.

### 4. Keep observers orthogonal to execution

Retain one operator logical chain outside all candidates, one immutable request observation and existing private physical attempt correlation. Metadata-only capture controls what the operator records, not what requests are admitted or what supported responses reach consumers. Test mapped tools/files/reasoning under existing observers, disabled capture and concurrent shared-model calls. Do not broaden telemetry capture or add public attempt transport.

Alternative rejected: independently wrapping candidates with logical middleware duplicates generations and mixes operator policy with capability admission.

### 5. Make evidence layered and regression-first

Add failing focused cases before removing the guard. Use existing registered-client request generation and independent Go-client request paths; add compact witnesses only where existing ones do not cover the new route behavior. Cover both clients × direct/configured-fallback × unary/streaming for representative functions/history/choices, supported file arms/presence, reasoning and headers/options. Model-recording tests establish all mapped arms/scopes; fake native requests use appropriate existing backend support rather than demand every file arm succeed on every provider.

Extend existing authenticated command helpers/fake native servers rather than create another protocol harness. Exercise primary success, eligible failover in a three-candidate order, exhaustion, noneligible/cancelled failures and local loops. Retain synthetic model tests for empty pre-part and malformed/setup lifecycle cases that cannot truthfully be manufactured as native provider recordings. Native adapters commonly emit stream-start early, so eligible native setup failure and model-channel empty pre-part EOF are separate witnesses.

Add selected tool-call and stream-start/error commitment cases with a secondary that fails the test if invoked, including post-selection unsupported output/encoding failure. Re-run existing invalid/result-plus-error/late setup, blocked consumer, decider race, drain/part-budget and observer panic/saturation suites without weakening them. Use focused race tests for request-scoped maps/raw JSON, reusable candidate state and logical correlation.

No invented `recorded/` or `upstream/` inputs. No baseline bump is needed. If frontend wire behavior changes unexpectedly, stop and reassess scope; the intended change uses existing chunks/codecs. Existing schema-parsed tool/reasoning/frontend integration checks remain regression gates, with a new schema-parsed frontend scenario required if implementation changes UI/SSE behavior.

## Risks / Trade-offs

- [Stacked parent changes before merge] → Implement/review on the existing #318 stack, reconfirm parent changes/baseline and validate cumulative source. While #326 is open, the child PR targets `nrbrd/opt-forwarding`; it merges after the parent and rebases/retargets to main after the parent merges. No implementation wait or branch restructuring is required, and the parent is not claimed as merged. Do not resume #303/#309 or duplicate #318.
- [Backend rejects a mapped capability] → Keep native interpretation/eligibility, document backend limits and separate admission proof from live acceptance. Do not invent route intersections.
- [Unobservable provider work before setup failure] → Preserve the existing boundary and document duplicate paid-generation/effect risk; local execution counts do not imply provider exactly-once behavior.
- [Legacy rejection assertions/docs become stale] → Replace only the retired tool/file/option fallback refusals; retain strict malformed/deferred/bypass tests at the actual owning boundary. Update all affected guides/specs together.
- [Observer policy accidentally rejects or changes content] → Assert one generation per request, returned payload equivalence and separate metadata-only capture, including concurrent calls.
- [Response metadata gaps remain] → Keep #280 separate, prove supplied history and current codecs honestly, and update the parity map only for delivered fallback evidence.
- [Cloud-auth compatibility wording overstates or understates coverage] → Replace the blanket remaining-effectful-fallback clause with evidence-specific mapped local-tool/file/reasoning coverage. Preserve dummy-edge/auth/credential limits; missing provider-executed/MCP codecs and actual Cloud/BYOK support remain separate, not implied by configured-route tests.
- [Scope expands into public identity/failure confidentiality] → Those historical restrictions are superseded product work owned by #320–#324; this change does not alter their codecs or assert they are permanent policy.

## Migration Plan

1. Reconfirm the existing #318 stack, parent changes, affected specs and registered upstream versions; implement and review on `nrbrd/gw-fallback` and validate cumulative source. While #326 is open, target the child PR at `nrbrd/opt-forwarding`; merge after the parent, rebasing/retargeting to main after the parent merges.
2. Establish failing route/client regressions, remove the extra guard, and extend native/lifecycle/isolation evidence without new dependencies or protocol fields.
3. Ship updated existing spec requirements, centralized guides and stable coverage-map boundaries with passing source-workspace Gateway, ProviderWire, command, parity and module-boundary checks.
4. Existing YAML and public client requests need no migration. Deploy the normal Gateway image from the candidate source; rollback restores the previous image and its narrower eligibility, which applications using newly admitted fallback requests must account for.

## Open Questions

No unresolved API/design decision or parent-merge implementation gate is required. Use the already correct #318 stack, with parent-first merge ordering. If parent changes materially alter the composition/protection seams, or native mutation/codec changes prove necessary, stop and get scope approval before continuing.
