## Context

Issue [#318](https://github.com/grafana/ai-sdk/issues/318) and work package 31 of `~/ai-sdk-ai-gateway-plan.md` own direct native request-option forwarding. This is a fresh plan from canonical main `c0299776`, not a continuation of stopped #303/#309.

The mapper currently preserves namespace JSON as `provider.RawProviderOption`, but rejects a normalized global list of field names. After resolution, `handler.go` applies `catalog.ProviderOptionPolicy` recursively across call/message/part/tool/file-result scopes. Service inventories enumerate native fields and `sharedOptionPolicy` intersects candidate policies; unknown fields and namespaces disappear. This loses already-supported values, including Anthropic `caller` and contextual `type`, and unnecessarily duplicates native namespace selection.

### Registered authority and evidence

The reference is commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`: ai 7.0.116, Gateway 4.0.94, Provider 4.0.18, Anthropic 4.0.65, OpenAI 4.0.78 and openai-compatible 3.0.57. Source and relevant tests were inspected with `git show` at that commit. The local `~/src/ai` HEAD is newer (`b3033f77`); its working files were not used as the baseline.

Relevant upstream paths at that commit:

- `packages/gateway/src/gateway-language-model.ts` and its tests: removes abortSignal, encodes file data and submits the remaining options; call headers also participate in outer header composition. It does not implement the Go Gateway inventories.
- `packages/anthropic/src/convert-to-anthropic-prompt.ts` and its tests: reads scoped options for cache/document/history conversion, contextual compaction `type`, reasoning and supplied tool-call/result `caller`. The provider-executed dynamic-filtering test is a consumption reference, not approval to enable that unsupported Gateway branch. Current Go `convertAssistantContent` consumes caller on ordinary assistant tool-use; ordinary tool-role results go through `convertToolContent`/`appendToolResultBlock` without caller extraction. Assistant web-result caller branches are deferred. Preserve ordinary result-part caller to native invocation, but do not claim native consumption or change the converter to manufacture it.
- `packages/openai/src/responses/convert-to-openai-responses-input.ts` and its tests: uses selected namespaces for assistant item IDs, phase, reasoning and tool history. Native conversion, rather than Gateway field enumeration, owns precedence and representation.
- `packages/openai-compatible/src/chat/openai-compatible-chat-language-model.ts`, `convert-to-openai-compatible-chat-messages.ts` and associated conversion tests: spreads configured-name call extensions and `openaiCompatible` scoped metadata. Later native fields override some earlier spreads; others can change mapped role/content or model if left unprotected.

`test/conformance/PARITY.md` classifies this as ProviderWire request projection, Gateway runtime/client and provider implementation boundary work. Existing client goldens establish projection; mapped model assertions alone cannot prove native consumption. Fake native request tests are synthetic evidence, not recordings or proof of live provider acceptance. Retained credential/protocol/execution protections are intentional Gateway adaptations, while loss from inventories is an implementation bug. Unsupported codecs and unproven native behavior remain coverage/support gaps.

## Goals / Non-Goals

**Goals:**

- Carry ordinary options to native adapters at all already-supported scopes without namespace/field loss or mutation.
- Let native parsing, matching namespaces, warnings and precedence determine ordinary option behavior.
- Retain concrete bypass protections, demonstrated by positive and ordinary-value negative controls.
- Prove direct unary/streaming behavior through both clients, production handlers and the authenticated command.

**Non-Goals:**

- No response metadata transport, actual output-derived continuation, raw/diagnostic carrier or response-policy rewrite.
- No fallback capability expansion, new executor, per-attempt translation or replay semantics.
- No routing/BYOK implementation, discovery, provider-tool/MCP activation, new unsupported content codecs or capture-policy changes.
- No unrelated native adapter fixes or upstream pin upgrade.

## Decisions

### 1. Remove inventory filtering rather than expand it

Delete the handler filtering pass and its helper implementation. Remove catalog `ProviderOptionPolicy`, policy storage/cloning and service inventories/intersection once repository-wide references confirm no real consumer remains. Mapping keeps namespace objects as opaque JSON at their existing positions, preserving raw namespace bytes where no host-owned control is consumed. Unknown ordinary namespaces and fields reach the native model; the adapter may ignore them, validate them, consume them or emit native warnings.

Do not hoist message/part/tool/file-entry values, flatten nesting, normalize namespace names or rewrite ordinary JSON. Preserve explicit `{}` and nested null/false/zero/empty strings/arrays/objects. Namespace case and native alias/precedence semantics remain adapter-owned. Independent invocations must not share mutable maps, slices or raw JSON storage.

Alternatives rejected: expanding the existing field lists merely postpones silent loss; a new direct-only exact-key registry recreates the same defect; preselecting one candidate's namespaces conflates routing with native interpretation.

### 2. Safety follows actual consumption, not suspicious spelling

Keep complete schema/role/union validation and unsupported codec checks in the protocol mapper. Keep reserved host controls and protected body-header checks separate from ordinary options. Audit each current protected field against native Go construction/conversion and the registered upstream behavior at the exact consuming scope. For each retained check, identify the path that can change credentials/account/destination, the resolved model/prompt, mapped role/union, tool ownership or expected transport/execution.

The audit is a completion gate before weakening any global guard: record the actual consuming code path, namespace/scope, recognized spelling, precedence, protection/native-precedence/harmless disposition and paired bypass/ordinary regression cases in this design. In particular, current Go `convertAssistantContent` checks `isMCPToolUse` before `ProviderExecuted`, whereas the pinned TypeScript MCP branch is inside `providerExecuted`. A local assistant function-call can therefore enter MCP conversion through `anthropic.type`; protect this concrete Gateway execution boundary without repairing the native converter in #318. Distinguish it from safe assistant text compaction `type` and nested caller metadata. The audit is bounded safety evidence, not an allowed-field catalog or blanket all-provider scope validator.

Enforce native-specific checks at the trusted native invocation boundary, with the adapter identity/configuration available, before external I/O. Prefer existing native precedence where it demonstrably prevents the bypass. Where native spreading really can bypass the Gateway boundary, add a narrowly scoped refusal rather than silently strip the field. Do not infer a native adapter from caller-supplied namespaces or a middleware-renamed logical Provider() value. Choose the smallest existing composition seam after the audit; do not reintroduce allowed-field or allowed-namespace catalogs.

Check only names and aliases the consuming code actually recognizes; `encoding/json` case-insensitive decoding differs from exact map spreads. Do not globally fold hyphens/underscores or recursively scan nested opaque values. An unused `role`/`type` member in an irrelevant namespace is ordinary data. A native-consumed Anthropic history discriminator is not a compatible-message role override. Nested `caller.type` or application strings resembling credentials are not host controls.

The test matrix must include Anthropic typed call/tool options, supplied caller/history conversion, OpenAI/Azure selected namespaces and compatible configured-name call extensions versus fixed `openaiCompatible` scoped spreads. New or unknown ordinary fields are not automatically refused. If an unrelated adapter defect prevents proof, report and scope it separately rather than adding a Gateway workaround inventory.

Alternatives rejected: deleting every guard would expose actual overrides; a global blacklist rejects legitimate native history; a generic recursive secret scanner invents semantics for opaque application JSON.

### 3. Keep host controls and the two HTTP hops distinct

Existing reserved `gateway`, `grafana` and `grafana-ai-sdk` controls remain explicit unsupported failures until an owning feature consumes them. They never become native option namespaces. #316/#317 own routing and request-scoped BYOK; this change does not partially implement either. Host-control refusals and malformed namespace validation remain before invocation, without echoing contents.

Outer authentication stays in the authenticated edge. Only mapped body headers participate in native calls, under existing credential/protocol checks and native SDK precedence. Arbitrary outer headers do not become native headers. The registered TS client combines call headers into its outer hop too, so authenticated test transport adaptation must remain explicit rather than being mistaken for server forwarding.

Operator metadata-only capture does not filter request options or determine returned semantics. Preserve operator and consumer settings and do not reflect credential-bearing controls or expand result metadata as part of these tests.

### 4. Keep fallback separately owned, even when removing intersection plumbing

Remove dead policy intersection along with the catalogs, but retain the existing `fallbackTextModel` guard, selection, cancellation and commitment logic. The guard sees the preserved request, not a sanitized projection. Empty message namespace objects keep their existing semantic-empty allowance; active namespaces cannot become eligible merely because an inventory would have discarded them. This can reject requests previously admitted after silent filtering and is an explicit consequence, not a new fallback expansion.

Update stale spec/docs references to selected-backend filtering. Add regression checks for rejected tools/files/active options and preserved empty message options; do not use fallback success as acceptance for opaque native options. #319 independently owns broader mapped fallback support and native options across attempts. Retained fallback rejection is an interim delivery boundary, not the desired permanent product policy.

### 5. Reproduce loss before implementation and prove native requests independently

Extend existing ProviderWire request cases/client differential tests and `gateway-command.test.ts` using native fake HTTP endpoints. Add focused synthetic request assertions before the behavior fix and confirm failure against current main. Use the registered TS Gateway client and independent Go Grafana client in both modes; raw HTTP and mapper tests retain authority for malformed inputs, exact refusal/processing order and raw JSON preservation.

The acceptance matrix covers:

- Call/message/content/function-tool/file-result scopes, empty namespace objects and nested opaque JSON; scope isolation and irrelevant namespaces at the mapped model boundary.
- Actual native consumption: Anthropic call/cache/tool options and supplied local assistant-call caller history; OpenAI/Azure phase, item/reference and reasoning controls; compatible custom call fields and message/part extensions. Ordinary tool-role result caller is retained to native invocation but currently ignored by the native converter; test that boundary separately, not as consumed result caller. Compare against direct native calls with equivalent options where practical. Do not change native converters or enable deferred assistant/provider-executed result branches to satisfy an overbroad history assertion.
- Native request negative controls: attempted credential/account/destination/model/prompt/role/union/tool/transport overrides versus similarly named harmless fields at non-consuming scopes; namespace aliases, exact case and competing namespace precedence.
- No native I/O for malformed namespace objects, reserved controls, protected headers, missing codecs or real bypass attempts. Adapter-aware checks may require resolution, unlike schema/host checks.
- Concurrent distinct requests, immutable mapped inputs/namespace JSON and reusable adapter configuration, tested under Go race detection.
- Identical request/result semantics with operator observation enabled versus disabled while forbidden credential-bearing values remain absent from operator capture.

No hand-authored provider chunks enter `recorded/` or `upstream/`; new native fake requests belong in focused tests. Existing pinned-client goldens change only through their registered generator when justified. Update the coverage map only for stable evidence/boundary changes, not to claim exhaustive native parity.

## Risks / Trade-offs

- [Removing filtering exposes native override paths] → Complete the consumption/precedence audit and paired bypass/ordinary tests before deleting broad protections; require zero external I/O on actual bypasses.
- [Middleware hides native identity] → Place any native-specific safety checks at trusted construction/invocation seams, not on public logical identity or caller namespaces.
- [Native SDKs ignore unknown fields] → Promise lossless forwarding to the adapter, not universal passthrough to provider HTTP. Show consumption separately from mapped preservation.
- [Preserved options make previously filtered fallback requests fail] → Document the stricter visible request boundary; retain the existing guard and defer expansion to #319.
- [Supplied history is mistaken for full continuation] → Use explicit caller-supplied values; do not claim response-derived metadata support or live acceptance.
- [Audit grows into adapter repair or response/capture redesign] → Stop and request scope approval; register unrelated gaps separately.

## Implementation validation

The new focused tests failed on the initial code: harmless scopes were rejected, options were inventoried away, and compatible assistant-call `function` metadata could rewrite native history. Final focused native requests, both-client handler/command cases, and shared-handler races pass after removing filtering and applying the audited native-boundary checks. Ordinary Anthropic result caller remains ignored by native conversion; no native provider source or response codec changed.

Passed: `mise run test-providerwire-v4`, `test-ai-gateway-command`, `test-ai-gateway-source-integration`, `test-ai-gateway`, `parity-check`, `build`, `vet`, `lint`, `lint-docs`, `verify-gateway-workspace`, `verify-merged-pins`, `verify-ai-gateway-boundary`, `verify-sdk-gateway-isolation`, and `test-ai-gateway-image-source`; standalone readonly `MODULE=providers/grafana mise run verify-published-module`; Go race tests for Gateway runtime, service and catalog; strict OpenSpec validation and whitespace checks. `gofmt -l` is clean. `mise run fmt-check` reports the existing uncommitted diff because its final gate is `git diff --exit-code`; it is not usable as a clean-worktree formatting gate before committing. No commit was made to satisfy that gate.

The race run initially caught the new capture test reading an unsynchronized test log buffer; using the existing locked test buffer fixed it. No production concurrency workaround was needed. Provider fixture inputs, generated goldens and registered pins are unchanged. Image-source/build checks establish candidate-source delivery, not deployment or live provider acceptance. Final scope review found no response/fallback-expansion/routing/BYOK/capture-policy changes; fallback rejection remains the documented interim boundary.

## Migration Plan

Land removal, focused protections, specs/docs and tests together. There is no compatibility shim for the removed catalog policy API and no protocol version or baseline bump. Deploy using candidate-source Gateway checks; rollback reverts the complete change, including its safety checks. A rollback restores historical filtering and is not a fallback feature solution.

## Completed consuming-path audit

Implementation baseline reconfirmed at `c029977691f75fd1cc1095e2d2d586ef4ac434a4`; registered pins unchanged. Repository-wide policy consumers are only the handler filter, catalog storage, command construction and their test harnesses; no functional consumer requires an inventory.

| Consuming path | Native spelling/scope and precedence | Disposition and paired proof |
| --- | --- | --- |
| Anthropic `ResolveOption[AnthropicOptions]`, `applyProviderOptions`, `applyFallbacks` | Exact `anthropic` call namespace; typed JSON decoding is case-insensitive but does not fold underscores/hyphens. Nonempty MCP servers, container skills and active fallbacks add server tools/models. | Refuse these active values; allow empty/null no-op values, container ID without skills, unrelated namespaces and unrecognized spellings. Native malformed-value validation remains native. |
| Anthropic `convertAssistantContent`, `extractAnthropicType` | Exact `anthropic` assistant tool-call part; raw JSON struct decoding recognizes case-insensitive `type`; MCP branch precedes ProviderExecuted. | Refuse `mcp-tool-use` here; allow compaction on assistant text, ordinary local-call caller, ignored type on other scopes and namespaces. Do not fix native converter. |
| Anthropic ordinary tool-role conversion | Result-part caller reaches `convertToolContent`/`appendToolResultBlock` but is not read. | Preserve to adapter invocation; assert no native result-caller consumption. Deferred assistant result paths remain unsupported. |
| OpenAI Responses `resolveProviderOptions`, typed request/input builders | OpenAI-first/Azure-fallback selection; typed options and selected part fields, no arbitrary request/body spreads. | No native-specific bypass guard: unknown model/role/tools/type spellings cannot override native request ownership. Preserve phase/item/reasoning and existing native instructions/system-mode controls. |
| Compatible `readOpenAIOptions`/`buildRequest` | Configured name before first dot, trimmed, and its camel alias spread unknown call fields with exact JSON keys. Native later fields protect messages/tools/tool_choice/stream/stream_options; earlier model/response_format may be replaced and legacy functions/function_call may be added. | Refuse exact model, response_format, functions and function_call in those spreading call namespaces. Allow native-protected later fields, other case/spelling, unrelated namespaces and headers/baseURL-like body values that cannot redirect transport. No guard for fixed typed namespaces when they do not spread. |
| Compatible `marshalWithExtra`, `chatContentPart.MarshalJSON` | Exact openaiCompatible scoped metadata overrides constructed message/part/tool-call fields. System/assistant message options and ordinary result-part options become message extras. Single user text-part extras become message extras (user message options ignored); multi-part user message options remain message extras and each part has content extras. Assistant text/reasoning options, tool-role message options, function-tool/file-result options are ignored. | Protect exact role/content/tool_calls/tool_call_id/function_call/reasoning_content at actual message spreads; type/text/image_url/input_audio/file at user multi-part content spreads; id/type/function at assistant tool-call spreads. Paired tests admit protected-looking names at ignored scopes and preserve safe extensions/nesting. |
| Host/body headers | Mapper reserves exact host namespaces and refuses credential headers case-insensitively; protocol names in body are omitted. Outer edge auth is independent. | Keep existing checks and fixed errors; arbitrary application strings/ordinary namespaces are not host controls. |

The chosen seam is a small service-owned native model wrapper inserted at candidate construction, before logical middleware renames identity. It delegates unchanged options after only the relevant Anthropic or compatible consuming-path check. OpenAI needs no wrapper. Failures use existing `catalog.ErrUnsupportedRequest` and the existing fixed invalid-request response; no new validator/catalog API or field inventory. Resolver-supplied models remain trusted execution implementations responsible for their native safety, just as service composition owns credentials/endpoints. Paired guard tests and native request assertions must pass before completion.

## Open Questions

No product decision blocks planning. The implementation audit must resolve the minimal native-specific guard seam and exact consuming field/scope cases before weakening current checks. Preserve the established outcome rather than preapproving a generic validation framework; escalate any actual scope or supported-codec conflict to nara.
