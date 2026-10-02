## Context

This delivers [#324](https://github.com/grafana/ai-sdk/issues/324), following proposal.md, the revised Gateway plan's work package 37, and the [approved #321 decision](../archive/2026-10-02-define-gateway-developer-evidence-contract/decision.md). The decision authorizes the exact configured-discovery shape, small Go public addition and TS companion helper. Subsequent owner feedback places route policy solely at startup, removes server discovery response-size gating and uses ordinary client decoding. This does not authorize a new discovery dialect or customer account system.

Current flow is strict YAML → resolved provider configuration → `service.BuildCatalog` → immutable `catalog.ModelInfo` → authenticated discovery handler. Catalog listing has one sorted canonical entry with aliases; HTTP discovery expands aliases and sorts all rows lexicographically. Every row's specification remains `v4`/`grafana`/that row's public ID. Configured candidate descriptors are retained with listing metadata; `namespace.go` copies candidates with aliases/capabilities. Route policy belongs to strict startup configuration, not discovery response building. Go `ListModels` uses ordinary typed JSON decoding after bounding received bytes.

Pinned reference is `@ai-sdk/gateway` 4.0.94, `ai` 7.0.116, provider 4.0.18 and provider-utils 5.0.49, commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. Matching [gateway-fetch-metadata.ts](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/gateway/src/gateway-fetch-metadata.ts) and [tests](https://github.com/vercel/ai/blob/ee3169b3c4880e2abe4d0d7c781243bb81822ec4/packages/gateway/src/gateway-fetch-metadata.test.ts) use authenticated `GET /config`, a stripping object schema, optional/null description, optional pricing transforms and model-type filtering. Our additive `gateway` object therefore remains compatible but is not available from stock normalized discovery. #321's TS/Go access probes confirm both current losses; its test-only raw helper is not a complete production validator. This is a Grafana extension, not a Vercel private-service parity claim.

`PARITY.md` classifies Gateway runtime/client and host composition as mixed. Required proof is independent server projection, hostile client tests and pinned-client/real-command HTTP consumption, not provider recordings or UI chunk snapshots. No inference or frontend SSE format changes occur here.

## Goals / Non-Goals

**Goals:**

- Inspect explicit canonical routes, aliases and ordered configured provider/model candidates through authenticated discovery without generation or provider inventory calls.
- Preserve normalized rows, public-ID grammar, row ordering, specification identity, existing Go discovery method and immutable catalog behavior.
- Validate route semantics and cardinality/string policy at startup; serve the complete catalog without a discovery response cap, while clients retain transport read safeguards.
- Document actual access and current account boundaries, with targeted credential exclusions and independently configured operator capture.

**Non-Goals:**

- Runtime attempts, selected/response identity, native transport, opaque result metadata, execution/fallback policy, new request routing or BYOK.
- Account provisioning, tenant authorization machinery, credential storage, unsupported provider activation, availability guarantees, ranking or a published TS client package.
- Relaxing the existing 1–128 ASCII public-ID grammar; the 2,048-byte identity ceiling is an upper bound, not permission to expand route IDs.

## Decisions

### 1. Carry explicit safe candidate values through the existing catalog

Add a small catalog candidate value containing provider-instance reference, effective native adapter/provider identifier and exact configured invocation model ID, and a candidate slice to `catalog.ModelInfo`. The command populates it alongside model construction from the same primary/fallback descriptors and provider configuration, before canonical middleware wrapping. For current types the effective provider is `anthropic`, `openai`, or the compatible provider's configured `providerName`/existing `openai-compatible` default. Verify this against constructors; do not use the final wrapped model's `Provider()`/`ModelID()` or provider-reported response identity.

Store only these string facts, not `config.Provider`, `ResolvedProvider`, credentials, base URLs, environment references, middleware state or executable candidate models. Deep-copy candidates with other nested metadata at construction and listing, including registry-backed catalogs when explicitly supplied. Catalogs without supplied candidate metadata keep ordinary public discovery; they do not infer it by querying models or inventories. Catalog metadata describes configuration, not which candidate will run or succeed.

Alternative rejected: reflect provider models/configuration or reuse operator attempt records. Reflection risks credential disclosure, and runtime observations cannot establish an uninvoked configured catalog. This small explicit value keeps the transport-neutral catalog free of HTTP types and execution policy.

### 2. One additive projection on the existing authenticated route

For command-configured routes, each canonical and alias row receives the same `gateway` value:

```json
{"canonicalModelId":"grafana/assistant","aliases":["assistant"],"candidates":[{"providerInstance":"primary","provider":"anthropic","modelId":"native-primary"},{"providerInstance":"secondary","provider":"openai","modelId":"native-secondary"}]}
```

`canonicalModelId` is the catalog ID; `aliases` preserves explicit alias order, including an empty array; candidates preserve primary followed by fallback order. The outer `id` and `specification.modelId` remain the individual row ID, never the native ID. No configured route is synthesized from response identity. Generic entries lacking candidate metadata omit `gateway` rather than emit incomplete or fabricated facts.

Server projection uses explicit private DTO fields and standard JSON encoding, with the updated closed discovery test schema as output evidence. Encode the complete projected catalog before HTTP 200; listing errors/panics use the existing fixed internal error. Do not revalidate route semantics or gate discovery on encoded size. Configuration loading owns policy before listener creation; custom host owners must supply valid metadata.

Alternatives rejected: new endpoint, display-string encoding, or assumed stock TS retention. All contradict the approved one-route access decision.

### 3. Independent Go retention and a tested copyable TS helper

Go adds `ModelInfo.Gateway *ConfiguredRoute`, with `CanonicalModelID`, `Aliases` and `Candidates []ConfiguredCandidate`; the candidate type contains `ProviderInstance`, `Provider`, and `ModelID`. `Provider.ListModels(ctx)` remains the sole discovery method and uses the existing auth/request/error handling. Missing/null `gateway` remains nil for ordinary rows. Use ordinary Go JSON decoding into typed values, including normal field-case, missing/null and surrogate semantics. Type-decoding errors fail the whole result; unrelated additive fields remain ignored rather than exposed. No server types, validators or AGPL imports enter this module.

Supply a copyable TS consumer example under `ai-gateway/examples/`, imported/typechecked and exercised by the registered ProviderWire workspace: `fetchConfiguredModels({baseURL, headers, fetch, signal, maxBytes})`, returning a typed `{models}` document retaining the recognized route extension. It accepts explicit JWT or CAP outer headers; it neither exchanges tokens nor selects/falls back between authentication modes. Base URL is the existing HTTP(S) API prefix, without userinfo/query/fragment, and `/config` is appended without losing that prefix. Use HTTPS in deployment guidance.

The helper performs one GET with redirect refusal, checks status and JSON media type, reads incrementally under the approved byte cap, validates raw UTF-8 and one complete JSON document, and checks the JSON shape/types of recognized metadata before return, without route-policy validation. Abort cancels pending network/read work; rejected or completed reads release/cancel the body reader. Non-2xx and validation errors do not embed credential headers or arbitrary response bodies. No unbounded `response.json()`/`text()` path, caching or inference is introduced. `maxBytes` defaults to 4,194,304 and must be a positive safely represented integer no greater than that approved helper cap; lower values are supported. Go retains its configurable read limit, defaulting to 4,194,304 bytes (4 MiB). The server has no discovery response-size cap; accepted configuration determines what is served.

Consumers retain supplied metadata rather than checking public-ID grammar, nonblank strings, specification agreement, route groups or duplicate entries. These are server configuration rules, with canonical/alias consistency guaranteed by projection. Candidate uniqueness at startup is by `(providerInstance, modelId)`; different instances of the same adapter/model remain valid. Standard JSON last-member behavior remains unchanged. TS checks runtime JSON shape/types; Go uses encoding/json's typed decoding, including zero/nil values for missing/null fields. This accepted Go adaptation does not imply identical handling of malformed shapes across languages.

Alternative rejected: publish a TS package or modify pinned `getAvailableModels`. A tested companion example gives explicit access without a new distribution/API surface. Shared HTTP fixtures may prove agreement, but each client validator remains independent of production server validation.

### 4. Validate configuration once; bound client reads independently

Config loading owns route policy, before readiness or inference:

| Dimension | Ceiling |
| --- | ---: |
| Configured expanded HTTP rows, including aliases | 1,024 |
| Configured candidates per route | 16 |
| Configured aliases per route | 128 |
| Configured identity/display string | 2,048 UTF-8 bytes |

The public-ID grammar remains 1–128 ASCII bytes. Required candidate strings are nonblank valid UTF-8; descriptions may be empty. Existing namespace/reference and duplicate-candidate/alias checks stay in startup validation. Discovery does not repeat those checks or impose a response-size requirement; remove the discovery response-byte setting, startup feasibility pass and custom preflight/assembly machinery. Ordinary typed JSON projection serves every configured visible row without truncation, including catalogs larger than the former 1 MiB cap.

Go reads within its configurable DiscoveryBytes limit, defaulting to 4 MiB; the TS helper defaults to/maxes out at 4 MiB and supports smaller values. Consumers decode metadata without numeric or semantic route policy. Go normalizes escaped lone UTF-16 surrogates to U+FFFD; TS retains the decoded UTF-16 value. No lossless cross-client agreement is promised for those escapes. Raw malformed UTF-8, malformed JSON, incompatible field types and client read overflow still fail atomically with cleanup. Tests distinguish startup policy, full server projection and client transport/type safety.

### 5. Reuse the deployed visibility boundary, without manufacturing BYOK isolation

Authentication still precedes listing, and the request context reaches the existing `ModelLister`. Projection consumes only its visible entries; it never reads an unfiltered global configuration to supplement a scoped listing. Host decorators must apply the same route/candidate visibility to listing and resolution. Scoped test catalogs prove this seam and isolation of projections, not deployed entitlement enforcement.

The current command builds one startup catalog and wires it to both operations, including existing Cloud-auth test composition. It does not yet construct request-scoped customer/BYOK accounts. Describe that limitation explicitly in operator/support evidence; do not advertise Cloud authentication as customer account isolation or silently introduce a mode ban/policy engine here. Internal configured-account visibility is the delivered feature; #317 owns future customer account construction and prevention of internal-key fallback. Real customer deployment requires that separate boundary, not these fixtures.

Project only the approved facts. Keep secret values/references and unrelated provider/account entries excluded by construction; test them with distinct dummy markers while retaining useful provider/model identifiers and ordinary key-looking display text. Discovery permission does not enable server payload capture or alter operator logical metric labels. Existing response/error/native-diagnostic concealment changes remain with their owners; delta specs carve out configured discovery only.

### 6. Deliver regression proof, docs and scope-consistent spec updates together

Begin with failing independent server/Go/TS discovery cases and real-command witnesses. Update #321's Go discovery-loss expectation to retention and TS proposed-helper expectation to the delivered helper; stock TS stripping remains explicitly tested. Real-command tests run raw HTTP, pinned normalized TS discovery, the TS helper and Go `ListModels` against configured direct/mixed-fallback aliases, asserting zero native inference requests and correct auth. JWT rejection and dummy CAP scope/credential denial precede discovery; Cloud fixtures retain their explicit evidence limit.

Update catalog, client, provider-configuration and fallback concealment requirements only where #324 changes discovery. Preserve runtime wire/error behavior, fallback guards and operator capture rules. Client guidance owns working access recipes; Gateway docs own operator configuration/visibility and deployment limitations; godoc owns exhaustive Go field reference. Register/typecheck/test the TS example within the existing exact-pinned workspace. Update PARITY.md only for durable configured-discovery evidence/support boundaries. No invented provider recordings or conformance regeneration is necessary for this catalog feature.

## Risks / Trade-offs

- [Every alias repeats route data and can expand encoded bytes] → Serve the complete configured catalog; document independent client read limits and test large discovery without inference.
- [A future scoped lister bypasses visibility by rebuilding global facts] → Populate/project only explicit visible metadata and test paired listing/resolution decorators.
- [Compatible provider name differs from configured type or wrapped identity] → Use effective constructor configuration and verify default/custom provider identifiers before middleware wrapping.
- [Stock normalized TS still drops candidates] → Document helper access and keep executable stock-loss tests; do not claim permissive parsing exposes discarded fields.
- [New ceilings reject previously accepted large deployments] → Fail startup clearly without credential values; document bounds before rollout rather than silently truncate or automatically increase limits.
- [Parallel #318/#319/#320/#322 spec work overlaps historical concealment] → Reconfirm parent-stack changes and preserve delivered runtime changes; rebase/retarget main after the parent merges, modifying only discovery exceptions and never resurrecting historical runtime restrictions.
- [Copied TS helper diverges or fails to honor redirects/abort in custom fetch] → Test the shipped source, require fetch implementations honoring standard options, and document that injected fetch is a caller-owned transport trust boundary.

## Migration Plan

Implement and review on the already-correct authorized stack: `nrbrd/gw-routing` contains approved #321/#327 at `7b0b35cf` above canonical-main base `c0299776`. Parent PR #327 (`nrbrd/gw-contract` → `main`) is open; no restructuring or merge-first implementation gate is necessary. Reconfirm parent ancestry, changes, approval and baseline, and validate cumulative source. When PR delivery is authorized, the child targets `nrbrd/gw-contract` while #327 is open, merges only after the parent, and rebases/retargets main after parent merge. Do not imply that #321 is already merged. “Fresh from main” excludes stopped #303/#309 reuse, not authorized prerequisite stacking.

Existing consumers ignore additive server fields; updated Go clients accept absent extensions, and TS users explicitly adopt the helper. No new endpoint, auth mode or dependency baseline is needed. The approved simplification removes discovery.response-bytes and its environment binding rather than retaining an unused setting. Candidate/alias limits are startup compatibility constraints and must be documented in release/deployment guidance.

Rollback removes the additive projection or reverts the feature deployment. Updated Go discovery still handles unextended responses; examples must handle unavailable configured metadata rather than infer it. Maintain the current candidate-source Gateway workspace/image policy and standalone readonly Apache client checks; do not introduce local replacement directives or require Gateway as a standalone published module.

## Open Questions

None in the approved discovery/access contract. At implementation start, reconfirm approved parent-stack ancestry/changes, exact registered versions and any concurrently delivered catalog/spec changes; parent merge is not an implementation prerequisite. If current account visibility cannot safely support this scoped projection or the public access contract must change, stop for owner approval rather than silently expanding scope.
