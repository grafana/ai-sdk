# Restriction dispositions

These are present review decisions, not approval of filters merely because they exist. P = protocol correctness, C = capability support, S = credential/tenant authorization, T = telemetry, O = obsolete account concealment. Future capability/transport proof is a named handoff, not a #303 completion gate.

## Caller responses and observation

| Restriction / evidence | Class | Decision / concrete reason / owner |
|---|---|---|
| Unary warning omission; fixed stream warning prose (`response.go:58-61`, `stream.go:232-266`) | O | Remove in #303: registered caller semantics, preserving warning bounds/union/order. |
| Source ID/display substitution (`sources.go:67-143`) | O | Remove in #303: preserve native source IDs/display without changing metadata transport. |
| Numeric citation/closed reasoning and text/tool/result/finish metadata filtering (`sources.go`, `reasoning.go`, `response.go`, `stream.go`) | O/C | Desired restriction removed by contract; implementation entirely #280. Existing bounded projections remain explicit temporary capability gaps until that successor, not security rationale. |
| Canonical actual-model substitution / unary identity omission (`stream.go:617-629`, `response.go`) | O | Remove in #303 at registered fields; typed unary overwrite remains a pinned client limitation. |
| Generic provider diagnostic concealment (`errors.go`, Go client `errors.go`) | O/S | Remove blanket rationale; #303 delivers minimal reviewed direct-route diagnostics within existing lifecycle/error API. Keep fixed internal/host errors and credential-source exclusions. Ambiguous aggregate/message-only/richer API work is a separately registered Gateway follow-up coordinated with #299, not a preapproved framework. |
| Native request/response URLs/headers/bodies/causes | S / transport ownership | Keep exclusion: concrete credential/signed URL/configuration/SDK dump sources and pinned client overwrite. Never generic raw serialization. |
| Canonical logical identity, route-only discovery, closed log/metric/metadata-only AO projection | T/S | Keep: authorization/configuration boundary and low-cardinality/content-free telemetry, not caller-response censorship. |
| Strict unions, UTF-8/input/encoded bounds, known usage, cancellation/finish/cleanup authority | P/S | Keep/extend for preserved fields. Source ID cap stays 1024 bytes; raw usage remains #281's object/1 MiB behavior. |

## Credentials, protocol and effect controls

Evidence: `ai-gateway/providerwire/v4/request.go:275-427`, `function_tools.go`, strict request schemas.

| Names / rule | Class | Decision / reason |
|---|---|---|
| Host namespaces `grafana`, `gateway`, `grafana-ai-sdk` | S/P | Keep explicit rejection: host routing/authorization controls cannot be restated in opaque provider options. Exact namespace matching remains. |
| Body headers authorization, proxy-authorization, x-access-token, x-grafana-id, x-api-key, api-key, openai-api-key, anthropic-api-key | S | Keep case-insensitive refusal: providers apply caller headers after owned auth; forwarding substitutes credentials. Independently test edge refused-header agreement. |
| Duplicate case-insensitive body-header names | P | Keep rejection: conflicting values otherwise depend on map order. |
| ai-language-model-* and ai-o11y-* body headers | P/T | Keep wire acceptance/non-forwarding: registered client body includes them, but they control Gateway protocol/observation rather than provider traffic. Ordinary noncredential headers retain their existing forwarding. |
| model, fallbacks | S/P | Keep refusal: cannot redirect an authorized route/backend or invoke unconfigured model candidates. |
| messages, prompt; role, content, type | P/S | Keep refusal: opaque spreading would bypass mapped role/content unions. Nested caller.type is not this top-level override. |
| tools, toolChoice, functions, function_call; tool_calls, tool_call_id | P/C/S | Keep bypass refusal; supported tool fields must use registered mapper. Does not prohibit authorized ordinary function history/caller. Provider ownership remains separate. |
| mcpServers, container | C/S | Keep current refusal: unconfigured effect/egress/credential paths. #239/#240 own reviewed activation, not account ownership. |
| responseFormat | P/C | Keep option override refusal; registered structured output capability remains separate. |
| stream, streamOptions | P | Keep option override refusal: cannot change the transport being decoded. |
| image_url, input_audio, file | P/C | Keep bypass refusal: ordinary supported file union has its own validation/bounds, not opaque option injection. |
| Output-level/non-file nested tool-result options | C/P | Keep current explicit unsupported boundary: no established mapping for these arms. #238/#239 readiness/runtime own extensions. File-entry options use reviewed existing scopes. |

