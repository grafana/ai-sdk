# Gateway developer evidence/access decision

Status: approved by nara's explicit `approved` reply to the concrete decision's approval request in this conversation. #321 is complete as design/probes only; production implementation remains with the downstream owners.

## Reference and evidence boundary

Starting Go source: canonical-main `c0299776`. Installed packages match `test/conformance/upstream.yaml`: ai 7.0.116, Gateway 4.0.94, Provider 4.0.18, Provider Utils 5.0.49; upstream commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`.

Pinned sources: [Gateway model](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/gateway/src/gateway-language-model.ts), [errors](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/gateway/src/errors/create-gateway-error.ts), [discovery](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/gateway/src/gateway-fetch-metadata.ts), [stream normalization](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/ai/src/prompt/normalize-stream-provider-error.ts). These are public-client evidence, not a private-service oracle.

Current [Vercel routing documentation](https://vercel.com/docs/ai-gateway/models-and-providers/provider-filtering-and-ordering.md) shows `gateway.routing.originalModelId`, `resolvedProvider`, `resolvedProviderApiModelId`, `canonicalSlug`, `finalProvider`, `modelAttempts[].providerAttempts[]`, counts and completed `success` values. It is a separately inspected service reference, not pinned runtime proof. Do not copy planning prose, credential/account identifiers, costs, private timing conventions or available-fallback claims that our producers cannot establish.

## Native producer inventory

| Source | Available on successful unary | Stream setup / events | Failure evidence / missing fields |
| --- | --- | --- | --- |
| Anthropic / Vertex | Final transformed JSON request, response JSON, response headers and native identity (`providers/anthropic/model.go`) | Request JSON, initial response headers; native identity/events | APICallError URL/request JSON/headers/body/data where captured (`wrap_api_error.go`). Request headers are not exposed. Vertex shares producer; project/location is topology, workload/private-key/token material is not |
| OpenAI Responses / Mantle | Final captured JSON request, raw response JSON, headers/native identity (`providers/openai/model.go`) | Request JSON, initial headers, response identity/events | API failures expose URL/request JSON/headers/body; typed stream errors may expose frame data. Request headers and actual SDK retry count are not exposed. Mantle's signed transport must not be reconstructed from model identity |
| OpenAI-compatible | Encoded request, raw response JSON/headers/identity (`http.go`, `convert_response.go`) | Request JSON, initial headers/identity | API error fields where returned. No request-header capture or native SDK retry count |
| Bedrock Converse | Encoded request, headers, response identity (`http.go`, `convert_response.go`) | Request JSON, initial headers; native Smithy parts are not a JSON response body | Unary response body is not retained by GenerateResponse; request signing headers and actual retry count unavailable. HTTP failures have URL/request/header/body evidence; stream exception availability differs (`wrap_api_error.go`, `convert_stream.go`) |

All success RequestMetadata types contain only body; no generic native request-header source exists. A full successful streaming response body does not exist in StreamResult. Mark these unavailable, not absent empty objects; do not add a capture seam or another stream consumer in #321/#323 merely to manufacture them. Non-JSON/transformation/transport failures may have less evidence than their adapter's normal path. Native type/code values are extracted only from well-formed available provider data, not inferred from joined error prose (#299 owns producer/core changes).

`fallback.Attempt` provides actual index/provider/model/start/end/error/selection outcome and `WillFallback` intent at decision time. Another actual attempt proves subsequent invocation; selection does not prove stream completion. Direct attribution requires the analogous invocation seam. The canonical wrapper is not native identity.

Existing Go client limits: error 65,536 bytes; event 1,048,576; unary 16,777,216; discovery 4,194,304; cumulative stream 67,108,864; events 100,000. PARITY.md classifies Gateway runtime/client and provider transport as mixed; synthetic access tests are not native recordings, deployment isolation or complete parity proof.

## Approved contract and access

**Approved decision:** use additive registered metadata/error shapes, not a new endpoint, sentinel protocol or serialized Go error tree. Keep normal Request/Response about the Gateway hop. The extension is explicitly Grafana-owned; acceptance by a permissive client does not make it a Vercel service guarantee.

| Carrier | Exact placement / access |
| --- | --- |
| Unary success | `providerMetadata.gateway.evidence` in the server result; TS `doGenerate().providerMetadata`, `generateText().providerMetadata`; Go `GenerateResult.ProviderMetadata` after #280 |
| Stream success | The same snapshot at registered finish `providerMetadata.gateway.evidence`; low-level finish and high-level result metadata after consumption. Ordinary earlier native metadata stays at its original registered event positions; no invented metadata event or extra reader |
| Unary/setup/direct/all-failed error | Add top-level `providerMetadata.gateway.evidence` to the existing `{error:{message,type,code,param:null}}` envelope. TS: guard `GatewayError.cause` with `APICallError.isInstance`, then read `cause.data.providerMetadata` (or bounded `responseBody` JSON). Go after #322: `errors.As` to `*provider.APICallError`, then decode `Data` as that complete envelope |
| Committed stream error | Keep existing Gateway-mapped `error.message/type/code/param/statusCode/retryable`. Add `error.data = {providerMetadata:{gateway:{evidence}},nativeError?}`. Go after #322 exposes exactly that JSON in APICallError.Data. Pinned TS forwards it unchanged low-level; high-level StreamProviderError.data retains the entire error object for this shape, so use `.data.data.providerMetadata` after the public instance guard |
| Configured discovery | Existing `/config` model rows gain `gateway:{canonicalModelId,aliases,candidates:[{providerInstance,provider,modelId}]}`. Canonical/alias rows remain ordered and compatible; candidate array order is configured order. Go #324 adds optional `ModelInfo.Gateway *ConfiguredRoute`, with typed aliases/candidates, to existing ListModels; no second client/method. Stock TS getAvailableModels still strips extras; #324 supplies a documented/typechecked bounded `fetchConfiguredModels({baseURL,headers,fetch,signal,maxBytes})` consumer helper against this same authenticated route, not a new published TS package |

The minimal public change approved here is Go discovery's optional field/types and the explicit TS discovery helper/example. Go success/error retention uses existing ProviderMetadata/APICallError.Data, requiring internal decoding changes but no new error API. No generalized diagnostic accessor is necessary. The discovery helper must validate the complete configured document, reject malformed/duplicate/over-limit catalogs atomically, support selected JWT or CAP headers explicitly, cancel bounded reads and refuse redirects; the current test-only helper proves bounded access/auth placement, not that complete future validator.

### Evidence shape and semantics

`evidence` is one request-scoped JSON object:

- `requestedModelId`, `canonicalModelId`: original request and catalog resolution. Omit canonical when resolution never happened; pre-provider/internal errors have no fabricated attempts.
- `selectedAttempt`: optional one-based actual invocation index. No value for an exhausted/unselected request.
- `attempts`: ordered actual invocations, never configured-but-unrun candidates. Each has `index`, `provider` (adapter identity), `modelId` (configured invocation model), optional `providerInstance` (service's configured candidate reference, never inferred from native response), `selection: failed|selected|canceled`, optional `completion: completed|incomplete`, optional `willFallback` (intent at decision time), optional `nativeRetryCount` only if observed, and optional `nativeError`.
- `nativeError`: native `message`, and independently available `type`, string/number `code`, HTTP `statusCode`, `isRetryable`, optional `details` diagnostic component. Unknown fields/counts are absent, not zero, false or inferred from status/prose. Distinguish native retryability from actual fallback eligibility/intent. No primary identity is paired with an arbitrarily unwrapped aggregate cause.
- `native`: optional **selected-attempt** transport with `requestBody`, `responseHeaders`, `responseBody` diagnostic components and optional `responseIdentity:{id,modelId,timestamp}`. It is attributed through `selectedAttempt`, not requested/canonical IDs. Failure transport belongs to the matching attempt's `nativeError.details` when available; use the same component rules, not a second error codec. Unselected setup failures have no selected-native success object. No request headers or full successful stream body are fabricated.
- `gateway`: `{httpStatusCode,classification:<existing mapped category>,phase: unary|setup|committed,isRetryable,replayRisk?: generationMayHaveRun|generationCompleted}` when applicable. HTTP status is the actual Gateway hop (200 on committed SSE), classification/retryability are Gateway-mapped facts. The stream error's existing `statusCode` is its mapped error status, not a second native status. Native SDK retry facts, server fallback intent and consumer/core retry decisions remain separate.

Completion means observed terminal generation outcome, not error-free content or merely selected setup. Omit completion until observed. Error events followed by later valid content/finish keep their event order; a later terminal observation does not erase the earlier failure. Post-generation adaptation failures report `completion: completed` only when generation really finished and expose a Gateway adaptation classification/replay risk in `gateway`; they never claim no paid execution or exactly-once behavior. Cancellation between fall-through intent and the next invocation creates no invented next attempt.

Compatible routing projection is deliberately smaller than Vercel's example: `gateway.routing.originalModelId` = requested ID, `canonicalSlug` = canonical public route, `finalProvider` only after an actual selected candidate. Do not use routing fields as native response identity. Injected `resolvedProvider/resolvedProviderApiModelId` in probes show consumption only; their phase semantics across Vercel fallback are not precisely established by the available documentation, so omit those fields rather than silently equate planning with final selection. Similarly omit planning prose, credentialType, private timing, cost, availability and unproved nested model-attempt grouping. The authoritative exact actual-attempt facts are in the approved Grafana evidence extension; #316 reuses them.

### Collision and replacement

Gateway constructs its own `gateway.routing/evidence`; native metadata cannot supply those trusted values. If native result/finish metadata already has a `gateway` namespace, retain its entire original object under `providerMetadata.gateway.nativeMetadata` instead of merging/overwriting it. Other namespaces stay opaque and unchanged. This reserved-namespace relocation is an explicitly approved Grafana extension/deviation requiring downstream documentation, not a generic #280 metadata rule. The witness includes an opposing native routing claim and retains nested null/false/zero/empty values.

All Gateway snapshots replace the previous **Gateway snapshot** at the relevant scope; do not recursively merge attempt lists or native metadata. Preserve ordinary registered metadata's original placement and upstream non-nullish replacement semantics. Collision relocation must be performed once by the feature owner, never by generic middleware/Go decoding. NativeMetadata is untrusted evidence, not authorization input.

### Observed access today / downstream updates

Focused [TS access probes](../../../../ai-gateway/test/providerwire-v4/developer-evidence-access.test.ts) and [independent Go probes](../../../../providers/grafana/developer_evidence_access_test.go) exercise injected responses at `c0299776`:

| Surface | Observed stock behavior / limitation |
| --- | --- |
| TS unary | Metadata survives low/high level by default. Native server transport survives only inside low-level response.body. High-level body requires `include:{responseBody:true}`; default omits it. Top-level response identity is replaced/generated, not native |
| TS stream | Initial headers describe Gateway hop; response-metadata preserves native identity; finish metadata reaches the assembled high-level result |
| TS errors | Setup/unary evidence is accessible via the public API-call cause, not GatewayError top-level fields. Committed error normalizes to StreamProviderError with whole error as data; later text/finish remains consumable |
| Go unary | Replaces normalized transport and drops result metadata; raw bounded Response.Body still permits explicit JSON inspection. #280 updates the normalized-loss expectation |
| Go stream | Preserves native response-metadata identity but drops finish metadata. #280 updates that expectation |
| Go errors | Reconstructs the limited public error body, dropping additive attempts/native evidence and Data. #322 updates these loss expectations. StreamText retains later text and Err; GenerateText returns no result when Err is present. #299, not this change, owns core policy |
| Discovery | Both normalized clients drop candidates today. #324 updates Go loss expectations and implements the explicit TS helper; stock TS normalization remains unchanged |
| Consumer middleware | Existing WrapLanguageModel hooks observe low-level result/stream/error surfaces under independent in-memory capture configuration. No server/capture infrastructure is used. Values already discarded by client decoding cannot be recovered by middleware |

These test names explicitly describe current-baseline losses, not permanent required behavior. #280/#322/#324 update only the expectations for retention they actually deliver; #318 options/#320 scalar identity changes may require witness adjustments. A test-only hand-prepared collision/sanitized object proves access/representation, not server provenance/redaction enforcement.

## Bounds and credential dispositions

A diagnostic component is `{state:available,value:<JSON>,redacted?:true}` or `{state:unavailable|redacted|malformed|over-limit,reason:<closed reason>}`. A body declared JSON must be valid complete JSON; a known non-JSON response body can be a complete text string with its media type, not incorrectly called malformed. Valid empty/null JSON values remain available; absent component means not applicable, unavailable means applicable but producer does not expose it. A wholly credential-only component is redacted; partial known-field removal retains the useful value with `redacted:true`. Reasons are closed identifiers (producerDoesNotExpose, credentialSource, invalidJSON, sourceBytes, encodedBytes, aggregateBytes, attemptCount); never exception prose or secret values.

| Approved default bound | Value / rationale |
| --- | --- |
| Source bytes scanned/copied per component | 1,048,576; finite native JSON parsing/copying comparable to existing event cap; check before parsing or UTF-8 scans/copies |
| Encoded component / success diagnostic aggregate | 131,072 / 262,144 bytes; at most two full components, below the 1 MiB event cap. Remaining components become explicit over-limit, not partial JSON |
| Essential attributed error envelope / optional error diagnostic aggregate | 32,768 / 24,576 encoded bytes; leaves 8,192 bytes within the existing 65,536 error cap for framing/markers/envelope overhead. Large optional detail does not replace actionable candidate summaries |
| Actual attempt evidence | 16 records; greater executions remain allowed, but evidence becomes explicitly over-limit with actual count if available. Do not change fallback execution or emit a misleading first-16 complete list |
| Header component | 64 entries, 8,192 aggregate original key/value bytes, 2,048 bytes per key/value; bound count before allocation and exclude known credentials before retention. Over-limit applies to the whole component |
| Configured discovery | 1,024 rows, 16 candidates/route, 128 aliases/route, 2,048 UTF-8 bytes per identity/display string; existing 4,194,304 complete-document cap. Reject excessive configured catalog atomically, never return a partial listing or infer inventory |
| Complete documents | Keep existing configurable client/server unary/error/event/discovery/stream limits. Evidence is subordinate to those limits; construction rejects invalid/overflowing limits, not unsupported ordinary data |

These are approved service defaults, not claims of live-size measurements. [Budget witnesses](../../../../ai-gateway/test/providerwire-v4/developer-evidence-bounds.test.ts) show exact/one-over encoded component and error allocations, JSON escaping growth, pre-parse source rejection, complete disposition JSON, 16 heterogeneous compact summaries and envelope headroom. They justify bounded feasibility; production aggregate/cardinality enforcement, fuzzing and real provider size evidence remain downstream acceptance. Limits do not increase automatically when evidence is requested.

On optional overflow, omit the component's value and report its disposition. On aggregate exhaustion, collapse optional native/details to one bounded aggregate over-limit marker; retain all essential request/selection/failure facts if they fit. The `attempts` shape is the complete ordered array up to 16, or `{state:over-limit,reason:attemptCount,count?:<actual observed count>}` above that limit, never a partial array; this is an explicit evidence support boundary, not an execution ceiling. Essential attempted identities/failure summaries exceeding their whole 32 KiB allocation fail adaptation explicitly before unary/error commitment (or produce the existing terminal adaptation error after SSE commitment), not generic successful-looking evidence. Never truncate a native message to fit. Ordinary content/metadata are not budget-trimmed or reserved away to make room: try the minimal optional marker after core encoding; if even core plus essential facts/minimal dispositions cannot fit the final configured document, retain existing whole-response/frame failure behavior. No second stream reader or retroactive replay is introduced.

Credential exclusions are source/field specific, case-insensitive for header names:

| Source | Excluded material / retained useful data |
| --- | --- |
| Gateway caller auth | Authorization CAP/bearer, X-Access-Token, X-Grafana-Id, cookies and token-exchange credentials never become native/server-reflected evidence; verified public route facts remain |
| Native HTTP auth | Authorization, Proxy-Authorization, x-api-key, api-key, x-goog-api-key, Cookie, Set-Cookie; x-amz-security-token and signing credentials/signatures. Ordinary application/response diagnostic headers are not blanket excluded |
| Native credential URLs | Userinfo and documented auth query keys (access_token/api_key, AWS X-Amz-Credential/Signature/Security-Token) removed under the adapter's verified URL policy; preserve credential-free endpoint/project/region/model topology and ordinary query values. No generic ban on URLs or arbitrary key-looking strings |
| Provider/workload config | apiKey, accessKeyId/secretAccessKey/sessionToken, OAuth/workload tokens, googleCredentials.privateKey/clientEmail, API-key environment/secret references; retain authorized provider/model/project/location/region identifiers |
| BYOK/body options | Exact `providerOptions.gateway.byok` credential subtree and native credential override fields when their source schema identifies them. Request/model mappings that are topology are projected separately by their owner; no whole provider-options or prompt censorship |
| Native bodies/errors | Preserve useful message/type/code/status/details and application strings; remove credential-bearing fields identified by the producer's source shape. If that particular source cannot be safely projected, report unavailable/redacted at that component, not a categorical transport ban |

The sanitized-shape witness deliberately retains `sk-ordinary-*` prompts/headers and authorized topology while excluding known dummy source credentials/references and leaving source objects unchanged. It is not a production redactor or proof that arbitrary upstream echoes are safe. No token-pattern DLP is promised for callers deliberately putting secrets into ordinary application content. Native source-specific credential rules and header casing/URL handling must be proved against the actual producers in #322/#323/#324; request-scoped BYOK capture/selection stays #317.

Operator capture settings govern server recording only. Consumer configuration/destinations independently govern application capture. Pinned client Request.Body contains submitted gateway.byok (the access test asserts this); server exclusions cannot sanitize that caller-owned object. #317 owns credential-aware consumer capture guidance/defaults. Neither enabling native evidence nor consumer capture enables operator capture.

## Ownership and approval

Approved concrete edges:

```text
#318 -> #319
#318 -> #280                     (#319 only for mapped-fallback continuation proof)
#321 approval + #319 + #280 -> #322
#321 approval + #280 + #322 -> #323
#321 approval -> #324
#318/#319 + #322 + #324 -> #316
```

| Owner | Seam / required downstream spec update |
| --- | --- |
| #280 | One general opaque metadata server codec and independent Go decoding; no Gateway evidence schema, credential projector or collision relocation |
| #322 | One request-scoped actual-attempt attribution seam, Gateway routing/evidence/collision placement and unary/setup/committed/all-failed error projection; retain Go APICallError.Data once. Updates client closed-error/backend-concealment and protocol/fallback attribution requirements |
| #323 | Native result/error source projection and optional-component credential/budget dispositions; reuse #322 attribution, envelope/data codec and Gateway assembly; no competing collector or additional reader |
| #324 | Authorized configured catalog row projection, Go ModelInfo.Gateway/types and documented/typechecked TS configured-discovery helper/validation. Updates catalog metadata and client discovery concealment requirements; no dependence on #280/#322/#323 |
| #316 | Routing selection only, consuming the above vocabulary/access/attribution; no second capture/codec |
| #299 / #317 | Separately coordinated core producer/error policy and BYOK secret-aware capture; absence is identified, not a reverse prerequisite or work absorbed here |

#321 has no production prerequisite on its downstream consumers. #320 scalar fidelity and #318 options run independently; inspect their delivered revisions before refreshing current-loss assertions. No stopped #303/#309 collector/codec/API, temporary allowlist or umbrella completion gate is required.

Owner approval covers: (1) the additive evidence/data schema and selected-versus-completed semantics, (2) reserved gateway collision relocation and documented omitted Vercel fields, (3) numeric budgets/explicit unavailable support boundaries including >16-attempt evidence, and (4) the small Go discovery public field/types plus TS companion helper. Tests demonstrate consumption and feasibility, not delivery of these production changes. Downstream owners implement the approved contract under their scoped issues; this approval does not expand #321 into production implementation.

## Validation

Passed against the unchanged registered baseline:

- TS workspace typecheck and all 11 dedicated access/budget tests; new files are registered in `test:client`.
- `providers/grafana`: full `go test ./...`, `go test -race ./...`, `go vet ./...`; standalone `GOWORK=off go test -mod=readonly ./...` also passes with its committed root pin.
- `mise run validate-parity-baseline`, `mise run test-providerwire-v4`, `mise run parity-check`, `mise run verify-ai-gateway-boundary`, `mise run verify-merged-pins` and strict OpenSpec validation.
- The default parity run skips the advisory provider-shape report because npm source layout is unavailable to that reporter. Separately, `AI_SDK_UPSTREAM_ROOT=<temporary exact-commit provider export> mise run parity-provider-shape` passes all five comparisons. An attempted full-suite override to that non-Git export was rejected by coverage's Git provenance check; the normal full suite was rerun successfully. No pins, provenance checks or expected fixtures were weakened.

Review classification: Go metadata/error/discovery losses are existing implementation/access gaps owned by #280/#322/#324, not accepted permanent secrecy rules. TS transport replacement/default body omission are pinned normalization constraints. The additive schema, collision relocation, omitted ambiguous Vercel fields and explicit resource dispositions are owner-approved extensions/support boundaries. The producer transport inventory is source evidence, not live recording proof.

Only tests, their runner registration and change artifacts are modified. No provider input/expectation files, production codecs/public declarations, capture configuration or baseline dependencies changed. Production credential enforcement, complete configured-catalog validation and actual native producer availability must be proved by the downstream owners. PARITY.md is unchanged: these design/access witnesses do not claim improved production retention or completed native/frontend parity.

All 14 tasks are complete, including explicit owner approval. The change is synced and archived for PR delivery. Downstream production implementation remains with its scoped owners; no GitHub comments or delegation were performed.