No new credential/endpoint override is approved. Current global protected-field checks are necessary but not exhaustive semantic approval of an arbitrary compatible extension.

## Provider namespaces and fields

Evidence: command `service/provider_options.go`, runtime `provider_option_policy.go:17-109`, native provider converters/options. Keep means the indicated consumed request semantics at the already-supported scope, not an entitlement to every future response/capability.

| Fields / namespace policy | Decision / rationale / scope |
|---|---|
| Anthropic `thinking`, `effort`, `taskBudget`, `toolStreaming`, `disableParallelToolUse`, `betas`, `safeguards` | Keep consumed call controls: reasoning/budget/function configuration/provider opt-in classification. #280 owns lost returned metadata, including safeguard results; request acceptance is not complete response proof. No opaque tool/model injection. |
| Anthropic `structuredOutputMode` | Keep consumed mode on currently supported requests; it does not enable a refused structured response union. Structured-output activation remains separate. |
| Anthropic `toolChanges`, `cacheControl` | Keep native system/message/part semantics at their consumed scopes; do not lift tool-definition or role guards. |
| Anthropic `citations`, `title`, `context` | Keep document-part native input semantics; source display restoration is #303, full returned source metadata is #280. |
| Anthropic `signature`, `redactedData` | Keep native reasoning-history request data. #280 owns response-derived signatures/opaque continuation, not a new allowlist gate. |
| Anthropic `caller` (restored by this foundation) | **Remove omission in #303** for already-supported ordinary assistant function-tool-call history. Pinned/native consumers accept direct and both code_execution caller variants with toolId; do not impose #293's exact direct-only schema. History attribution does not enable provider execution/MCP. |
| Anthropic function-definition `deferLoading`, `allowedCallers`, `eagerInputStreaming` (consumed native tool options but absent from command field list) | Explicit capability/readiness handoff to #238/#239, including function-definition scopes and native beta/tool execution review. These are not credential secrets or proven ignored fields. Do not call omission full supported-option parity; caller history prerequisite does not implement deferred tool machinery. |
| OpenAI/Azure `conversation`, `previousResponseId`, `itemId`, `reasoningEncryptedContent`, `encryptedContent`, `phase` | Keep consumed conversation/history references and reasoning data; caller-supplied requests independent now, response-derived roundtrip #280. Route authorization is not altered by retaining registered references. |
| OpenAI/Azure `reasoningEffort`, `reasoningEffortUpdate`, `reasoningMode`, `reasoningContext`, `reasoningSummary`, `forceReasoning`, `compactionTrigger`, `contextManagement`, `truncation` | Keep consumed model-gated reasoning/context controls. Their returned data may have #280/capability gaps; do not silently lose accepted request values to hide the backend. |
| OpenAI/Azure `include`, `includeWebSearchSources`, `logprobs`, `metadata` | Keep consumed response-selection/request metadata semantics. These do not authorize provider tool definitions. #280 owns represented returned metadata; unsupported response content is an explicit capability boundary, not a parity claim. |
| OpenAI/Azure `instructions`, `systemMessageMode`, `textVerbosity`, `strictJsonSchema`, `passThroughUnsupportedFiles`, `imageDetail` | Keep consumed provider formatting/system/file controls within mapped scopes/current response support; schema/file unions and auth stay authoritative. Unsupported content/structured capabilities stay separate. |
| OpenAI/Azure `promptCacheKey`, `promptCacheRetention`, `promptCacheOptions`, `promptCacheBreakpoint`, `safetyIdentifier`, `serviceTier`, `store`, `user` | Keep consumed cache/safety/tier/storage/account request semantics. Customer authorization/provenance differs from telemetry; none are log labels. Returned service-tier/cache data belongs to #280. |
| OpenAI/Azure `parallelToolCalls`, `maxToolCalls`, `allowedTools` | Keep consumed limits/restriction of mapped declarations, not creation of undeclared provider tools. Native prepare_tools validates selection; provider definitions/execution remain #238/#239. |
| OpenAI/Azure `namespace`, `async`, `caller` | Keep consumed ordinary function history semantics where currently represented; provider program/server execution cannot be inferred from metadata. Extended markers/readiness #238/#239; response metadata #280. |
| OpenAI/Azure `approvalRequestId`, `approvalId` | Retain current recognized request-field policy but mark approval execution/history activation deferred: mapped union/ownership refusal still wins. Not accepted approval capability evidence. |
| OpenAI function-definition `deferLoading`, `allowedCallers`, `outputSchema` (native OpenAIToolOptions, absent from command fields) | Explicit #238 readiness / #239 runtime handoff at function-definition scope. Existing `async` and `namespace` are forwarded; deferred loading/programmatic authorization/output-schema activation is not proven by ordinary function history. These omissions are not ignored native values or credential protection. |
| Compatible assistant function-call `google.thoughtSignature` and configured-key thought signatures | Native Go consumes the Google fallback on ordinary tool-call parts; the command policy drops that namespace unless its provider name is google. Pinned converter also reads the configured namespace first, whereas current Go only reads google. Explicit #280 continuation-policy/producer handoff, with `TestDoGenerateSendsGoogleThoughtSignature` as direct consumed-scope evidence. No claim of complete supplied or response-derived signature parity. |
| Missing/unknown fields for typed Anthropic/OpenAI namespaces | Native consumed supported omissions are bugs to fix or explicitly hand off, never presumed harmless. Truly ignored fields may be omitted with matching pinned consumer evidence. Future/effectful fields require capability review; no catch-all silent-loss approval. |
| Namespace outside selected provider's consumed namespaces | Keep omission only where actual selected converters ignore it. Native scope tests justify irrelevance; namespace spelling remains exact. |
| Compatible configured-name/camel and fixed namespaces | Preserve provider-consumed namespace precedence and current extension purpose, not unrestricted approval. Native `readOpenAIOptions` only passes unknown fields for configured-name keys; message/part ExtraFields spread. Keep global auth/role/union/transport refusals and audit endpoint-specific retained extensions by scope; unsafe overrides refused, unsupported effect fields deferred to #239/#240/#115 policy. |
| Generic zero policy | Keep fail-closed capability boundary for an unreviewed resolver; document unsupported option semantics rather than claim such a model consumes nothing. |

Grouped rows enumerate every field currently listed in service policies; implementation must verify scopes and matching native consumed/ignored cases rather than infer support from list membership. Known omissions above have concrete owners. Any additional consumed omission found by those witnesses requires an explicit correction/handoff, not automatic retain-filter approval.

## Fallback

Evidence: `service/fallback_route.go:27-67`, `service/catalog.go:143-153`.

| Restriction | Decision / reason |
|---|---|
| tools, effectful history, non-pure-auto tool choice, files, raw/unsupported formats | Keep explicit pre-invocation refusal: existing fallback commits on first part and lacks capability-specific replay/effect design. #239/#240 must not silently enable effectful fallback. |
| Active provider options/call headers on fallback | Defer supported scalar/continuation/header routing until candidate semantics/auth/commitment proof exists. These controls are not all effectful or secret. Keep explicit unsupported-request response, not silent success with lost semantics. No new fallback capability in #303. |
| Semantically empty ordinary message namespaces | Keep eligibility and value preservation; no recursive active-value-to-empty normalization. |
| Different candidate policies collapse to zero | **Remove silent loss as an accepted policy in #303**: retain ordering/security, but explicitly reject active options consumed by any candidate when no safe common routing is supported, before invoking a candidate. Truly irrelevant options remain ignorable only when all candidates ignore them. Requires original-request versus filtered-request regression evidence. |
| First provider part (including error/source) commits; no replay afterward | Keep execution/lifecycle authority independent from caller-visible error/identity changes. |
| Canonical logical observation and private attempt records | Keep T/S protections; registered actual response identity is not the attempt/configuration dump. |

## Scope witnesses and evidence limits

The immutable registered commit remains `4e8c387622ee1bb0d55841664416d38754d5c9a3`. Reviewed sources include Anthropic options/prompt conversion, OpenAI Responses options/input conversion and compatible chat options/message conversion. This is a filter audit against the current supported Go surface, not an upstream upgrade or full native-adapter parity claim.

`TestProviderOptionPolicies_AllClassifiedFieldsAtMappedScopes` verifies every listed field's opaque values through the real handler at call, system/message, text/file, function-definition, assistant function-call and nested result-file scopes for Anthropic/OpenAI/Azure. Its synthetic opaque values prove policy forwarding only. Native tests below establish consumed groups independently; values at unrelated scopes remain native-ignored, not promoted to call options.

| Group | Native scope witness / explicit boundary |
|---|---|
| Anthropic thinking/effort/task budget/betas/safeguards | `TestBuildParams_ProviderOptions_*`, `TestSafeguardsRequest`; native call settings, with model-gated warnings/validation. Go `betas` is the current native spelling; pinned `anthropicBeta` is not equivalent policy coverage. |
| Anthropic structured/tool controls | `TestBuildParams_StructuredOutput`, `TestBuildParams_DefaultEagerInputStreaming`, `TestBuildParams_ToolChoice`; call-scoped mode/defaults do not lift the Gateway structured-output/provider-tool refusal. |
| Anthropic system/cache/document | `TestBuildParams_SystemMessage`, `TestBuildParams_*CacheControl`, `TestBuildParams_DocumentMediaTypes`, `TestExtractCitationDocuments`; toolChanges on system messages, cacheControl at consumed message/part/tool scopes, citations/title/context on document parts. |
| Anthropic reasoning/caller history | `TestBuildParams_AssistantWithReasoning`, `TestConvertAssistantContent_ProviderExecutedToolCalls` ordinary-history subcase; authenticated caller command tests cover direct and both code_execution variants. No output-derived continuation claim. |
| OpenAI conversation/reasoning/context | `TestBuildParams_ProviderOptions`, `TestBuildParams_TopLevelReasoning`, `TestBuildParams_GPT6ReasoningEffortCapabilities`; call-scoped references and capability-gated context controls. |
| OpenAI includes/logprobs/metadata/tool restrictions | `TestBuildParams_ProviderOptions`, `TestPrepareTools_WebSearchSourcesIncludePrecedence`, `TestPrepareTools_AllowedToolsMapsProviderToolNames`; selection does not declare or authorize provider execution. |
| OpenAI cache/account/storage/formatting | `TestBuildParams_PromptCacheBreakpoint`, `TestApplyProviderOptions_ServiceTierFast`, `TestBuildParams_SystemMessageModes`, `TestBuildParams_UserImageAndFile`; call controls versus system/user-part cache and image options remain distinct. |
| OpenAI item/encryption/phase/async/caller/approval | `TestBuildParams_StoredAssistantReference`, `TestBuildParams_ProviderToolContinuationProviderOptionsNamespace`, `TestBuildParams_ProviderToolContinuationTaxonomy`; supplied ordinary history is separate from stored/provider-executed/approval capability, which remains guarded. |
| Compatible namespaces/extensions | `TestOpenAICompatibleOptionPolicy_NativeScopesAndPrecedence` captures both modes: typed precedence deprecated → fixed → configured raw → camel; unknown call fields only configured keys, message/user part fields only openaiCompatible. Single text input's native shortcut projects part fields on the message rather than a content array. `TestDoGenerateProviderOptionsMatchUpstreamKeys` independently witnesses native precedence. |
| Protected fields/headers/fallback | Runtime protected-option/header tests, inbound edge refused-header agreement, command negative controls and mixed-provider refusal tests; no native invocation for protected or consumed-unrouteable inputs. Ignored namespaces and empty message namespaces retain eligible fallback. |

Additional registered producer gaps are not presumed irrelevant: Anthropic `sendReasoning`, call `metadata`, `speed`, `serviceTier`, `inferenceGeo`, `anthropicBeta`, `contextManagement`, mid-conversation `clearAt`/`effort`, and thinking blockBinding are missing in current Go option/converter representation. They require a separate Anthropic provider parity work package (registration candidate, not created here), including request snapshots and native model/beta semantics before Gateway adoption. Container upload/skills remain #239/#240 capability work. OpenAI `ultrafast` tier is likewise a producer capability gap, not a Gateway field omission. Go's string-only OpenAI metadata and typed numeric controls cannot claim every pinned opaque/number input. These representation gaps are outside this Gateway policy foundation and remain open; no fixture or permissive client test is used to claim otherwise.

#303 completion covers audited foundation corrections and explicit unsupported boundaries; metadata-derived continuation, provider tools/MCP and full authentic replay remain named later acceptance in handoffs.md.
